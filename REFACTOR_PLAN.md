# Kupgrade Refactor Plan

## STATUS: ✅ COMPLETE

All refactoring steps have been implemented and verified:
- `go build ./...` ✓
- `go vet ./...` ✓

---

## Problems Identified

### P1: Duplicate Stage Logic (Critical) ✅ FIXED
- `internal/stage/computer.go:27` - ComputeStage() - SINGLE SOURCE OF TRUTH
- TUI no longer computes stage, just displays what watcher sends

### P2: Wrong Data Flow (Critical) ✅ FIXED
Implemented correct flow:
```
Watcher → NodeState channel → TUI displays (no computation)
        → Event channel → events panel only
```

### P3: Getter Naming (Effective Go) ✅ FIXED
- `GetTargetVersion()` → `TargetVersion()`
- `GetPodCount()` → `PodCount()`
- `GetNodeStates()` → `InitialNodeStates()`
- `GetServerVersion()` → `ServerVersion()`

### P4: Missing Interface Compile-Time Checks ✅ FIXED
Added: `var _ EventEmitter = (*Manager)(nil)` in manager.go

### P5: Error Strings Missing Package Prefix ✅ FIXED
- `"watcher: cache sync failed..."`
- `"kube: failed to get server version: ..."`

---

## Completed Steps

### Step 1: Add NodeState channel to Manager ✅
File: `internal/watcher/manager.go`
- Added `nodeStateCh chan types.NodeState`
- Added `NodeStateUpdates() <-chan types.NodeState` method
- Added `EmitNodeState()` with ring buffer semantics

### Step 2: Update NodeWatcher to emit NodeState ✅
File: `internal/watcher/nodes.go`
- `buildState()` creates full NodeState with computed stage
- `onAdd/onUpdate/onDelete` emit NodeState
- Events are for display only (events panel)

### Step 3: Update TUI to receive NodeState ✅
File: `internal/tui/model.go`
- Added `Config` struct with all channels
- Added `waitForNodeState()` cmd
- Removed `computeStage()` - no longer needed

File: `internal/tui/update.go`
- `handleNodeUpdate()` just stores state
- `handleEvent()` only adds to events list

File: `internal/cli/watch.go`
- Passes both channels to TUI via Config struct

### Step 4: Rename Getters ✅
- `internal/stage/computer.go`: TargetVersion(), PodCount()
- `internal/watcher/interfaces.go`: Updated interface
- `internal/kube/version.go`: ServerVersion()
- `internal/watcher/manager.go`: InitialNodeStates()

### Step 5: Add Interface Compile-Time Checks ✅
File: `internal/watcher/manager.go`
```go
var _ EventEmitter = (*Manager)(nil)
```

### Step 6: Add Package Prefix to Errors ✅
All error messages now prefixed with package name.

### Step 7: Cleanup ✅
- Removed `NodeVersion` from Event type
- Added `Deleted` field to NodeState for delete handling
- Verified with `go vet ./...`

---

## Files Modified

1. `internal/watcher/interfaces.go` - added EmitNodeState, renamed getters
2. `internal/watcher/manager.go` - added nodeStateCh, interface check
3. `internal/watcher/nodes.go` - emit NodeState, buildState()
4. `internal/stage/computer.go` - renamed getters
5. `internal/tui/model.go` - Config struct, waitForNodeState
6. `internal/tui/update.go` - simplified handlers
7. `internal/tui/view.go` - use accessor methods
8. `internal/cli/watch.go` - wire up channels via Config
9. `internal/kube/version.go` - renamed ServerVersion()
10. `pkg/types/node.go` - added Deleted field
11. `pkg/types/event.go` - removed NodeVersion field

---

## Next Steps

Test with a real cluster:
```bash
./kupgrade watch --context <your-context>
```

Verify:
- Nodes appear in correct stages
- Stage transitions work during upgrade
- Events stream in events panel
- Progress bar updates correctly

---

## Practical Go Review (2025-01-25)

Code review against Dave Cheney's Practical Go principles.

### What's Good ✅

| Principle | Status | Location |
|-----------|--------|----------|
| Package main is small | ✅ | `cmd/kupgrade/main.go` - 15 lines, delegates to `cli.Execute()` |
| Return early pattern | ✅ | Guard clauses throughout, e.g., `computer.go:68` |
| Good interface design | ✅ | `watcher/interfaces.go` - small, focused interfaces |
| Consistent naming style | ✅ | Receivers: `m` Manager, `c` Computer, `w` Watcher |
| Zero value useful | ✅ | Maps initialized in constructors |
| Error wrapping with %w | ✅ | Consistent use of `fmt.Errorf("pkg: %w", err)` |

