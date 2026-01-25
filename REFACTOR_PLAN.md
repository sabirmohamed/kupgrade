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
