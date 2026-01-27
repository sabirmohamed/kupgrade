# kupgrade Roadmap

> **Vision**: The only single-binary tool that covers BEFORE, DURING, and AFTER Kubernetes upgrades.

---

## The Complete Upgrade Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         kupgrade: Full Upgrade Lifecycle                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   BEFORE                    DURING                      AFTER               │
│   ───────                   ──────                      ─────               │
│   kupgrade check            kupgrade watch              kupgrade report     │
│                                                                              │
│   • API deprecations        • Real-time stages          • Upgrade summary   │
│   • Feature gates           • Drain progress            • Duration stats    │
│   • PDB readiness           • Blocker detection         • Issues found      │
│   • Resource capacity       • Migration tracking        • Recommendations   │
│   • Webhook health          • Event stream                                  │
│                                                                              │
│   "Is it safe to start?"    "What's happening?"         "What happened?"    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Command Structure

```bash
# BEFORE: Pre-flight validation
kupgrade check                           # Full pre-flight report
kupgrade check --target-version 1.32     # Check upgrade to specific version
kupgrade check --snapshot                # Save state for post-upgrade comparison

# DURING: Real-time observation
kupgrade watch                           # Current functionality
kupgrade watch --check-first             # Run pre-flight, then watch

# AFTER: Post-upgrade analysis
kupgrade report                          # Generate upgrade report
kupgrade report --compare                # Compare to pre-upgrade snapshot
```

---

## Implementation Phases

### Phase 1: Polish WATCH (Current Priority)

**Goal:** Make the observer production-ready and impressive.

| Task | Status | Notes |
|------|--------|-------|
| Split view.go into focused files | TODO | Unblocks AI-assisted development |
| Blocker remediation suggestions | TODO | "How to fix" not just "what's blocked" |
| Describe overlay (d key) | TODO | k9s parity for nodes and pods |
| Drain velocity + ETA | PARTIAL | DrainStartTime exists, need rate calc |
| Event aggregation | TODO | Collapse routine events |
| Keyboard simplification | TODO | Mnemonic keys (d/b/e) not numbers |

### Phase 2: Minimal CHECK

**Goal:** Pre-flight validation with minimal code addition (~400 lines).

| Task | Status | Notes |
|------|--------|-------|
| Check engine interface | TODO | Pluggable check architecture |
| PDB readiness check | TODO | Reuse existing PDB watcher logic |
| Node conditions check | TODO | Reuse existing node watcher logic |
| Resource capacity check | TODO | Can cluster handle pod surge? |
| `kupgrade check` command | TODO | CLI integration |
| Text reporter | TODO | Terminal output |

**Deferred to v2:** Full deprecation scanner, webhook health check, etcd check.

### Phase 3: REPORT with State Diffing

**Goal:** Answer "was this broken before the upgrade?"

| Task | Status | Notes |
|------|--------|-------|
| Snapshot capture | TODO | Save workload state to JSON |
| State differ | TODO | Compare before/after |
| Diff categorization | TODO | New issue vs pre-existing vs resolved |
| `kupgrade report --compare` | TODO | CLI integration |

---

## Pre-flight Check Categories

### Tier 1: Blocking Checks (Must Pass)

| Check | What It Does | Complexity |
|-------|--------------|------------|
| Node Conditions | All nodes Ready? | Low - reuse watcher |
| PDB Readiness | Will PDBs block drains? | Low - reuse watcher |

### Tier 2: Warning Checks (Should Review)

| Check | What It Does | Complexity |
|-------|--------------|------------|
| Resource Capacity | Headroom for pod surge? | Medium |
| Pending Pods | Pods that can't schedule? | Low |

### Tier 3: Future (v2)

| Check | What It Does | Complexity |
|-------|--------------|------------|
| API Deprecations | Resources using removed APIs | High - needs data pipeline |
| Feature Gates | Removed feature gates in use | Medium |
| Webhook Health | Admission webhooks responding | Medium |
| etcd Health | Control plane healthy | Low |

---

## State Diffing: Pre/Post Upgrade Comparison

### The Problem