### Issues Identified

#### P6: Package-Level State (§4.5) 🔴 TODO
**File:** `internal/cli/root.go:8-14`
```go
var (
    ConfigFlags *genericclioptions.ConfigFlags
    Version = "dev"
)
```
**Problem:** Global mutable state creates tight coupling. `ConfigFlags` is set in `NewRootCmd()` and used in `runWatch()`.

**Fix:** Pass `ConfigFlags` explicitly to commands:
```go
func NewWatchCmd(configFlags *genericclioptions.ConfigFlags) *cobra.Command
```

#### P7: Type Assertion Without Check (§7.2) 🔴 TODO
**File:** `internal/watcher/manager.go:87-89`
```go
m.migrations.(*migrationTracker).runCleanup(ctx)
```
**Problem:** Panics if type doesn't match.

**Fix Options:**
1. Export `RunCleanup(ctx)` in `MigrationTracker` interface
2. Use type assertion with ok check: `if t, ok := m.migrations.(*migrationTracker); ok { ... }`

#### P8: Missing Why Comments (§3) 🟡 MINOR
**File:** `internal/stage/computer.go:40-41`
```go
upgradeActive := lowest != "" && target != "" && lowest != target
```
**Problem:** The *what* is clear but not *why* this detection matters.

**Fix:** Add context:
```go
// upgradeActive detects when cluster has mixed versions (upgrade in progress).
// Only show COMPLETE stage during active upgrades to indicate nodes that
// have finished upgrading to target version.
upgradeActive := lowest != "" && target != "" && lowest != target
```

#### P9: Duplicate isNodeReady Function 🟢 ACCEPTABLE
**Files:** `watcher/nodes.go:188` and `stage/computer.go:123`

Both packages have identical 8-line `isNodeReady()` function. Per §4.2 (avoid util packages), keeping it duplicated is acceptable for small functions to avoid coupling.

### Uncommitted Changes

The following fixes from AKS upgrade testing are uncommitted:

1. **COMPLETE stage fix** - `internal/stage/computer.go`
   - Added `lowestVersion` field to track version variance
   - COMPLETE only shows during active upgrades (mixed versions)

2. **Target version detection** - `internal/watcher/nodes.go`
   - Now passes actual node version to `SetTargetVersion()` on add/update

### Action Items

| Priority | Issue | Effort | Status |
|----------|-------|--------|--------|
| High | P6: Remove package-level ConfigFlags | Medium | TODO |
| High | P7: Fix type assertion in manager.go | Low | TODO |
| Low | P8: Add why comments | Low | TODO |
| - | P9: Duplicate isNodeReady | - | Acceptable |
| High | Commit COMPLETE stage fix | Low | PENDING TEST |

---

## Google Go Style Guide Review (2025-01-25)

