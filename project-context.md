# Kupgrade Project Context

> **Purpose**: Critical rules and patterns for AI agents implementing code.
> **Last Updated**: 2026-01-27

---

## Tech Stack

| Component | Technology | Version |
|-----------|------------|---------|
| Language | Go | 1.22+ |
| CLI | Cobra | kubectl-compatible flags |
| TUI | Bubble Tea | Elm architecture |
| Styling | Lip Gloss | Tokyo Night theme |
| K8s Client | client-go | Informer-based |

---

## Code Style

**Primary Reference**: [Google Go Style Guide](https://google.github.io/styleguide/go/)

Local copies in `style/go/`:
- `guide.md` - Core style guide
- `decisions.md` - Style decisions
- `best-practices.md` - Best practices

### Naming Conventions

| Element | Convention | Example |
|---------|------------|---------|
| Receivers | Single letter, consistent | `m` Manager, `c` Computer, `w` Watcher |
| Getters | No `Get` prefix | `TargetVersion()` not `GetTargetVersion()` |
| Errors | `pkg: context: %w` | `fmt.Errorf("watcher: cache sync: %w", err)` |
| Constants | MixedCaps | `eventBufferSize`, `StageReady` |

### File Organization

- Files should be <300 lines, focused on single concept
- View files split by screen: `view_overview.go`, `view_nodes.go`, etc.
- Tests alongside code: `foo.go` → `foo_test.go`

---

## Architecture Rules

### Single Source of Truth

```
Stage computation: internal/stage/computer.go ONLY
```

The TUI never computes stage - it displays what the watcher sends.

### Stage Progression

```
READY → CORDONED → DRAINING → UPGRADING → COMPLETE
  │         │          │           │           │
  │         │          │           │           └─ At target version, ready, schedulable
  │         │          │           └─ Node not ready (rebooting)
  │         │          └─ Cordoned + pods being evicted
  │         └─ Unschedulable but pods still present
  └─ Normal operation
```

### Data Flow (Unidirectional)

```
Kubernetes API
      ↓
  Informers (nodes, pods, events, pdbs)
      ↓
  Watcher Manager
      ↓
  ┌─────────────────┬──────────────────┐
  │ NodeState chan  │   Event chan     │
  └────────┬────────┴────────┬─────────┘
           ↓                 ↓
        TUI Model (display only, no computation)
```

### Channel Design

- Ring buffer semantics (non-blocking)
- TUI polls channels via `tea.Cmd`
- Never block on channel send

**Buffer Sizes:**

| Channel | Size | Rationale |
|---------|------|-----------|
| `eventCh` | 100 | Event history for display |
| `nodeStateCh` | 50 | Max reasonable cluster size |
| `podStateCh` | 200 | Higher pod churn during drains |
| `blockerCh` | 50 | PDBs typically fewer |

### Interface-First Design

```go
// Interfaces in consumer package (watcher/interfaces.go)
type StageComputer interface {
    ComputeStage(node *corev1.Node) types.Stage
    SetTargetVersion(version string)
}

// Compile-time check in implementation
var _ EventEmitter = (*Manager)(nil)
```

---

## Key Interfaces

| Interface | Location | Purpose |
|-----------|----------|---------|
| `Watcher` | `watcher/interfaces.go` | Resource observer pattern |
| `EventEmitter` | `watcher/interfaces.go` | Non-blocking event emission |
| `StageComputer` | `watcher/interfaces.go` | Node stage computation |
| `MigrationTracker` | `watcher/interfaces.go` | Pod delete/add correlation |

All interfaces defined in consumer package. Implementations use compile-time checks:
```go
var _ EventEmitter = (*Manager)(nil)
```

Full interface contracts in `docs/ARCHITECTURE.md` section 7.

---

## Type Definitions

| Type | Location | Fields |
|------|----------|--------|
| `NodeState` | `pkg/types/node.go` | Name, Stage, Version, PodCount, Conditions, Taints |
| `PodState` | `pkg/types/pod.go` | Name, Namespace, NodeName, Phase, Restarts, Probes |
| `Blocker` | `pkg/types/blocker.go` | Type (PDB/PV/DaemonSet), Name, Detail |
| `Migration` | `pkg/types/migration.go` | Owner, FromNode, ToNode, OldPod, NewPod |
| `Event` | `pkg/types/event.go` | Type, Severity, Message, Timestamp |

Full type definitions in `docs/ARCHITECTURE.md` section 6.

---

## TUI Patterns

**Reference**: `style/tui/k9s-patterns.md`

### Elm Architecture

```
Model (state) → Update (reducer) → View (render)
```

### Screen Navigation

| Key | Action |
|-----|--------|
| `0-6` | Switch screens |
| `↑/↓` or `j/k` | Navigate lists |
| `Enter` | Show details |
| `?` | Help overlay |
| `q` | Back/Quit |

### Color Theme

Tokyo Night palette defined in `internal/tui/styles.go` and `style/tui/tokyo-night.json`.

---

## Architecture Decision Records (ADRs)

| ADR | Decision | Rationale |
|-----|----------|-----------|
| ADR-001 | Cobra CLI | kubectl-compatible, ConfigFlags integration |
| ADR-002 | ConfigFlags pattern | Battle-tested kubeconfig handling |
| ADR-003 | Context-based lifecycle | Modern K8s pattern, deadline support |
| ADR-004 | Ring buffer channels | Non-blocking, bounded memory, graceful degradation |
| ADR-005 | Owner-based migration tracking | Pods don't move - owner ref links related pods |
| ADR-006 | Single source of truth for stages | Prevents drift, TUI is display-only |
| ADR-007 | Auto-detect target version | Infer from highest version, `--target-version` override |

Full ADR details in `docs/ARCHITECTURE.md` section 12.

---

## Known Tech Debt

### P6: Package-Level ConfigFlags (HIGH)

**Location**: `internal/cli/root.go:8-14`

```go
var ConfigFlags *genericclioptions.ConfigFlags  // Global mutable state
```

**Fix**: Pass explicitly to commands.

### P7: Type Assertion Without Check (HIGH)

**Location**: `internal/watcher/manager.go:87-89`

```go
m.migrations.(*migrationTracker).runCleanup(ctx)  // Panics if wrong type
```

**Fix**: Use ok pattern or export in interface.

---

## Forbidden Practices

1. **No stage computation in TUI** - Watcher computes, TUI displays
2. **No unchecked type assertions** - Always use `value, ok := x.(Type)`
3. **No blocking channel operations** - Use ring buffer pattern
4. **No `Get` prefix on getters** - `Version()` not `GetVersion()`
5. **No magic numbers in views** - Extract to named constants

---

## File Locations

| Purpose | Location |
|---------|----------|
| Stage logic | `internal/stage/computer.go` |
| Watcher coordination | `internal/watcher/manager.go` |
| TUI state | `internal/tui/model.go` |
| TUI rendering | `internal/tui/view_*.go` |
| Shared types | `pkg/types/` |
| Style guides | `style/go/`, `style/tui/` |

---

## For AI Agents

When implementing changes:

1. **Read this file first** - Understand constraints before coding
2. **Check REFACTOR_PLAN.md** - See `_bmad-output/planning-artifacts/kupgrade/`
3. **Follow Google Go Style** - See `style/go/decisions.md` for specific rules
4. **Run verification** - `go build ./...` and `go vet ./...` before completing
5. **Keep TUI dumb** - Any logic changes go in watcher, not TUI