```
Post-upgrade Slack conversation:
────────────────────────────────
Engineer: "The upgrade broke legacy-cron-job, it's in CrashLoopBackOff!"
SRE: "Was it working before?"
Engineer: "I... don't know. Let me check logs from 3 hours ago..."
[2 hours of debugging later]
Engineer: "Oh, it was already broken. False alarm."
```

### The Solution

```bash
# Before upgrade
$ kupgrade check --snapshot
Snapshot saved: ~/.kupgrade/snapshots/prod-cluster-2024-01-26.json
  • 142 pods across 12 namespaces
  • 3 pods unhealthy (will be tracked as pre-existing)

# After upgrade
$ kupgrade report --compare
```

### Diff Report Output

```
┌─────────────────────────────────────────────────────────────────────────┐
│ kupgrade report  prod-cluster  v1.28 → v1.29                            │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ SUMMARY                                                                 │
│   Healthy before & after:    43  ✓                                      │
│   New issues:                 2  ⚠ ← INVESTIGATE THESE                  │
│   Pre-existing issues:        1  ● ← ALREADY BROKEN                     │
│   Resolved:                   1  ✓ ← UPGRADE FIXED                      │
│                                                                         │
├─────────────────────────────────────────────────────────────────────────┤
│ ⚠ NEW ISSUES (2) — Likely caused by upgrade                            │
│                                                                         │
│ deployment/app-worker (prod)                                            │
│   Before:  Running (3/3 ready)                                          │
│   After:   CrashLoopBackOff (0/3 ready)                                │
│   Action:  Check if node OS changed broke shell compatibility          │
│                                                                         │
├─────────────────────────────────────────────────────────────────────────┤
│ ● PRE-EXISTING (1) — Already broken before upgrade                     │
│                                                                         │
│ cronjob/legacy-reporter (default)                                       │
│   Status:  CrashLoopBackOff → CrashLoopBackOff (UNCHANGED)             │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Diff Categorization Rules

| Before State | After State | Category |
|--------------|-------------|----------|
| Healthy | Unhealthy | NEW_ISSUE |
| Unhealthy | Unhealthy | PRE_EXISTING |
| Unhealthy | Healthy | RESOLVED |
| Healthy | Healthy | UNCHANGED (hidden) |

---

## Competitive Positioning

| Feature | kubent | pluto | kubepug | k9s | **kupgrade** |
|---------|:------:|:-----:|:-------:|:---:|:------------:|
| Pre-flight checks | ✓ | ✓ | ✓ | ✗ | ✓ |
| Real-time observer | ✗ | ✗ | ✗ | ✓ | ✓ |
| Upgrade-focused | ✗ | ✗ | ✗ | ✗ | ✓ |
| Blocker remediation | ✗ | ✗ | ✗ | ✗ | ✓ |
| Post-upgrade diff | ✗ | ✗ | ✗ | ✗ | ✓ |
| Single binary | ✓ | ✓ | ✓ | ✓ | ✓ |

**Unique value:** Only tool covering full BEFORE → DURING → AFTER lifecycle.

---

## Success Metrics

| Metric | Target |
|--------|--------|
| Time to answer "is this stuck and why" | < 2 seconds |
| Time to answer "was this already broken" | < 30 seconds |
| GitHub stars (6 months) | 500+ |
| Reddit launch impression | Front page of r/kubernetes |

---

## Package Structure (Future State)

```
kupgrade/
├── cmd/kupgrade/main.go
├── internal/
│   ├── cli/
│   │   ├── root.go, watch.go, version.go  # Existing
│   │   ├── check.go                        # Phase 2
│   │   └── report.go                       # Phase 3
│   ├── check/                              # Phase 2
│   │   ├── engine.go, check.go
│   │   ├── pdb.go, nodes.go, capacity.go
│   ├── snapshot/                           # Phase 3
│   │   ├── snapshot.go, loader.go
│   ├── differ/                             # Phase 3
│   │   ├── differ.go, categorize.go
│   ├── stage/          # Existing
│   ├── watcher/        # Existing
│   ├── tui/            # Existing (split into view_*.go)
│   └── kube/           # Existing
├── pkg/types/          # Existing + additions
└── docs/               # You are here
```