Code review against [Google's Go Style Guide](https://google.github.io/styleguide/go/).

### Compliance Summary ✅

| Category | Status | Notes |
|----------|--------|-------|
| Naming (MixedCaps) | ✅ | `NewManager`, `NodeState`, `ComputeStage` |
| Package Names | ✅ | `cli`, `watcher`, `tui`, `types`, `stage`, `kube` |
| Receiver Names | ✅ | Short & consistent: `m`, `c`, `w` |
| Getter Naming | ✅ | No `Get` prefix: `TargetVersion()`, `PodCount()` |
| Constants | ✅ | MixedCaps: `eventBufferSize`, `StageReady` |
| Doc Comments | ✅ | All exported types/functions documented |
| Error Strings | ✅ | Lowercase, no punctuation |
| Error Wrapping | ✅ | Consistent `%w` usage |
| Context First | ✅ | `Start(ctx context.Context)` |
| Channel Direction | ✅ | `Events() <-chan types.Event` |
| Interface Design | ✅ | In consumer package, concrete returns |
| Small main | ✅ | 15 lines, delegates to `cli.Execute()` |
| Goroutine Lifetimes | ✅ | WaitGroup + context cancellation |

### Issues Identified

#### G1: Package-Level Flags 🔴 TODO
**Location:** `internal/cli/root.go:14` and `watch.go:14`
```go
var ConfigFlags *genericclioptions.ConfigFlags
var targetVersion string
```
**Google says:** "Define flags only in `package main`"

**Same as P6** - Pass ConfigFlags explicitly to commands.

#### G2: Type Assertion Without Check 🔴 TODO
**Location:** `internal/watcher/manager.go:88`
```go
m.migrations.(*migrationTracker).runCleanup(ctx)
```
**Google says:** Type assertions should use the ok pattern or be demonstrably safe.

**Same as P7** - Export `RunCleanup` in interface or use ok check.

#### G3: Error String Inconsistency 🟡 MINOR
**Location:** `internal/kube/client.go:23-28`
```go
return nil, fmt.Errorf("failed to create rest config: %w", err)
```
**vs other files:**
```go
return fmt.Errorf("cli: %w", err)
```
**Issue:** Inconsistent error prefix style across packages.

**Fix:** Standardize to `pkg: context` pattern:
```go
return nil, fmt.Errorf("kube: rest config: %w", err)
```

#### G4: Redundant Error Context 🟢 MINOR
**Location:** `internal/cli/watch.go:65`
```go
return fmt.Errorf("cli: TUI error: %w", err)
```
**Issue:** "TUI error" may duplicate context from underlying error.

**Fix:** Simplify to `fmt.Errorf("cli: %w", err)` if bubbletea errors self-describe.

### What's Done Right

1. **Interface Design** - `StageComputer` in consumer package (watcher), implemented by producer (stage)
2. **Concrete Returns** - `NewManager() *Manager`, `New() *Computer`
3. **Compile-time Check** - `var _ EventEmitter = (*Manager)(nil)`
4. **Import Alias** - `tea "github.com/charmbracelet/bubbletea"` - community convention
5. **Nil Slice Handling** - Using `len(s) == 0` checks
6. **Goroutine Discipline** - Every goroutine has clear exit via context/WaitGroup

### Consolidated Action Items

| Priority | Issue | Source | Effort | Status |
|----------|-------|--------|--------|--------|
| High | Package-level flags | P6/G1 | Medium | TODO |
| High | Type assertion check | P7/G2 | Low | TODO |
| Low | Why comments | P8 | Low | TODO |
| Low | Error string consistency | G3 | Low | TODO |
| - | Duplicate isNodeReady | P9 | - | Acceptable |
| - | Redundant error context | G4 | - | Optional |

### Verdict

Code is **highly compliant** with both Practical Go and Google Style Guide. Only two real issues (P6/G1, P7/G2) appear in both reviews. Fix when naturally touching related code.

---

## Style Guide Comparison & Recommendation (2025-01-25)

Analysis of alignment between three Go style guides to determine primary reference.

### The Three Guides

| Guide | Year | Author | Purpose |
|-------|------|--------|---------|
| [Effective Go](https://go.dev/doc/effective_go) | 2009 | Go Team | Language idioms (the "what") |
| [Practical Go](https://dave.cheney.net/practical-go/presentations/qcon-china.html) | 2019 | Dave Cheney | Design principles (the "why") |
| [Google Go Style Guide](https://google.github.io/styleguide/go/) | 2022+ | Google | Explicit rules (the "how") |

### Full Agreement (All Three) ✅

| Topic | Status |
|-------|--------|
| MixedCaps naming | ✅ Universal |
| No Get prefix on getters | ✅ Universal |
| Package names lowercase | ✅ Universal |
| Avoid util/common/base packages | ✅ Universal |
| Doc comments on exports | ✅ Universal |
| gofmt formatting | ✅ Universal |
| Return early (guard clauses) | ✅ Universal |
| Zero value useful | ✅ Universal |
| Interface naming (-er suffix) | ✅ Universal |

### Where Guides Differ

| Topic | Effective Go | Practical Go | Google | Winner |
|-------|:------------:|:------------:|:------:|--------|
| Package-level state | 🟡 Silent | ❌ Avoid | ❌ Avoid | Practical/Google |
| Error string case | 🟡 Silent | 🟡 Silent | ✅ Lowercase | Google |
| Type assertion checks | 🟡 Shows both | 🟡 Implied | ✅ Explicit | Google |
| Interface location | 🟡 Silent | ✅ Consumer | ✅ Consumer | Practical/Google |
| Named return params | ✅ Encouraged | 🟡 Silent | ⚠️ Restrictive | Effective Go |

### Reference: Kubernetes Style

We analyzed the [Kubernetes codebase](https://github.com/kubernetes/kubernetes) which uses a **pragmatic hybrid approach** (70% Google, 15% Effective Go, 15% custom conventions). Kubernetes intentionally relaxes many rules due to legacy code and scale (e.g., allows underscores in conversion functions like `Convert_v1_Pod_To_v2_Pod`).

For kupgrade, we stick with **Google Go Style Guide** because:
- New codebase with no legacy constraints
- Smaller scale allows stricter enforcement
- Cleaner to follow one standard than pragmatic exceptions

### Why Google Wins

1. **Most comprehensive** - covers everything the others do, plus more
2. **Explicit rules** - no ambiguity (e.g., "error strings must be lowercase")
3. **Actively maintained** - reflects modern Go (generics, `any`, context patterns)
4. **Backed by scale** - battle-tested at Google's codebase size
5. **Aligns with Practical Go** - on the issues that matter (P6/G1, P7/G2)

The two issues found in kupgrade (package-level state, unchecked type assertion) are:
- **Silent in Effective Go** (2009, predates modern thinking)
- **Flagged by both Practical Go and Google** (modern consensus)

### Recommendation

**Primary Reference:** [Google Go Style Guide](https://google.github.io/styleguide/go/)

**Supplementary:**
- [Practical Go](https://dave.cheney.net/practical-go/presentations/qcon-china.html) - for understanding *why* patterns matter
- [Effective Go](https://go.dev/doc/effective_go) - for language mechanics reference

### Practical Workflow

```
┌─────────────────────────────────────────────────────┐
│  Writing Code                                       │
│  └── Follow Google Style Guide (Style Decisions)   │
│                                                     │
│  Understanding Why                                  │
│  └── Consult Practical Go                          │
│                                                     │
│  Language Mechanics                                 │
│  └── Consult Effective Go                          │
│                                                     │
│  Code Reviews                                       │
│  └── Cite Google Style Guide as authority          │
└─────────────────────────────────────────────────────┘
```

### For CONTRIBUTING.md (Future)

```markdown
## Code Style

This project follows the [Google Go Style Guide](https://google.github.io/styleguide/go/).

Key references:
- [Style Decisions](https://google.github.io/styleguide/go/decisions) - normative rules
- [Best Practices](https://google.github.io/styleguide/go/best-practices) - recommended patterns

For deeper understanding of Go design principles, see:
- [Practical Go](https://dave.cheney.net/practical-go/presentations/qcon-china.html) by Dave Cheney
```

---

## Recent Implementation Progress (2025-01-25)

### Completed This Session ✅

| Feature | Files Modified | Notes |
|---------|----------------|-------|
| Omarchy Tokyo Night theme | `internal/tui/styles.go` | Full color palette with layered backgrounds, text hierarchy, ANSI colors |
| PODS screen | `internal/tui/view.go` | Real-time pod table with namespace, status, restarts, probes, owner |
| RESTARTS column enhancement | `internal/watcher/pods.go`, `pkg/types/pod.go` | Shows "27 4m" format (count + time since last restart) |
| Two-row footer | `internal/tui/view.go` | Row 1: context hints, Row 2: screen navigation |
| Keyboard simplification | `internal/tui/keys.go` | Removed PgUp/PgDn, kept Ctrl+U/D for page navigation |
| Column width caps | `internal/tui/view.go` | Prevents excessive spacing on wide terminals |
| computePodStatus() | `internal/watcher/pods.go` | k9s-style detailed status (CrashLoopBackOff, Init:Error, Terminating) |

### Key Code Additions

**getRestartInfo()** - `internal/watcher/pods.go:283-318`
```go
func getRestartInfo(pod *corev1.Pod) (int, string) {
    // Returns restart count + age since most recent restart
    // Checks both regular and init container lastState.terminated.finishedAt
}
```

**PodState.LastRestartAge** - `pkg/types/pod.go:13`
```go
LastRestartAge  string // e.g., "4m", "8h" - empty if no restarts
```

---

## BMAD Team Review (2025-01-25)

Multi-agent review of commits and architecture.

### Team Feedback Summary

| Agent | Role | Assessment |
|-------|------|------------|
| Winston (Architect) | Architecture | ✅ Clean and scalable - proper event-driven design, single source of truth |
| Sally (UX Designer) | User Experience | ✅ Serves real operators - meaningful information density |
| Murat (Test Architect) | Test Coverage | ⚠️ Critical gap - no watcher/TUI tests |
| Mary (Analyst) | Requirements | ⚠️ E2 BLOCKERS should be prioritized |

### Key Recommendations

1. **Add watcher tests** - Migration tracking logic and pod state computations need unit tests
2. **Prioritize BLOCKERS screen** - Most valuable for upgrade monitoring (E2 in OBSERVABILITY_PLAN.md)
3. **Add PDB detection** - Pod Disruption Budgets are critical for safe upgrades
4. **Interface for testability** - Extract channel dependencies behind interfaces

---

## Priority Roadmap (Post-Review)

### Immediate Priority: BLOCKERS Screen (E2)

The BLOCKERS screen is the most operationally valuable feature for upgrade monitoring.

**Planned Implementation:**
1. PDB (Pod Disruption Budget) violations
2. Node taints preventing scheduling
3. Resource constraints blocking eviction
4. Critical pods that can't be evicted

**Files to modify:**
- `internal/watcher/` - New blocker detection logic
- `internal/tui/view.go` - BLOCKERS screen rendering
- `pkg/types/` - BlockerState type

### Priority Queue

| # | Item | Type | Effort | Rationale |
|---|------|------|--------|-----------|
| 1 | **BLOCKERS screen + PDB detection** | Feature | Medium | Most valuable for upgrades - answers "why isn't my upgrade progressing?" |
| 2 | P6/G1: Package-level ConfigFlags | Refactor | Medium | Google Style violation, affects testability |
| 3 | P7/G2: Type assertion without check | Refactor | Low | Panic risk in manager.go |
| 4 | T7: Basic watcher tests | Tests | Medium | Migration logic untested |
| 5 | T7: Basic TUI tests | Tests | Medium | Navigation and state untested |
| 6 | T2: Extract magic numbers | Cleanup | Low | Maintainability |
| 7 | T3: Migrate to Bubbles | Enhancement | Medium | Reuse Charm components |

### Deferred (Nice to Have)

- T1: Component abstraction
- T4: Consistent receiver types
- T5: Status bar for non-fatal errors
- T6: Interface for channel testability
- P8: Why comments

---

## TUI Implementation Review (2025-01-25)

Analysis of Bubble Tea and Lip Gloss usage in `internal/tui/`.

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      BUBBLE TEA (Elm Architecture)          │
│                                                             │
│  model.go ──▶ update.go ──▶ view.go ──▶ styles.go          │
│  (State)      (Reducer)     (Render)    (Lip Gloss)        │
│                                                             │
│  Message Types:                                             │
│  - tea.KeyMsg (keyboard)    - NodeUpdateMsg (from watcher) │
│  - tea.WindowSizeMsg        - EventMsg (from watcher)      │
│  - TickMsg (1s timer)       - SpinnerMsg (500ms animation) │
└─────────────────────────────────────────────────────────────┘
```

### What's Good ✅

| Aspect | Implementation | Why It's Good |
|--------|----------------|---------------|
| Clean separation | `model.go`, `update.go`, `view.go`, `styles.go` | Single responsibility per file |
| Elm Architecture | Model → Update → View cycle | Predictable state, easy to debug |
| Channel bridge | `waitForNodeState()` returns `tea.Cmd` | Idiomatic external data integration |
| Style reuse | `nodeCardBase.Copy().Border(...)` | DRY - base styles extended |
| Responsive layout | `nodeCardWidth()`, `panelWidths()` | Adapts to terminal size |
| Vim keybindings | `h/j/k/l` alongside arrows | Power user friendly |
| View modes | `ViewOverview`, `ViewNodeDetail`, `ViewHelp` | Clean state machine |
| Color semantics | `stageColors` map | Consistent visual language |
| Non-blocking | Ring buffer channels | TUI never freezes |

### What's Bad / Could Improve 🔴

#### T1: No Component Abstraction 🟡 FUTURE
**Location:** All render functions are methods on Model

```go
// Current: monolithic
func (m Model) renderNodeCard(...) string { ... }
func (m Model) renderEventsPanel(...) string { ... }
```

**Better (Bubble Tea components):**
```go
type NodeCard struct {
    node     types.NodeState
    selected bool
}
func (c NodeCard) View() string { ... }
```

#### T2: Hardcoded Magic Numbers 🟡 FUTURE
**Location:** `styles.go:122`, `view.go:259`

```go
eventTypeStyle = lipgloss.NewStyle().Width(14)  // Magic number
maxMsgLen := width - 18                          // Magic number
```

**Fix:** Extract to named constants:
```go
const (
    eventTypeWidth    = 14
    eventPaddingTotal = 18
)
```

#### T3: No Bubbles Library Usage 🟡 FUTURE
Reinventing wheels that [Bubbles](https://github.com/charmbracelet/bubbles) provides:

| Current Implementation | Bubbles Equivalent |
|-----------------------|-------------------|
| `spinnerFrames` + manual tick | `spinner.Model` |
| `matchKey()` + `keyMap` | `key.Binding` + `help.Model` |
| Manual progress bar | `progress.Model` |
| Manual list navigation | `list.Model` |

#### T4: Mixed Receiver Types 🟡 FUTURE
**Location:** `update.go`

```go
// Pointer receiver mutates directly
func (m *Model) handleNodeUpdate(node types.NodeState) {
    m.nodes[node.Name] = node  // Direct mutation
}
```

**Better (Bubble Tea style - value receiver, return new model):**
```go
func (m Model) handleNodeUpdate(node types.NodeState) Model {
    m.nodes = maps.Clone(m.nodes)
    m.nodes[node.Name] = node
    return m
}
```

#### T5: No Error Display for Non-Fatal Errors 🟡 FUTURE
**Location:** `view.go:12-14`

```go
// Only fatal errors shown
if m.fatalError != nil {
    return fmt.Sprintf("Error: %v\n", m.fatalError)
}
```

**Problem:** Non-fatal errors silently ignored.

**Fix:** Add `statusMessage` field for temporary notifications/warnings.

#### T6: Tight Coupling to Channels 🟡 FUTURE
**Location:** `model.go` Config struct

```go
type Config struct {
    EventCh     <-chan types.Event      // Direct channel dependency
    NodeStateCh <-chan types.NodeState
}
```

**Better (interface for testability):**
```go
type EventSource interface {
    Events() <-chan types.Event
}
```

#### T7: No TUI Tests 🔴 TODO
**Location:** No `*_test.go` files in `internal/tui/`

Bubble Tea models are easily testable:
```go
func TestNodeNavigation(t *testing.T) {
    m := New(Config{...})
    m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
    if m.selectedStage != 1 {
        t.Errorf("expected stage 1, got %d", m.selectedStage)
    }
}
```

### TUI Quality Score

| Category | Score | Notes |
|----------|-------|-------|
| Architecture | 8/10 | Clean Elm pattern, good file separation |
| Styling | 7/10 | Good color system, some magic numbers |
| Responsiveness | 8/10 | Adapts to terminal, good layout calc |
| Reusability | 4/10 | No components, no Bubbles usage |
| Testability | 3/10 | No tests, channel coupling |
| Maintainability | 6/10 | Readable but needs components |

### TUI Action Items

| Priority | Issue | Effort | Status |
|----------|-------|--------|--------|
| High | T7: Add basic TUI tests | Medium | TODO |
| Medium | T2: Extract magic numbers | Low | TODO |
| Medium | T3: Use Bubbles spinner/key | Low | FUTURE |
| Low | T1: Extract components | Medium | FUTURE |
| Low | T5: Add status bar | Low | FUTURE |
| Low | T4: Consistent receiver types | Medium | FUTURE |
| Low | T6: Interface for testability | Medium | FUTURE |

### Bubbles Migration Guide (Future)

When ready to adopt Bubbles:

```go
import (
    "github.com/charmbracelet/bubbles/spinner"
    "github.com/charmbracelet/bubbles/key"
    "github.com/charmbracelet/bubbles/help"
)

type Model struct {
    spinner  spinner.Model  // Replace manual spinnerFrames
    keys     keyMap         // Use key.Binding
    help     help.Model     // Auto-generate help text
}
```

Benefits:
- Built-in accessibility
- Consistent behavior across Charm apps
- Less code to maintain
- Better keyboard handling

---

## Overview Screen UX Refactor (2025-01-25)

### Design Principle

> Design for the anxious on-call engineer at 2am who needs to know "is this stuck and why" in under 2 seconds.

### Current Problems

1. **Empty columns waste 80% of screen** - Four "(empty)" boxes showing nothing
2. **Node boxes too tall** - 3 lines per node when 1 would suffice
3. **Horizontal layout doesn't scale** - Eye movement across 5 columns
4. **Blockers buried** - Most critical info competing with useless panels
5. **Information by structure, not priority** - Organized by stages, not user questions

### User Questions (In Priority Order)

1. Is anything broken/blocked? → **Blockers at top**
2. What's the overall progress? → **Pipeline summary**
3. What's actively happening? → **Drain detail (when relevant)**
4. What are the details? → **Node list, events**

---

### Target States

#### Scenario 1: Idle State (Pre-Upgrade)

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│ ★ kupgrade  testbed-aks  v1.32.1  ─────────────────────── 0%      22:58:42      │
├─────────────────────────────────────────────────────────────────────────────────┤
│    READY    →    CORDONED    →    DRAINING    →    UPGRADING    →    COMPLETE   │
│      5              0               0                0                 0        │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ⚠ BLOCKERS (2)                                                                  │
│ PDB default/critical-service-pdb    maxUnavailable=0     → 0 evictions allowed  │
│ PDB default/test-app-pdb-strict     minAvailable=100%    → 0 evictions allowed  │
├─────────────────────────────────────────────────────────────────────────────────┤
│ NODES: READY (5)                                              ↑↓ navigate       │
│ ► aks-stdpool-84629564-vms1      10 pods   v1.32.1                              │
│   aks-stdpool-84629564-vms2      11 pods   v1.32.1                              │
│   aks-stdpool-84629564-vms3      10 pods   v1.32.1                              │
│   ntpool-84629564-vmss000000     21 pods   v1.32.1                              │
│   ntpool-84629564-vmss000001     19 pods   v1.32.1                              │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ● EVENTS (LATEST)                                                               │
│ 22:53:42  ● Pod default/probe-test-liveness-fail Ready on vmss000001            │
│ 22:49:28  ● 5 nodes discovered                                                  │
│ 22:49:28  ● 2 blocking PDBs detected                                            │
├─────────────────────────────────────────────────────────────────────────────────┤
│ 0 overview  1 nodes  2 drains  3 pods  4 blockers  5 events  ? help  q quit     │
└─────────────────────────────────────────────────────────────────────────────────┘
```

#### Scenario 2: Active Upgrade (Mid-Drain)

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│ ★ kupgrade  testbed-aks  v1.32.1 → v1.33.0  ████████───── 40%     23:12:07      │
├─────────────────────────────────────────────────────────────────────────────────┤
│    READY    →    CORDONED    →    DRAINING    →    UPGRADING    →    COMPLETE   │
│      2              1               1                0                 1        │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ⚠ BLOCKERS (1) - BLOCKING DRAIN ON VMS2                                         │
│ PDB default/critical-service-pdb    maxUnavailable=0     → cannot evict nginx-x │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ◐ DRAINING: AKS-STDPOOL-84629564-VMS2                                           │
│ ████████████░░░░░░░░  12/21 pods evicted    Elapsed: 4m 23s                     │
│ Waiting on: nginx-deployment-7fb96c846b-xxx  app-backend-5d4f8c9b7-yyy          │
├─────────────────────────────────────────────────────────────────────────────────┤
│ NODES (5)                                                  ↑↓ navigate          │
│   aks-stdpool-84629564-vms1      10 pods   v1.32.1    READY                     │
│ ► aks-stdpool-84629564-vms2       9 pods   v1.32.1    DRAINING                  │
│   aks-stdpool-84629564-vms3      14 pods   v1.32.1    CORDONED                  │
│   ntpool-84629564-vmss000000     21 pods   v1.33.0    COMPLETE                  │
│   ntpool-84629564-vmss000001     19 pods   v1.32.1    READY                     │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ● EVENTS (LATEST)                                                               │
│ 23:12:05  ▲ Eviction blocked by PDB critical-service-pdb                        │
│ 23:11:42  ● Pod app-frontend-xxx migrated to vms1                               │
│ 23:11:38  ● Node vms2 drain started                                             │
├─────────────────────────────────────────────────────────────────────────────────┤
│ 0 overview  1 nodes  2 drains  3 pods  4 blockers  5 events  ? help  q quit     │
└─────────────────────────────────────────────────────────────────────────────────┘
```

#### Scenario 3: Upgrade Complete

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│ ★ kupgrade  testbed-aks  v1.33.0  ████████████████████ 100%       23:47:19      │
├─────────────────────────────────────────────────────────────────────────────────┤
│    READY    →    CORDONED    →    DRAINING    →    UPGRADING    →    COMPLETE   │
│      0              0               0                0                 5        │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ✓ ALL NODES UPGRADED                                       Duration: 48m 37s    │
├─────────────────────────────────────────────────────────────────────────────────┤
│ NODES (5)                                                                       │
│   aks-stdpool-84629564-vms1      12 pods   v1.33.0    ✓                         │
│   aks-stdpool-84629564-vms2      11 pods   v1.33.0    ✓                         │
│   aks-stdpool-84629564-vms3      10 pods   v1.33.0    ✓                         │
│   ntpool-84629564-vmss000000     21 pods   v1.33.0    ✓                         │
│   ntpool-84629564-vmss000001     17 pods   v1.33.0    ✓                         │
├─────────────────────────────────────────────────────────────────────────────────┤
│ ● EVENTS (LATEST)                                                               │
│ 23:47:19  ★ Upgrade complete! All 5 nodes now running v1.33.0                   │
│ 23:47:15  ● Node vms1 upgraded to v1.33.0                                       │
│ 23:42:08  ● Node vmss000001 upgraded to v1.33.0                                 │
├─────────────────────────────────────────────────────────────────────────────────┤
│ 0 overview  1 nodes  2 drains  3 pods  4 blockers  5 events  ? help  q quit     │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

### Implementation Phases

#### Phase 1: Layout Foundation ✅ COMPLETE

**Goal**: Replace column layout with vertical flow

| Task | File | Effort | Status |
|------|------|--------|--------|
| Compact pipeline header (counts + arrows) | `view.go` | 30 min | ✅ `renderCompactHeader()` + `renderPipelineRow()` |
| Blockers section (full width, top priority) | `view.go` | 30 min | ✅ `renderBlockersSection()` |
| Unified node list (stage as column) | `view.go` | 45 min | ✅ `renderNodeList()` |
| Simplified events panel | `view.go` | 20 min | ✅ `renderEventsSection()` |
| Remove old column rendering | `view.go` | 15 min | ⏳ Kept as `renderNodeColumns()` per rollback plan |

**Navigation changes (implemented):**
- ←/→ arrows: Highlight stage in pipeline header (nodes always visible)
- ↑/↓ arrows: Navigate unified node list via `listIndex`
- Enter: Show node detail overlay

**Code fixes (2025-01-26):**
- Consolidated duplicate `getSortedNodesFor*()` into shared `getSortedNodeList()` in model.go
- Fixed `selectedNodeState()` to use `listIndex` instead of old `selectedNode`
- Removed unused `sort` import from update.go

#### Phase 2: Drain Progress 🟡 PARTIAL

**Goal**: Show real-time drain status

| Task | File | Effort | Status |
|------|------|--------|--------|
| Track pods evicted per node | `watcher/pods.go` | 45 min | TODO |
| Track drain start time | `watcher/nodes.go` | 30 min | ✅ Field added to NodeState |
| Identify waiting pods (blocked by PDB) | `watcher/pdbs.go` | 1 hr | TODO |
| Render drain progress panel | `view.go` | 45 min | ✅ `renderDrainProgressSection()` |

**New data fields (added to NodeState):**
- ✅ `NodeState.DrainStartTime time.Time`
- ✅ `NodeState.WaitingPods []string`
- TODO: `NodeState.PodsEvicted int` (currently calculated from InitialPodCount - PodCount)

#### Phase 3: Contextual Intelligence (Future)

**Goal**: Smart, contextual information display

| Task | Effort | Status |
|------|--------|--------|
| Detect upgrade in progress (version change) | 30 min | ✅ Done in header |
| Version transition in header (v1.32.1 → v1.33.0) | 20 min | ✅ Done in `renderCompactHeader()` |
| Success state detection | 30 min | TODO |
| Duration tracking | 30 min | TODO |
| Contextual blocker messages ("BLOCKING DRAIN ON X") | 45 min | TODO |

---

### Files to Modify

#### Phase 1
- `internal/tui/view.go` - Main rendering changes
- `internal/tui/model.go` - May need state adjustments
- `internal/tui/update.go` - Navigation logic if changed

#### Phase 2
- `internal/watcher/nodes.go` - Drain tracking
- `internal/watcher/pods.go` - Eviction counting
- `pkg/types/node.go` - New fields
- `internal/tui/view.go` - Drain panel

#### Phase 3
- `internal/watcher/manager.go` - Upgrade detection
- `internal/tui/model.go` - Upgrade state
- `internal/tui/view.go` - Conditional rendering

---

### Success Criteria

1. **2-second test**: Can identify "stuck and why" in under 2 seconds
2. **Terminal size**: Works well in 80x24 minimum
3. **Scale test**: Handles 50+ nodes without breaking layout
4. **Empty state**: No wasted space when stages are empty
5. **Information hierarchy**: Most critical info at top

---

### Rollback Plan

Keep current `renderOverviewScreen()` as `renderOverviewScreenLegacy()` until new layout is validated.

---

### Open Questions

1. ~~Should ←/→ filter the node list by stage, or show all nodes always?~~ **RESOLVED**: Show all nodes always, ←/→ highlights stage in pipeline
2. Should we add a "compact mode" toggle for very large clusters?
3. Do we need keyboard shortcut to jump to draining node?

---

### Future Ideas (Brainstorm)

**PDB → Node Linking**
- Currently: Blockers and draining nodes shown side-by-side (user infers connection)
- Future: Explicitly link "PDB X is blocking Node Y"
- Requires: Match PDB label selectors to pods on draining nodes
- Complexity: Medium - need to join PDB selector → Pod labels → Pod nodeName
- Note: Kubernetes doesn't emit events for PDB blocks (only 429 API errors)
