# kupgrade Observability Plan

## Vision

**k9s-style layered screens** for maximum observability during Kubernetes rolling upgrades.

Each screen focuses on one concern. Users switch between screens with keyboard shortcuts.

**Scope:** Observer only (read-only). kupgrade watches and reports - it does not cordon, drain, or modify cluster state. This keeps the tool safe to run during production upgrades without risk of unintended actions.

---

## Data Architecture

```
┌─────────────────────┐     ┌────────────────────┐     ┌─────────────────┐
│   K8s Informers     │     │   Computed State   │     │     Screens     │
│   (raw watches)     │ ──▶ │   (derived data)   │ ──▶ │   (view only)   │
├─────────────────────┤     ├────────────────────┤     ├─────────────────┤
│ • Node informer     │     │ • Node stages      │     │ • Overview      │
│ • Pod informer      │     │ • Eviction queue   │     │ • Nodes         │
│ • PDB informer      │     │ • Blockers list    │     │ • Drains        │
│ • Event informer    │     │ • Stage durations  │     │ • Pods          │
└─────────────────────┘     │ • Velocity/ETA     │     │ • Blockers      │
                            │ • Attention flags  │     │ • Events        │
                            └────────────────────┘     │ • Stats         │
                                                       └─────────────────┘
```

**Data Flow:**
1. Informers receive real-time updates from K8s API (watch-based, not polling)
2. Each informer event triggers computed state recalculation
3. Screens render from computed state (never query K8s directly)
4. Screen refresh: 100ms debounce after state change

**State Ownership:**
- `NodeStageManager` - determines stage (READY/CORDONED/DRAINING/UPGRADING/COMPLETE)
- `BlockerDetector` - identifies PDBs, local storage, orphan pods blocking drains
- `EvictionTracker` - tracks pod eviction progress per draining node
- `MetricsCalculator` - computes velocity, ETA, stage durations

---

## Stage Detection Heuristics

How kupgrade determines each node's stage:

| Stage | Detection Logic |
|-------|-----------------|
| **READY** | `node.spec.unschedulable == false` AND version matches source version |
| **CORDONED** | `node.spec.unschedulable == true` AND pods still present (not draining yet) |
| **DRAINING** | Cordoned AND eviction activity detected (pods terminating or eviction events) |
| **UPGRADING** | Previously was DRAINING, now `Ready` condition is `False` or `Unknown` |
| **COMPLETE** | `Ready` condition is `True` AND kubelet version matches target version |

**Edge cases:**
- Node crashes during upgrade → stays in UPGRADING (NotReady) until recovered
- Rollback detected → version decreases, flag in EVENTS
- Node stuck in DRAINING > 10min → surface in BLOCKERS

---

## Screen Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  MAIN (default)                                                 │
│  └── Pipeline overview (current implementation)                 │
│                                                                 │
│  Press key → Switch screen                                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  [0] OVERVIEW   Pipeline stages with node cards (default)       │
│  [1] NODES      Full node details, conditions, taints           │
│  [2] DRAINS     Eviction progress per node, stuck pods          │
│  [3] PODS       Pod health, probes, phase by node               │
│  [4] BLOCKERS   PDBs, local storage, stuck evictions            │
│  [5] EVENTS     Full event log with filtering                   │
│  [6] STATS      Timing, velocity, ETA, history                  │
│                                                                 │
│  [/] FILTER     Filter by node/namespace                        │
│  [?] HELP       Keyboard shortcuts                              │
│  [q] QUIT                                                       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Screen Designs

### [0] OVERVIEW (Default) - Current Implementation

Pipeline stages with node cards. Entry point. **Surfaces attention indicators** so users know when to dive deeper.

```
⎈ kupgrade  aks-prod | v1.28→v1.29 | ████░░ 40% | ETA ~15m

⚠️  ATTENTION: node-04 drain stuck 3m (PDB: coredns-pdb) — [4] blockers

  READY(2)     CORDONED(1)    DRAINING(1)⚠️   UPGRADING(1)    COMPLETE(3)
 ┌────────┐   ┌────────┐    ┌────────┐     ┌────────┐      ┌────────┐
 │node-07 │   │node-03 │    │node-04 │     │node-05 │      │node-01 ✓│
 └────────┘   └────────┘    └────────┘     └────────┘      └────────┘

[1]nodes [2]drains [3]pods [4]blockers [5]events [6]stats [?]help [q]quit
```

**Attention triggers (shown in alert bar):**
- Drain stuck > 2 minutes
- PDB blocking eviction
- Pod stuck in Terminating > 5 minutes
- Node in UPGRADING > 15 minutes (possible failure)
- Connection to K8s API lost

