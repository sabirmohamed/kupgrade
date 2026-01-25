# K9s TUI Patterns Reference

Reference patterns from k9s (github.com/derailed/k9s) for building Kubernetes TUI applications.

## Event Display

### Filtering
- **Severity toggle** (Ctrl+Z): Switch between all events and Warning/Error only
- Filter constant: `faultsFilter = "Warning|Error"`
- Applied via regex-based matching

### Refresh Rate
- **Initial**: 300ms for quick startup
- **Normal**: 2 seconds (configurable)
- **Max retry**: 2 minutes with exponential backoff
- Key insight: NOT real-time streaming - prevents flooding

### Deduplication
- Uses "Upsert" pattern - rows updated in-place, not duplicated
- Delta tracking: computes changes between old and new rows
- Skips comparison of time columns (Age, Last Seen, First Seen)
- Marks unchanged rows as `EventUnchanged` (no visual update)

### Update Loop Protection
```go
// Prevents concurrent updates - drops redundant refreshes
if !atomic.CompareAndSwapInt32(&t.inUpdate, 0, 1) {
    slog.Debug("Dropping update...")
    return nil
}
```

## Color Coding

Row event types with colors:
- `EventAdd` → Green (new items)
- `EventUpdate` → Yellow/Orange (modified)
- `EventDelete` → Red (removed)
- `EventUnchanged` → Standard (no visual change)

## Table Rendering

### Column Detection
- Time columns auto-detected: "Last Seen", "First Seen", "Age"
- Column names uppercased: "Name" → "NAME"
- Proper alignment handling

### Sorting
- Events sorted by "LAST SEEN" descending (newest first)
- Time-based column detection for proper duration sorting

## Architecture

### Layer Separation
- **View Layer**: UI rendering and user interaction
- **Model Layer**: Data fetching and state management
- **Render Layer**: Row formatting and health checks
- **DAO Layer**: Kubernetes API access

### Key Files (for reference)
```
internal/view/event.go      - Event viewer with toggle faults
internal/render/table.go    - Generic table rendering
internal/render/ev.go       - Event-specific health checks
internal/model1/table_data.go - Row event management and filtering
internal/model1/delta.go    - Change tracking logic
internal/model/table.go     - Table model with refresh loop
```

## Lessons for kupgrade

1. **Don't stream everything**: 2-second refresh is fine, prevents flooding
2. **Filter at source**: Don't emit events you'll just filter out in the view
3. **Deduplicate updates**: Only push changes, not full state
4. **Severity filtering**: Let users toggle between all/warnings-only
5. **Color feedback**: Visual cues for add/update/delete operations
6. **Atomic updates**: Prevent concurrent refresh races
