# kupgrade Architecture Reference

> **Purpose**: Comprehensive codebase documentation for AI agents and developers.
> **Last Updated**: 2026-01-26
> **Style Guide**: [Google Go Style Guide](https://google.github.io/styleguide/go/)

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Codebase Statistics](#2-codebase-statistics)
3. [Package Structure](#3-package-structure)
4. [Data Flow Architecture](#4-data-flow-architecture)
5. [Component Deep Dive](#5-component-deep-dive)
6. [Type Definitions](#6-type-definitions)
7. [Interface Contracts](#7-interface-contracts)
8. [TUI Architecture](#8-tui-architecture)
9. [Keyboard Navigation](#9-keyboard-navigation)
10. [Style & Color System](#10-style--color-system)
11. [Dependencies](#11-dependencies)
12. [Architecture Decisions](#12-architecture-decisions)
13. [Known Issues & Tech Debt](#13-known-issues--tech-debt)
14. [Architecture vs Implementation](#14-architecture-vs-implementation)

---

## 1. Executive Summary

**kupgrade** is a real-time Kubernetes upgrade observer CLI built in Go. It watches cluster resources during upgrades and displays node stages, pod migrations, blockers (PDBs), and events in a terminal UI.

### Core Design Principles

1. **Single Source of Truth**: Stage computation happens in `internal/stage/computer.go` only
2. **Unidirectional Data Flow**: Watchers → Channels → TUI (no computation in TUI)
3. **Non-blocking**: Ring buffer channels prevent TUI freezes
4. **Interface-first**: All major components defined by interfaces for testability
5. **kubectl-compatible**: Uses `ConfigFlags` for standard Kubernetes CLI behavior

### Key Technologies

| Component | Technology | Purpose |
|-----------|------------|---------|
| CLI | Cobra | kubectl-compatible flags |
| TUI | Bubble Tea | Elm architecture terminal UI |
| Styling | Lip Gloss | Terminal styling |
| K8s Client | client-go | Informer-based watching |
| Semver | golang.org/x/mod/semver | Version comparison |

---

## 2. Codebase Statistics

### Lines of Code by File

| File | Lines | Description |
|------|------:|-------------|
| **cmd/kupgrade/** | | |
| `main.go` | 15 | Entry point |
| **internal/cli/** | | |
| `root.go` | 39 | Root command, ConfigFlags |
| `watch.go` | 82 | Watch command |
| `version.go` | 18 | Version command |
| **internal/kube/** | | |
| `client.go` | 57 | K8s client wrapper |
| `version.go` | 16 | Server version |
| **internal/signals/** | | |
| `signals.go` | 26 | Signal handling |
| **internal/stage/** | | |
| `computer.go` | 135 | Stage computation (SINGLE SOURCE OF TRUTH) |
| **internal/watcher/** | | |
| `interfaces.go` | 71 | Interface definitions |
| `manager.go` | 250 | Watcher coordination |
| `nodes.go` | 302 | Node watcher |
| `pods.go` | 380 | Pod watcher |
| `events.go` | 135 | K8s Event watcher |
| `pdbs.go` | 143 | PDB watcher |
| `migrations.go` | 123 | Migration tracker |
| `stage.go` | 10 | StageComputer factory |
| **internal/tui/** | | |
| `model.go` | 283 | TUI state |
| `update.go` | 420 | Message handlers |
| `view.go` | 1,427 | Render functions (LARGEST FILE) |
| `styles.go` | 269 | Lip Gloss styles |
| `messages.go` | 35 | Message types |
| `keys.go` | 80 | Keyboard bindings |
| `navigation_test.go` | 210 | Tests |
| **pkg/types/** | | |
| `event.go` | 65 | Event types |
| `node.go` | 49 | Node types |
| `pod.go` | 22 | Pod types |
| `migration.go` | 24 | Migration types |
| `blocker.go` | 18 | Blocker types |
| **TOTAL** | **4,704** | |

### Package Size Distribution

```
internal/tui/      2,724 lines (58%)  <- Largest package (UI rendering)
internal/watcher/  1,414 lines (30%)  <- Watchers + manager
pkg/types/           178 lines (4%)   <- Shared types
internal/cli/        139 lines (3%)   <- CLI commands
internal/stage/      135 lines (3%)   <- Stage logic
internal/kube/        73 lines (2%)   <- K8s client
Other (main+signals)  41 lines (<1%)
─────────────────────────────────────
TOTAL              4,704 lines
```

---

## 3. Package Structure

```
kupgrade/
├── cmd/kupgrade/
│   └── main.go                 # Entry point (15 lines)
│
├── internal/
│   ├── cli/                    # Cobra CLI layer
│   │   ├── root.go             # Root cmd, ConfigFlags setup
│   │   ├── watch.go            # Watch command implementation
│   │   └── version.go          # Version command
│   │
│   ├── kube/                   # Kubernetes client
│   │   ├── client.go           # Client wrapper, Factory creation
│   │   └── version.go          # Server version detection
│   │
│   ├── signals/                # OS signal handling
│   │   └── signals.go          # SIGTERM/SIGINT → context.Cancel
│   │
│   ├── stage/                  # Stage computation
│   │   └── computer.go         # SINGLE SOURCE OF TRUTH for stages
│   │
│   ├── watcher/                # Informer-based watchers
│   │   ├── interfaces.go       # Watcher, EventEmitter, StageComputer, MigrationTracker
│   │   ├── manager.go          # Coordinates all watchers, ring buffer channels
│   │   ├── nodes.go            # Node informer + state building
│   │   ├── pods.go             # Pod informer + state building
│   │   ├── events.go           # K8s Event informer (filtered)
│   │   ├── pdbs.go             # PDB informer → Blocker detection
│   │   ├── migrations.go       # Pod delete/add correlation
│   │   └── stage.go            # StageComputer factory
│   │
│   └── tui/                    # Bubble Tea TUI
│       ├── model.go            # TUI state (Model)
│       ├── update.go           # Message handlers (Update)
│       ├── view.go             # Render functions (View)
│       ├── styles.go           # Lip Gloss styles (Tokyo Night)
│       ├── messages.go         # TUI message types
│       ├── keys.go             # Keyboard bindings
│       └── navigation_test.go  # Navigation tests
│
├── pkg/types/                  # Shared types (exported)
│   ├── event.go                # Event, EventType, Severity
│   ├── node.go                 # NodeState, NodeStage
│   ├── pod.go                  # PodState
│   ├── migration.go            # Migration, PendingMigration
│   └── blocker.go              # Blocker, BlockerType
│
├── docs/
│   ├── ARCHITECTURE.md         # This document
│   └── images/                 # Screenshots
│
├── go.mod                      # Go 1.22, dependencies
├── go.sum
├── README.md                   # User documentation
└── REFACTOR_PLAN.md            # Development notes
```

---

## 4. Data Flow Architecture

### High-Level Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              main.go                                     │
│    ctx := signals.SetupSignalHandler()                                  │
│    configFlags → restConfig → clientset → factory                       │
└──────────────────────────────┬──────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                          watcher.Manager                                 │
│                                                                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐    │
│  │NodeWatcher  │  │ PodWatcher  │  │EventWatcher │  │ PDBWatcher  │    │
│  │ (informer)  │  │ (informer)  │  │ (informer)  │  │ (informer)  │    │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘    │
│         │                │                │                │            │
│         ▼                ▼                ▼                ▼            │
│  ┌──────────────────────────────────────────────────────────────┐      │
│  │                    EventEmitter Interface                     │      │
│  │   Emit()  EmitNodeState()  EmitPodState()  EmitBlocker()     │      │
│  └──────────────────────────────┬───────────────────────────────┘      │
│                                 │                                       │
│                                 ▼                                       │
│  ┌──────────────────────────────────────────────────────────────┐      │
│  │                   Ring Buffer Channels                        │      │
│  │   eventCh (100)  nodeStateCh (50)  podStateCh (200)          │      │
│  │   blockerCh (50)                                              │      │
│  └──────────────────────────────┬───────────────────────────────┘      │
│                                 │                                       │
│  ┌───────────────────────┐      │                                       │
│  │   StageComputer       │◄─────┤  (Nodes call ComputeStage)           │
│  │   (stage/computer.go) │      │                                       │
│  └───────────────────────┘      │                                       │
│                                 │                                       │
│  ┌───────────────────────┐      │                                       │
│  │  MigrationTracker     │◄─────┤  (Pods call OnPodDelete/OnPodAdd)    │
│  │  (migrations.go)      │      │                                       │
│  └───────────────────────┘      │                                       │
└─────────────────────────────────┼───────────────────────────────────────┘
                                  │
                                  ▼ Channels
┌─────────────────────────────────────────────────────────────────────────┐
│                            tui.Model                                     │
│                                                                         │
│  Init() → Batch(waitForEvent, waitForNodeState, waitForPodState,       │
│           waitForBlocker, tick, spinnerTick)                            │
│                                                                         │
│  Update(msg) → switch msg.(type):                                       │
│      EventMsg     → append to events[]                                  │
│      NodeUpdateMsg → store in nodes map, rebuildNodesByStage()         │
│      PodUpdateMsg  → store in pods map                                  │
│      BlockerMsg    → add/remove from blockers[]                         │
│      TickMsg       → update currentTime                                 │
│      SpinnerMsg    → advance spinnerFrame                               │
│      KeyMsg        → navigation/screen switching                        │
│                                                                         │
│  View() → renderOverview() / renderNodesScreen() / ...                 │
└─────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                             Terminal                                     │
│  ★ kupgrade  my-cluster  v1.28 → v1.29  ████████░░  62%  14:32:07       │
└─────────────────────────────────────────────────────────────────────────┘
```

### Channel Semantics (Ring Buffer)

All channels use ring buffer semantics to prevent watcher blocking:

```go
// From manager.go:128-138
func (m *Manager) Emit(event types.Event) {
    select {
    case m.eventCh <- event:
        // Sent successfully
    default:
        // Channel full - drop oldest, send new
        select {
        case <-m.eventCh:  // Remove oldest
        default:
        }
        m.eventCh <- event
    }
}
```

### Buffer Sizes

| Channel | Buffer Size | Rationale |
|---------|-------------|-----------|
| `eventCh` | 100 | Event history for display |
| `nodeStateCh` | 50 | Max reasonable cluster size |
| `podStateCh` | 200 | Higher pod churn during drains |
| `blockerCh` | 50 | PDBs typically fewer |

---

## 5. Component Deep Dive

### 5.1 Entry Point (`cmd/kupgrade/main.go`)

```go
func main() {
    if err := cli.Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

15 lines. Delegates entirely to `cli.Execute()`.

### 5.2 CLI Layer (`internal/cli/`)

**root.go** - Sets up Cobra root command with ConfigFlags:

```go
var ConfigFlags *genericclioptions.ConfigFlags  // Package-level (TODO: pass explicitly)

func NewRootCmd() *cobra.Command {
    ConfigFlags = genericclioptions.NewConfigFlags(true)
    cmd := &cobra.Command{...}
    ConfigFlags.AddFlags(cmd.PersistentFlags())  // Adds --context, --kubeconfig, etc.
    cmd.AddCommand(NewWatchCmd())
    cmd.AddCommand(NewVersionCmd())
    return cmd
}
```

**watch.go** - Main command implementation:

```go
func runWatch(cmd *cobra.Command, args []string) error {
    ctx := signals.SetupSignalHandler()           // 1. Signal handling
    client, _ := kube.NewClient(ConfigFlags)      // 2. K8s client
    serverVersion, _ := client.ServerVersion(ctx) // 3. Get cluster version

    manager := watcher.NewManager(client.Factory, client.Namespace, targetVersion)
    manager.Start(ctx)                            // 4. Start watchers

    stageComputer := manager.StageComputer()
    detectedTarget := stageComputer.TargetVersion()
    lowestVersion := stageComputer.LowestVersion()

    model := tui.New(tui.Config{                  // 5. Create TUI model
        Context:         client.Context,
        ServerVersion:   lowestVersion,           // Lowest node version as "from"
        TargetVersion:   detectedTarget,          // Highest node version as "to"
        InitialNodes:    manager.InitialNodeStates(),
        InitialPods:     manager.InitialPodStates(),
        InitialBlockers: manager.InitialBlockers(),
        EventCh:         manager.Events(),
        NodeStateCh:     manager.NodeStateUpdates(),
        PodStateCh:      manager.PodStateUpdates(),
        BlockerCh:       manager.BlockerUpdates(),
    })

    p := tea.NewProgram(model, tea.WithAltScreen())
    p.Run()                                       // 6. Run TUI (blocking)
    manager.Wait()                                // 7. Clean shutdown
    return nil
}
```

### 5.3 Kubernetes Client (`internal/kube/`)

Uses `genericclioptions.ConfigFlags` pattern from kubectl:

```go
func NewClient(configFlags *genericclioptions.ConfigFlags) (*Client, error) {
    restConfig, _ := configFlags.ToRESTConfig()
    clientset, _ := kubernetes.NewForConfig(restConfig)

    // Get context name for display
    contextName := ""
    if configFlags.Context != nil && *configFlags.Context != "" {
        contextName = *configFlags.Context
    } else {
        rawConfig, _ := configFlags.ToRawKubeConfigLoader().RawConfig()
        contextName = rawConfig.CurrentContext
    }

    // Create informer factory (0 = no periodic resync)
    factory := informers.NewSharedInformerFactory(clientset, 0)

    return &Client{Clientset, Factory, Context, Namespace}, nil
}
```

### 5.4 Stage Computer (`internal/stage/computer.go`)

**SINGLE SOURCE OF TRUTH** for node stage computation:

```go
type Computer struct {
    targetVersion  string         // Highest version seen (upgrade target)
    lowestVersion  string         // Lowest version seen (upgrade source)
    nodePodCounts  map[string]int // Pod count per node
    mu             sync.RWMutex   // Thread safety
}

func (c *Computer) ComputeStage(node *corev1.Node) types.NodeStage {
    version := node.Status.NodeInfo.KubeletVersion
    schedulable := !node.Spec.Unschedulable
    ready := isNodeReady(node)

    // Detect active upgrade (mixed versions)
    upgradeActive := lowest != "" && target != "" && lowest != target

    switch {
    case upgradeActive && version == target && ready && schedulable:
        return types.StageComplete    // At target, healthy
    case !ready:
        return types.StageUpgrading   // Node rebooting/upgrading
    case !schedulable:
        return types.StageCordoned    // Cordoned (NodeWatcher may correct to DRAINING)
    default:
        return types.StageReady       // Normal state
    }
}

func (c *Computer) SetTargetVersion(version string) {
    // Tracks both highest (target) and lowest (source) versions
    if semver.Compare(version, c.targetVersion) > 0 {
        c.targetVersion = version
    }
    if semver.Compare(version, c.lowestVersion) < 0 {
        c.lowestVersion = version
    }
}
```

**Stage Progression:**
```
READY → CORDONED → DRAINING → UPGRADING → COMPLETE
  │         │          │           │           │
  │         │          │           │           └─ At target version, ready, schedulable
  │         │          │           └─ Node not ready (rebooting)
  │         │          └─ Cordoned + pods being evicted
  │         └─ Unschedulable but pods still present
  └─ Normal operation
```

### 5.5 Watcher Manager (`internal/watcher/manager.go`)

Coordinates all informer-based watchers:

```go
type Manager struct {
    factory      informers.SharedInformerFactory
    namespace    string

    eventCh      chan types.Event      // Ring buffer (100)
    nodeStateCh  chan types.NodeState  // Ring buffer (50)
    podStateCh   chan types.PodState   // Ring buffer (200)
    blockerCh    chan types.Blocker    // Ring buffer (50)
    wg           sync.WaitGroup        // Shutdown coordination

    nodeWatcher  *NodeWatcher
    podWatcher   *PodWatcher
    eventWatcher *EventWatcher
    pdbWatcher   *PDBWatcher
    migrations   MigrationTracker
    stages       StageComputer
}

func (m *Manager) Start(ctx context.Context) error {
    m.factory.Start(ctx.Done())                          // Start informers
    synced := m.factory.WaitForCacheSync(ctx.Done())     // Wait for initial sync

    m.nodeWatcher.Start(ctx)   // Register handlers
    m.podWatcher.Start(ctx)
    m.eventWatcher.Start(ctx)
    m.pdbWatcher.Start(ctx)

    if tracker, ok := m.migrations.(*migrationTracker); ok {
        m.wg.Add(1)
        go func() {                                      // Migration cleanup goroutine
            defer m.wg.Done()
            tracker.runCleanup(ctx)
        }()
    }

    return nil
}
```

### 5.6 Node Watcher (`internal/watcher/nodes.go`)

Watches nodes and builds `NodeState`:

```go
type NodeWatcher struct {
    informer        cache.SharedIndexInformer
    emitter         EventEmitter
    stages          StageComputer
    podCounter      func(nodeName string) int
    drainStartTimes map[string]time.Time  // Drain timing
    initialPodCount map[string]int        // For drain progress
}

func (w *NodeWatcher) onUpdate(oldObj, newObj interface{}) {
    newNode := newObj.(*corev1.Node)
    w.stages.SetTargetVersion(newNode.Status.NodeInfo.KubeletVersion)
    w.emitChangeEvents(oldNode, newNode)  // Cordon/uncordon, ready/notready
    w.emitter.EmitNodeState(w.buildState(newNode))
}

func (w *NodeWatcher) buildState(node *corev1.Node) types.NodeState {
    podCount := w.podCounter(node.Name)
    stage := w.stages.ComputeStage(node)

    // Drain progress tracking
    if stage == types.StageDraining || stage == types.StageCordoned {
        if _, ok := w.drainStartTimes[node.Name]; !ok {
            w.drainStartTimes[node.Name] = time.Now()
            w.initialPodCount[node.Name] = podCount
        }
        // Correct CORDONED → DRAINING if pods evicted
        if podCount < w.initialPodCount[node.Name] {
            stage = types.StageDraining
        }
    }

    return types.NodeState{
        Name, Stage, Version, Ready, Schedulable, PodCount,
        Conditions, Taints, Age,
        DrainStartTime, InitialPodCount, DrainProgress,
    }
}
```

### 5.7 Pod Watcher (`internal/watcher/pods.go`)

Watches pods and tracks migrations:

```go
func (w *PodWatcher) onAdd(obj interface{}) {
    pod := obj.(*corev1.Pod)
    w.emitter.EmitPodState(w.buildState(pod))

    if pod.Spec.NodeName != "" {
        w.stages.UpdatePodCount(pod.Spec.NodeName, 1)
        w.emitter.RefreshNodeState(pod.Spec.NodeName)
    }

    // Check for migration completion
    if migration := w.migrations.OnPodAdd(pod); migration != nil {
        w.emitter.Emit(types.Event{
            Type:    types.EventMigration,
            Message: fmt.Sprintf("Pod migrated: %s → %s", migration.FromNode, migration.ToNode),
        })
    }
}

func (w *PodWatcher) onDelete(obj interface{}) {
    pod := obj.(*corev1.Pod)
    w.stages.UpdatePodCount(pod.Spec.NodeName, -1)
    w.emitter.RefreshNodeState(pod.Spec.NodeName)
    w.migrations.OnPodDelete(pod)  // Track for potential migration
    w.emitter.EmitPodState(types.PodState{Name, Namespace, Deleted: true})
}
```

### 5.8 PDB Watcher (`internal/watcher/pdbs.go`)

Detects blocking PodDisruptionBudgets:

```go
func isBlocking(pdb *policyv1.PodDisruptionBudget) bool {
    // PDB blocks when disruptionsAllowed is 0 and pods exist
    return pdb.Status.DisruptionsAllowed == 0 && pdb.Status.ExpectedPods > 0
}

func (w *PDBWatcher) buildBlocker(pdb *policyv1.PodDisruptionBudget) *types.Blocker {
    if !isBlocking(pdb) {
        return nil
    }

    var detail string
    if pdb.Spec.MinAvailable != nil {
        detail = fmt.Sprintf("minAvailable=%s, %d/%d healthy → 0 evictions allowed",
            pdb.Spec.MinAvailable.String(),
            pdb.Status.CurrentHealthy,
            pdb.Status.ExpectedPods)
    }
    // ... similar for MaxUnavailable

    return &types.Blocker{Type: types.BlockerPDB, Name, Namespace, Detail}
}
```

### 5.9 Migration Tracker (`internal/watcher/migrations.go`)

Correlates pod deletes with creates via owner reference:

```go
type migrationTracker struct {
    pending map[string]types.PendingMigration  // Key: owner UID
    mu      sync.Mutex
}

func (t *migrationTracker) OnPodDelete(pod *corev1.Pod) {
    owner := getControllerOwner(pod)  // Get controller UID
    if owner == "" { return }         // Standalone pod

    t.pending[owner] = types.PendingMigration{
        OwnerRef:  owner,
        FromNode:  pod.Spec.NodeName,
        PodName:   pod.Name,
        Timestamp: time.Now(),
    }
}

func (t *migrationTracker) OnPodAdd(pod *corev1.Pod) *types.Migration {
    owner := getControllerOwner(pod)
    if owner == "" || pod.Spec.NodeName == "" { return nil }

    if pending, ok := t.pending[owner]; ok {
        delete(t.pending, owner)
        if pending.FromNode != pod.Spec.NodeName {
            return &types.Migration{FromNode, ToNode, OldPod, NewPod}
        }
    }
    return nil
}

// Cleanup removes stale entries (5 min TTL, 30s interval)
func (t *migrationTracker) runCleanup(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    for {
        select {
        case <-ctx.Done(): return
        case <-ticker.C: t.Cleanup(5 * time.Minute)
        }
    }
}
```

---

## 6. Type Definitions

### 6.1 Event Types (`pkg/types/event.go`)

```go
type EventType string

const (
    // Node events
    EventNodeCordon   EventType = "NODE_CORDON"
    EventNodeUncordon EventType = "NODE_UNCORDON"
    EventNodeReady    EventType = "NODE_READY"
    EventNodeNotReady EventType = "NODE_NOTREADY"
    EventNodeVersion  EventType = "NODE_VERSION"

    // Pod events
    EventPodEvicted   EventType = "POD_EVICTED"
    EventPodScheduled EventType = "POD_SCHEDULED"
    EventPodReady     EventType = "POD_READY"
    EventPodFailed    EventType = "POD_FAILED"
    EventPodDeleted   EventType = "POD_DELETED"

    // K8s events
    EventK8sWarning EventType = "K8S_WARNING"
    EventK8sError   EventType = "K8S_ERROR"
    EventK8sNormal  EventType = "K8S_NORMAL"

    // Migration
    EventMigration EventType = "MIGRATION"
)

type Severity string
const (
    SeverityInfo    Severity = "info"
    SeverityWarning Severity = "warning"
    SeverityError   Severity = "error"
)

type Event struct {
    Type      EventType
    Severity  Severity
    Timestamp time.Time
    Message   string
    NodeName  string
    PodName   string
    Namespace string
    Reason    string  // K8s event reason
}
```

### 6.2 Node Types (`pkg/types/node.go`)

```go
type NodeStage string

const (
    StageReady     NodeStage = "READY"
    StageCordoned  NodeStage = "CORDONED"
    StageDraining  NodeStage = "DRAINING"
    StageUpgrading NodeStage = "UPGRADING"
    StageComplete  NodeStage = "COMPLETE"
)

type NodeState struct {
    Name        string
    Stage       NodeStage
    Version     string
    Ready       bool
    Schedulable bool
    PodCount    int
    Deleted     bool  // True when node was deleted

    // Phase 2 fields
    InitialPodCount int
    DrainProgress   int
    Blocked         bool
    BlockerReason   string
    DrainStartTime  time.Time
    WaitingPods     []string

    // Enhanced details
    Conditions []string  // MemoryPressure, DiskPressure, etc.
    Taints     []string  // NoSchedule, NoExecute, etc.
    Age        string    // "5d", "2h", etc.
}

func AllStages() []NodeStage {
    return []NodeStage{StageReady, StageCordoned, StageDraining, StageUpgrading, StageComplete}
}
```

### 6.3 Pod Types (`pkg/types/pod.go`)

```go
type PodState struct {
    Name            string
    Namespace       string
    NodeName        string
    Ready           bool
    ReadyContainers int     // 1 in "1/2"
    TotalContainers int     // 2 in "1/2"
    Phase           string  // Running, CrashLoopBackOff, Terminating, etc.
    Restarts        int
    LastRestartAge  string  // "4m", "8h", or "" if no restarts
    Age             string
    HasLiveness     bool
    HasReadiness    bool
    LivenessOK      bool
    ReadinessOK     bool
    OwnerKind       string  // Deployment, DaemonSet, StatefulSet
    OwnerRef        string  // Controller UID
    Deleted         bool
}
```

### 6.4 Blocker Types (`pkg/types/blocker.go`)

```go
type BlockerType string

const (
    BlockerPDB       BlockerType = "PDB"
    BlockerPV        BlockerType = "PV"
    BlockerDaemonSet BlockerType = "DaemonSet"
)

type Blocker struct {
    Type      BlockerType
    Name      string
    Namespace string
    Detail    string  // "minAvailable=2, 2/2 healthy → 0 evictions allowed"
    NodeName  string
    Cleared   bool    // True when blocker resolved
}
```

### 6.5 Migration Types (`pkg/types/migration.go`)

```go
type Migration struct {
    Owner     string  // Controller UID
    FromNode  string
    ToNode    string
    OldPod    string
    NewPod    string
    Namespace string
    Timestamp time.Time
    Complete  bool    // True when new pod is Ready
}

type PendingMigration struct {
    OwnerRef  string
    FromNode  string
    PodName   string
    Namespace string
    Timestamp time.Time
}
```

---

## 7. Interface Contracts

### 7.1 Watcher Interface

```go
// Watcher observes a Kubernetes resource type and emits events.
type Watcher interface {
    // Start begins watching. Returns when ctx is cancelled.
    // MUST call WaitForCacheSync before returning from initialization.
    // MUST NOT block the caller - run watch loop in goroutine.
    Start(ctx context.Context) error
}
```

### 7.2 EventEmitter Interface

```go
// EventEmitter sends events, node state, and pod state updates.
type EventEmitter interface {
    // Emit sends an event. MUST NOT block.
    Emit(event types.Event)

    // EmitNodeState sends a node state update. MUST NOT block.
    EmitNodeState(state types.NodeState)

    // EmitPodState sends a pod state update. MUST NOT block.
    EmitPodState(state types.PodState)

    // EmitBlocker sends a blocker update. MUST NOT block.
    EmitBlocker(blocker types.Blocker)

    // RefreshNodeState triggers a node state refresh (e.g., when pods change).
    RefreshNodeState(nodeName string)
}
```

### 7.3 StageComputer Interface

```go
// StageComputer determines node upgrade stage from observable state.
type StageComputer interface {
    // ComputeStage returns current stage for a node.
    ComputeStage(node *corev1.Node) types.NodeStage

    // UpdatePodCount updates the pod count for a node.
    UpdatePodCount(nodeName string, delta int)

    // SetTargetVersion sets the upgrade target.
    SetTargetVersion(version string)

    // TargetVersion returns the current target version.
    TargetVersion() string

    // LowestVersion returns the lowest version seen across nodes.
    LowestVersion() string

    // PodCount returns the pod count for a node.
    PodCount(nodeName string) int
}
```

### 7.4 MigrationTracker Interface

```go
// MigrationTracker correlates pod deletes with creates to detect migrations.
type MigrationTracker interface {
    // OnPodDelete records a potential migration source.
    OnPodDelete(pod *corev1.Pod)

    // OnPodAdd checks for migration correlation.
    // Returns Migration if this pod completes a migration, nil otherwise.
    OnPodAdd(pod *corev1.Pod) *types.Migration

    // Cleanup removes stale pending migrations.
    // SHOULD be called periodically (e.g., every 30s).
    Cleanup(maxAge time.Duration)
}
```

### 7.5 Compile-Time Interface Check

```go
// manager.go:22
var _ EventEmitter = (*Manager)(nil)
```

---

## 8. TUI Architecture

### 8.1 Elm Architecture (Model-Update-View)

```
┌─────────────────────────────────────────────────────────────┐
│                    BUBBLE TEA LOOP                           │
│                                                             │
│   ┌─────────┐    ┌─────────┐    ┌─────────┐                │
│   │  Model  │───▶│ Update  │───▶│  View   │                │
│   │(state)  │    │(reducer)│    │(render) │                │
│   └────▲────┘    └────┬────┘    └────┬────┘                │
│        │              │              │                      │
│        └──────────────┴──────────────┘                      │
│                      Msg                                    │
└─────────────────────────────────────────────────────────────┘
```

### 8.2 Model State (`internal/tui/model.go`)

```go
type Model struct {
    config Config  // Immutable configuration

    // Dimensions
    width, height int

    // Navigation state
    screen        Screen   // Current screen (0-6)
    overlay       Overlay  // Modal overlay (none, help, detail)
    selectedStage int      // For Overview pipeline
    selectedNode  int      // For Overview (legacy)
    listIndex     int      // For list-based screens

    // Data (display only - no computation)
    nodes        map[string]types.NodeState
    nodesByStage map[types.NodeStage][]string  // Rebuilt on node updates
    pods         map[string]types.PodState     // key: namespace/name
    events       []types.Event                 // Ring buffer (max 100)
    migrations   []types.Migration             // Ring buffer (max 50)
    blockers     []types.Blocker
    eventCount   int

    // Animation
    spinnerFrame int
    currentTime  time.Time

    // Error
    fatalError error
}
```

### 8.3 Screens

```go
type Screen int

const (
    ScreenOverview  Screen = iota  // 0 - Pipeline + node list + blockers + events
    ScreenNodes                    // 1 - Full node details table
    ScreenDrains                   // 2 - Drain progress per node
    ScreenPods                     // 3 - Pod list with health/probes
    ScreenBlockers                 // 4 - PDBs blocking evictions
    ScreenEvents                   // 5 - Full event log
    ScreenStats                    // 6 - Progress statistics
)
```

### 8.4 Message Types (`internal/tui/messages.go`)

```go
type EventMsg struct { Event types.Event }      // From watcher
type NodeUpdateMsg struct { Node types.NodeState }
type PodUpdateMsg struct { Pod types.PodState }
type BlockerMsg struct { Blocker types.Blocker }
type ErrorMsg struct { Err error; Recoverable bool }
type TickMsg struct{}      // 1s timer for time display
type SpinnerMsg struct{}   // 500ms timer for spinner animation
```

### 8.5 Update Flow (`internal/tui/update.go`)

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        return m.handleKey(msg)
    case tea.WindowSizeMsg:
        m.width, m.height = msg.Width, msg.Height
    case NodeUpdateMsg:
        m.handleNodeUpdate(msg.Node)  // Store + rebuildNodesByStage()
        return m, waitForNodeState(m.config.NodeStateCh)
    case PodUpdateMsg:
        m.handlePodUpdate(msg.Pod)    // Store in pods map
        return m, waitForPodState(m.config.PodStateCh)
    case BlockerMsg:
        m.handleBlockerUpdate(msg.Blocker)  // Add/remove from blockers
        return m, waitForBlocker(m.config.BlockerCh)
    case EventMsg:
        m.handleEvent(msg.Event)      // Append to events
        return m, waitForEvent(m.config.EventCh)
    case TickMsg:
        m.currentTime = time.Now()
        return m, tick()
    case SpinnerMsg:
        m.spinnerFrame = (m.spinnerFrame + 1) % 4
        return m, spinnerTick()
    }
    return m, nil
}
```

### 8.6 View Rendering (`internal/tui/view.go`)

**Overview Screen Layout:**
```
┌─────────────────────────────────────────────────────────────┐
│ ★ kupgrade  cluster  v1.28 → v1.29  ████░░  40%  14:32:07   │  renderCompactHeader()
├─────────────────────────────────────────────────────────────┤
│   READY  →  CORDONED  →  DRAINING  →  UPGRADING  →  COMPLETE│  renderPipelineRow()
│     2          1           1            0             1     │
├─────────────────────────────────────────────────────────────┤
│ ⚠ BLOCKERS (1)                                              │  renderBlockersSection()
│ PDB default/my-pdb    minAvailable=2 → 0 evictions allowed  │  (only if blockers exist)
├─────────────────────────────────────────────────────────────┤
│ ◐ DRAINING: NODE-ABC                                        │  renderDrainProgressSection()
│ ████████████░░░░  18/24 pods evicted    Elapsed: 4m 12s     │  (only if draining)
├─────────────────────────────────────────────────────────────┤
│ NODES (5)                                   ↑↓ navigate     │  renderNodeList()
│ ► node-abc    18 pods   v1.28   DRAINING                    │
│   node-def    12 pods   v1.29   COMPLETE                    │
├─────────────────────────────────────────────────────────────┤
│ ● EVENTS                                                    │  renderEventsSection()
│ 14:32:05  ⚠ Node node-abc cordoned                          │
├─────────────────────────────────────────────────────────────┤
│ 0 overview  1 nodes  2 drains  3 pods  ? help  q quit       │  renderFooter()
└─────────────────────────────────────────────────────────────┘
```

---

## 9. Keyboard Navigation

### 9.1 Key Bindings (`internal/tui/keys.go`)

| Key | Action | Context |
|-----|--------|---------|
| `0-6` | Switch screens | Global |
| `q` | Quit (from Overview) / Return to Overview (from other screens) | Global |
| `?` | Toggle help overlay | Global |
| `Esc` | Close overlay / Return to Overview | Global |
| `↑/k` | Move up in list | All screens |
| `↓/j` | Move down in list | All screens |
| `←/h` | Previous stage | Overview pipeline |
| `→/l` | Next stage | Overview pipeline |
| `Enter` | Show details | Lists |
| `g` | Go to top | Lists |
| `G` | Go to bottom | Lists |
| `Ctrl+U` | Page up | Lists |
| `Ctrl+D` | Page down | Lists |

### 9.2 Navigation Logic

```go
func (m *Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
    // 1. Handle overlays first (capture input)
    if m.overlay != OverlayNone {
        return m.handleOverlayKey(msg)
    }

    // 2. Quit: from Overview exits, from other screens returns to Overview
    if matchKey(msg, keys.Quit) {
        if m.screen != ScreenOverview {
            m.screen = ScreenOverview
            return *m, nil
        }
        return *m, tea.Quit
    }

    // 3. Screen navigation (0-6)
    if screen := screenFromKey(msg); screen >= 0 {
        m.screen = screen
        m.listIndex = 0  // Reset list position
        return *m, nil
    }

    // 4. Delegate to screen-specific handler
    switch m.screen {
    case ScreenOverview:
        return m.handleOverviewKey(msg)
    case ScreenNodes:
        return m.handleNodesKey(msg)
    // ...
    }
}
```

---

## 10. Style & Color System

### 10.1 Tokyo Night Theme (`internal/tui/styles.go`)

Based on [Omarchy Tokyo Night](https://github.com/omarchy/themes/tokyo-night):

```go
// Backgrounds (layered depth)
colorBg        = "#1a1b26"  // Main background
colorBgAlt     = "#16161e"  // Panel/sidebar
colorBgHover   = "#1f2335"  // Hover state
colorSelected  = "#414868"  // Selected item
colorBorder    = "#565f89"  // Borders
colorBorderDim = "#32344a"  // Subtle borders

// Text hierarchy
colorText      = "#a9b1d6"  // Primary text
colorTextBold  = "#c0caf5"  // Emphasized
colorTextMuted = "#787c99"  // Secondary
colorTextDim   = "#565f89"  // Tertiary

// Stage colors (semantic)
colorReady     = "#787c99"  // Muted grey - waiting
colorCordoned  = "#e0af68"  // Yellow - warning/paused
colorDraining  = "#ff9e64"  // Orange - active attention
colorUpgrading = "#7dcfff"  // Cyan - in progress
colorComplete  = "#9ece6a"  // Green - done

// Status colors
colorError   = "#f7768e"  // Red
colorWarning = "#e0af68"  // Yellow
colorSuccess = "#9ece6a"  // Green
colorInfo    = "#7aa2f7"  // Blue
```

### 10.2 Layout Constants

```go
const (
    headerProgressBarWidth = 10
    nodeCardMinWidth       = 16
    nodeCardMaxWidth       = 30
    nodeCardGapWidth       = 2
    stageCount             = 5
    eventTypeWidth         = 14
    eventTimestampWidth    = 8
    panelGapWidth          = 2
    panelMinWidth          = 25
)
```

### 10.3 Style Examples

```go
// Header
headerStyle = lipgloss.NewStyle().Bold(true).Foreground(colorInfo)

// Stage-specific styling
func stageStyle(stage string) lipgloss.Style {
    return lipgloss.NewStyle().Foreground(stageColors[stage]).Bold(true)
}

// Node cards
nodeCardSelected = nodeCardBase.Copy().
    Border(lipgloss.DoubleBorder()).
    BorderForeground(colorAccent).
    Background(colorSelected)

// Events
infoStyle    = lipgloss.NewStyle().Foreground(colorInfo)
warningStyle = lipgloss.NewStyle().Foreground(colorWarning)
errorStyle   = lipgloss.NewStyle().Foreground(colorError)
```

---

## 11. Dependencies

### 11.1 Direct Dependencies (`go.mod`)

```go
require (
    // CLI
    github.com/spf13/cobra v1.8.0

    // TUI
    github.com/charmbracelet/bubbletea v0.25.0
    github.com/charmbracelet/lipgloss v0.9.1

    // Kubernetes
    k8s.io/api v0.29.0
    k8s.io/cli-runtime v0.29.0
    k8s.io/client-go v0.29.0

    // Utilities
    golang.org/x/mod v0.8.0  // semver comparison
)
```

### 11.2 Dependency Rationale

| Dependency | Purpose | Alternative Considered |
|------------|---------|------------------------|
| `cobra` | kubectl-compatible CLI | `urfave/cli`, `kong` - less K8s ecosystem alignment |
| `bubbletea` | TUI framework | `tview`, `termui` - less elegant architecture |
| `lipgloss` | Terminal styling | Included with bubbletea ecosystem |
| `cli-runtime` | ConfigFlags pattern | Custom kubeconfig - more code, less battle-tested |
| `golang.org/x/mod/semver` | Version comparison | Manual parsing - error prone |

---

## 12. Architecture Decisions

### ADR-001: Cobra CLI

**Decision**: Use Cobra for CLI structure.

**Rationale**:
- kubectl uses Cobra - familiar patterns
- ConfigFlags integration for `--context`, `--kubeconfig`, `--namespace`
- Built-in shell completion

### ADR-002: ConfigFlags Pattern

**Decision**: Use `genericclioptions.ConfigFlags` instead of custom kubeconfig handling.

**Rationale**:
- Battle-tested edge case handling
- Free impersonation support (`--as`, `--as-group`)
- ~50 lines replaced by 3 lines

### ADR-003: Context-based Lifecycle

**Decision**: Use `context.Context` for lifecycle, not raw `chan struct{}`.

**Rationale**:
- Modern kubernetes pattern (`signals.SetupSignalHandler()`)
- Deadline support if needed
- Value propagation (logger, trace IDs)

### ADR-004: Ring Buffer Channels

**Decision**: Non-blocking sends with oldest-drop on overflow.

**Rationale**:
- Informer callbacks must return quickly
- Bounded memory
- Graceful degradation (drop oldest events)

### ADR-005: Owner-based Migration Tracking

**Decision**: Track migrations via ReplicaSet/Deployment owner references.

**Rationale**:
- Pod `spec.nodeName` is immutable after scheduling
- Pods don't "move" - old deleted, new created
- Owner reference links related pods

### ADR-006: Single Source of Truth for Stages

**Decision**: Stage computation only in `internal/stage/computer.go`.

**Rationale**:
- Prevents drift between watcher and TUI
- TUI is display-only
- Clear data flow

### ADR-007: Auto-detect Target Version

**Decision**: Infer target from highest version in cluster.

**Rationale**:
- No user input required
- `--target-version` flag available for override
- Handles mixed-version clusters

---

## 13. Known Issues & Tech Debt

### 13.1 High Priority

| Issue | Location | Description |
|-------|----------|-------------|
| **Package-level state** | `cli/root.go:8-14` | `ConfigFlags` is global; should pass explicitly |

### 13.2 Medium Priority

| Issue | Location | Description |
|-------|----------|-------------|
| No watcher tests | `internal/watcher/` | Migration logic untested |
| Magic numbers | `tui/view.go` | Some hardcoded widths |
| Duplicate `isNodeReady` | `watcher/nodes.go`, `stage/computer.go` | Acceptable per style guide |

### 13.3 Low Priority

| Issue | Location | Description |
|-------|----------|-------------|
| No Bubbles components | `internal/tui/` | Could use `bubbles/spinner`, `bubbles/list` |
| Mixed receiver types | `tui/update.go` | Some pointer, some value receivers |
| Error string inconsistency | Various | Some use "pkg: context", some use full sentences |

### 13.4 Future Enhancements

- PDB → Node linking (show which PDB blocks which node)
- Timing/velocity metrics on Stats screen
- Non-fatal error display (status bar)
- Bubbles library adoption

---

## 14. Architecture vs Implementation

### 14.1 Alignment Summary

| Architecture Doc (ADR) | Implementation | Status |
|------------------------|----------------|--------|
| ADR-001: Cobra CLI | `internal/cli/` | ✅ Aligned |
| ADR-002: ConfigFlags | `internal/kube/client.go` | ✅ Aligned |
| ADR-003: Context lifecycle | `internal/signals/signals.go` | ✅ Aligned |
| ADR-004: WaitGroup shutdown | `watcher/manager.go:192-194` | ✅ Aligned |
| ADR-005: Ring buffer channels | `watcher/manager.go:128-177` | ✅ Aligned |
| ADR-006: Owner-based migrations | `watcher/migrations.go` | ✅ Aligned |
| ADR-007: Stage detection | `stage/computer.go` | ✅ Aligned |
| ADR-008: Event filtering | `watcher/events.go:17-55` | ✅ Aligned |

### 14.2 Implementation Additions (Not in Original Architecture)

| Feature | Location | Notes |
|---------|----------|-------|
| PDB watcher | `watcher/pdbs.go` | Blocker detection |
| Pod watcher | `watcher/pods.go` | Full pod state tracking |
| Multiple TUI screens | `tui/view.go` | 7 screens (0-6) |
| Tokyo Night theme | `tui/styles.go` | Full color system |
| Drain progress tracking | `watcher/nodes.go:166-211` | DrainStartTime, InitialPodCount |
| Version display fix | `stage/computer.go:92-97` | LowestVersion() for "from" version |

### 14.3 Architecture Doc Gaps

The original architecture doc was written before implementation. These features were added during development:

1. **PDB/Blocker system** - Not in original ADRs
2. **Multiple screens** - Original only showed single view
3. **Drain progress** - Not detailed in original
4. **Pod state tracking** - Original focused on nodes

---

## Quick Reference Card

### File → Responsibility

| File | One-line Summary |
|------|------------------|
| `cmd/kupgrade/main.go` | Entry point, delegates to cli.Execute() |
| `cli/root.go` | Cobra root command, ConfigFlags setup |
| `cli/watch.go` | Watch command, wires everything together |
| `kube/client.go` | K8s clientset + informer factory creation |
| `signals/signals.go` | SIGTERM/SIGINT → context cancellation |
| `stage/computer.go` | **SINGLE SOURCE OF TRUTH** for node stages |
| `watcher/manager.go` | Coordinates all watchers, ring buffer channels |
| `watcher/nodes.go` | Node informer, builds NodeState |
| `watcher/pods.go` | Pod informer, builds PodState |
| `watcher/events.go` | K8s Event informer (filtered) |
| `watcher/pdbs.go` | PDB informer → Blocker detection |
| `watcher/migrations.go` | Pod delete/add correlation |
| `tui/model.go` | TUI state (Model) |
| `tui/update.go` | Message handlers (Update) |
| `tui/view.go` | Render functions (View) |
| `tui/styles.go` | Lip Gloss styles (Tokyo Night) |
| `pkg/types/*.go` | Shared type definitions |

### Data Flow Summary

```
K8s API → Informers → Watchers → Ring Buffer Channels → TUI Model → View
                         ↓
                   StageComputer (computes stages)
                   MigrationTracker (correlates pod moves)
```

### Key Interfaces

- `Watcher` - Start watching, non-blocking
- `EventEmitter` - Emit(), EmitNodeState(), EmitPodState(), EmitBlocker()
- `StageComputer` - ComputeStage(), TargetVersion(), LowestVersion()
- `MigrationTracker` - OnPodDelete(), OnPodAdd(), Cleanup()

---

*Document generated by Winston (Architect) for kupgrade codebase review.*