---

### [1] NODES Screen

Full node details with conditions and taints.

```
⎈ kupgrade › NODES                                          [0] back

  NAME        VERSION   STAGE      AGE    CONDITIONS         TAINTS
  node-01     v1.29.0   COMPLETE   3m     Ready              -
  node-02     v1.29.0   COMPLETE   8m     Ready              -
  node-03     v1.28.2   CORDONED   2m     Ready              NoSchedule
  node-04     v1.28.2   DRAINING   5m     Ready              NoSchedule
  node-05     v1.28.2   UPGRADING  4m     NotReady           NoSchedule
► node-06     v1.28.2   READY      -      Ready,MemPressure  -
  node-07     v1.28.2   READY      -      Ready              -

[enter] details  [/] filter  [0] back
```

**Data to display:**
- Node name
- Kubelet version
- Current stage
- Time in stage
- Node conditions (Ready, MemoryPressure, DiskPressure, PIDPressure, NetworkUnavailable)
- Taints (NoSchedule, NoExecute)
- Kubelet vs API server version drift

---

### [2] DRAINS Screen

Eviction progress, queue, and stuck pods.

```
⎈ kupgrade › DRAINS                                         [0] back

  NODE        PROGRESS    REMAINING    STUCK    EVICTING
► node-04    ████████░░   3 pods       1        coredns-xyz

  EVICTION QUEUE (node-04):
  ├─ ✓ nginx-deployment-7b4d      evicted → node-01
  ├─ ✓ redis-cache-0              evicted → node-06
  ├─ ◐ coredns-5d78c9f8b4-xyz     evicting... (PDB blocked)
  ├─ ○ monitoring-agent-k8s4f     pending
  └─ ○ app-backend-6f9d           pending

  STUCK REASON: PodDisruptionBudget kube-system/coredns-pdb
                minAvailable: 2, currently available: 2

[enter] pod details  [/] filter  [0] back
```

**Data to display:**
- Nodes currently draining
- Progress bar (pods evicted / total)
- Eviction queue per node
- Pod eviction status (evicted, evicting, pending, stuck)
- Destination node for evicted pods
- Stuck reason (PDB, local storage, etc.)

---

### [3] PODS Screen

Pod health, probes, and phase by node.

```
⎈ kupgrade › PODS                                           [0] back

  NODE       POD                        PHASE     READY   PROBES    AGE
  node-04    coredns-5d78c9f8b4-xyz    Running   1/1     ✓ L ✓ R   4h
  node-04    nginx-deployment-7b4d     Running   1/1     ✓ L ✓ R   2h
► node-04    app-backend-6f9d          Running   0/1     ✓ L ✗ R   1h
  node-05    (no pods - upgrading)
  node-01    redis-cache-0             Running   1/1     ✓ L ✓ R   5m

  POD DETAIL (app-backend-6f9d):
  ├─ Readiness Probe: FAILING (HTTP GET /health → 503)
  ├─ Last Success: 3m ago
  └─ Restart Count: 2

[enter] describe  [l] logs  [/] filter  [0] back
```

**Data to display:**
- Pods grouped by node
- Pod phase (Running, Pending, Failed, etc.)
- Ready containers (1/1, 0/1)
- Probe status (Liveness ✓/✗, Readiness ✓/✗)
- Pod age
- Restart count
- Probe failure details

---

### [4] BLOCKERS Screen

PDBs, local storage, and stuck evictions.

```
⎈ kupgrade › BLOCKERS                                       [0] back

  TYPE              NAME                      IMPACT         NODE
► PDB               kube-system/coredns-pdb   blocking 1     node-04
  LocalStorage      monitoring/prometheus-0    can't evict   node-04
  NoController      default/orphan-pod        manual delete  node-06

  BLOCKER DETAIL (coredns-pdb):
  ├─ minAvailable: 2
  ├─ currentHealthy: 2
  ├─ desiredHealthy: 2
  ├─ disruptionsAllowed: 0
  └─ Blocking: coredns-5d78c9f8b4-xyz on node-04

  RECOMMENDATION: Scale coredns to 3+ replicas or wait

[enter] details  [/] filter  [0] back
```

**Blocker types to detect:**
- PodDisruptionBudgets (PDBs) blocking eviction
- Pods with local storage (emptyDir, hostPath)
- Pods without controllers (orphan pods)
- Failed evictions
- Pods stuck in Terminating
- DaemonSet pods (informational)

---

### [5] EVENTS Screen

Full event log with filtering.

```
⎈ kupgrade › EVENTS                               filter: all  [0] back

  TIME      TYPE     NODE      MESSAGE
  15:04:32  INFO     node-01   Upgrade complete v1.28.2 → v1.29.0
  15:04:28  INFO     node-01   Node uncordoned
  15:04:15  INFO     node-01   Node ready
► 15:03:58  WARN     node-04   Eviction blocked by PDB coredns-pdb
  15:03:45  INFO     node-04   Evicting coredns-5d78c9f8b4-xyz
  15:03:30  INFO     node-04   Drain started
  15:03:28  INFO     node-04   Node cordoned
  15:02:15  INFO     node-05   Node upgrading (NotReady)
  15:01:00  INFO     node-05   Drain complete

[/] filter by node/type  [f] toggle follow  [0] back
```

**Event types:**
- Node: cordoned, uncordoned, ready, not ready, version change
- Pod: evicted, scheduled, failed, stuck
- Blocker: PDB violation, eviction failed
- Upgrade: started, complete, rollback detected

---

### [6] STATS Screen

Timing, velocity, ETA, and history.

```
⎈ kupgrade › STATS                                          [0] back

  PROGRESS
  ├─ Nodes Complete:    3 / 8  (37.5%)
  ├─ Nodes In Progress: 3      (CORDONED: 1, DRAINING: 1, UPGRADING: 1)
  └─ Nodes Remaining:   2

  TIMING
  ├─ Upgrade Started:   14:45:00
  ├─ Elapsed:           22m 15s
  ├─ Avg per Node:      7m 25s
  └─ ETA:               ~15m (3 nodes × 5m avg)

  VELOCITY
  ├─ Current:           2.1 nodes/hour
  └─ Trend:             ↑ improving (was 1.8)

  HISTORY (this upgrade)
  ├─ node-01: READY → COMPLETE in 6m 30s
  ├─ node-02: READY → COMPLETE in 8m 12s
  └─ node-03: READY → CORDONED (2m ago)

[0] back
```

**Metrics to track:**
- Total progress (nodes complete / total)
- Nodes by stage
- Upgrade start time
- Elapsed time
- Average time per node
- ETA based on velocity
- Current velocity (nodes/hour)
- Velocity trend
- Per-node history with timing

---

## Scalability Considerations

**Large cluster handling (100+ nodes, 1000+ pods):**

| Concern | Solution |
|---------|----------|
| List rendering | Virtualized lists - only render visible rows |
| Pod explosion | Default filter: only pods on nodes in upgrade pipeline |
| Memory | Informer caches bounded; evict completed node history after 1hr |
| Screen updates | Debounce renders; batch state updates |

**Default behaviors:**
- PODS screen: Only show pods on CORDONED/DRAINING/UPGRADING nodes by default
- EVENTS screen: Rolling buffer of last 1000 events
- STATS history: Keep last 50 nodes, then summarize

---

## Error Handling

**Connection status indicator (header bar):**
```
⎈ kupgrade  aks-prod | v1.28→v1.29 | ████░░ 40% | ● Connected
⎈ kupgrade  aks-prod | v1.28→v1.29 | ████░░ 40% | ○ Reconnecting...
⎈ kupgrade  aks-prod | v1.28→v1.29 | ████░░ 40% | ✗ Disconnected (5s)
```

**Graceful degradation:**
- On disconnect: Show last known state, timestamp, reconnection attempts
- On reconnect: Full state reconciliation from informer cache
- Auth expiry: Prompt user to re-authenticate (show kubectl command)

---

## Implementation Epics

### Phase 1: Foundation (prerequisite for all screens)

| Epic | Description | Complexity |
|------|-------------|------------|
| E0 | Screen navigation framework + state architecture | Medium |

### Phase 2: Core Visibility (MVP)

| Epic | Screen | Value | Complexity |
|------|--------|-------|------------|
| E1 | [1] NODES screen | Validates framework, shows stage progression | Low |
| E2 | [4] BLOCKERS screen | High value - explains why things are stuck | High |
| E3 | [2] DRAINS screen | High value - real-time eviction progress | High |

### Phase 3: Enhanced Observability

| Epic | Screen | Value | Complexity |
|------|--------|-------|------------|
| E4 | [3] PODS screen | Pod-level detail for debugging | Medium |
| E5 | [5] EVENTS screen | Full event history | Low |
| E6 | [6] STATS screen | Timing, velocity, ETA | Medium |
| E7 | [/] Filter system | Navigate large clusters | Medium |
| E8 | Attention system | Surface problems on Overview | Medium |

**Dependency graph:**
```
E0 (navigation)
 ├── E1 (NODES) ─────────────────────┐
 ├── E2 (BLOCKERS) ──┐               │
 └── E3 (DRAINS) ────┴── E8 (Attention) ── Overview alerts
      │
      └── E4 (PODS) ── E5 (EVENTS) ── E6 (STATS) ── E7 (Filters)
```

---

## Data Requirements

### Informers Needed

| Informer | Key Fields | Screens |
|----------|------------|---------|
| **Node** | `spec.unschedulable`, `status.conditions`, `status.nodeInfo.kubeletVersion`, `spec.taints`, `metadata.creationTimestamp` | All |
| **Pod** | `spec.nodeName`, `status.phase`, `status.conditions`, `status.containerStatuses[].state`, `status.containerStatuses[].restartCount`, `spec.containers[].livenessProbe`, `spec.containers[].readinessProbe`, `metadata.deletionTimestamp`, `spec.terminationGracePeriodSeconds`, `spec.volumes` (for local storage detection) | PODS, DRAINS, BLOCKERS |
| **PodDisruptionBudget** | `status.currentHealthy`, `status.desiredHealthy`, `status.disruptionsAllowed`, `spec.minAvailable`, `spec.maxUnavailable`, `spec.selector` | BLOCKERS |
| **Event** | `involvedObject`, `reason`, `message`, `type`, `lastTimestamp`, `count` | EVENTS |

### Computed State

| State | Computation | Used By |
|-------|-------------|---------|
| **Node stage** | State machine: READY→CORDONED→DRAINING→UPGRADING→COMPLETE | All screens |
| **Stage timestamps** | Record time when node enters each stage | NODES, STATS |
| **Eviction queue** | Pods on draining nodes, sorted by eviction order | DRAINS |
| **Pod eviction status** | `evicted` / `evicting` / `pending` / `stuck` based on pod state | DRAINS |
| **Blocker list** | PDBs with `disruptionsAllowed=0`, pods with local storage, orphan pods | BLOCKERS, Overview |
| **Stuck detection** | Drain > 2min with no progress, pod Terminating > 5min | BLOCKERS, Overview |
| **Velocity** | `nodesCompleted / elapsedTime` | STATS |
| **ETA** | `remainingNodes × avgTimePerNode` | Overview, STATS |
| **Attention flags** | Boolean flags for conditions requiring user attention | Overview |

### Blocker Detection Logic

| Blocker Type | Detection |
|--------------|-----------|
| **PDB** | `pdb.status.disruptionsAllowed == 0` AND pods on draining node match PDB selector |
| **Local storage** | Pod has `emptyDir` or `hostPath` volume |
| **Orphan pod** | Pod has no `ownerReferences` (no controller) |
| **Stuck Terminating** | `metadata.deletionTimestamp` set AND `now - deletionTimestamp > terminationGracePeriodSeconds + 30s` |
| **Failed eviction** | Eviction API returned error (tracked via events) |

---

## Keyboard Shortcuts

All shortcuts are **navigation only** - kupgrade is read-only and never modifies cluster state.

| Key | Action |
|-----|--------|
| `0` | Overview (default) |
| `1` | Nodes screen |
| `2` | Drains screen |
| `3` | Pods screen |
| `4` | Blockers screen |
| `5` | Events screen |
| `6` | Stats screen |
| `/` | Filter |
| `?` | Help |
| `q` | Quit |
| `Enter` | Select/Details (view more info) |
| `Esc` | Back to previous screen |
| `↑↓` or `jk` | Navigate list |
| `g` / `G` | Jump to top / bottom of list |
| `f` | Toggle follow mode (Events screen) |

---

## Future Considerations (Post-MVP)

Explicitly deferred to keep MVP focused:

| Feature | Reason to Defer |
|---------|-----------------|
| **Write actions** (cordon, drain, uncordon) | Safety - observer-only reduces risk |
| **Multi-cluster support** | Complexity - single cluster sufficient for MVP |
| **Historical data persistence** | Scope - in-memory sufficient initially |
| **Custom alerting thresholds** | Config complexity - use sensible defaults first |
| **Integration with PagerDuty/Slack** | External dependencies |
| **Upgrade orchestration** | Out of scope - kupgrade observes, doesn't control |

---

## Reference

Inspired by [k9s](https://k9scli.io/) - the Kubernetes CLI that makes cluster management easy.

Focus: **Rolling upgrades only, maximum observability, read-only safety.**
