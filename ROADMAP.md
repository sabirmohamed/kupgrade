# kupgrade Roadmap

> **Last Updated**: 2026-01-29

---

## Current State (v0.x — Pre-release)

Working TUI with 7 screens, real-time Kubernetes upgrade monitoring, lipgloss/table rendering, Tokyo Night theme. Tested on Ghostty against live AKS clusters.

---

## Next: Polish & Describe Feature

### Tweaks & Fixes

- [ ] Apple Terminal scrolling/color degradation — investigate or document as unsupported
- [ ] Review screen usage — do Stats (6) and Blockers (4) screens pull their weight?
- [ ] Graceful color fallback for non-TrueColor terminals

### Describe Feature (Enter/Click on resource)

Add `kubectl describe`-style detail overlays when selecting a node, pod, or event:

- **Node detail** — Enter on a node in Nodes/Overview screen shows: conditions, taints, labels, allocatable resources, running pods, version, age, stage history
- **Pod detail** — Enter on a pod in Pods screen shows: container statuses, restart reasons, probe config, resource requests/limits, events, owner chain
- **Event detail** — Enter on an event in Events screen shows: full message (untruncated), involved object, source component, first/last seen, count

Implementation approach:
- Overlay panel (like existing Help overlay) rendered on top of current screen
- Data sourced from existing watcher state where possible (no extra API calls)
- For richer detail, optional `kubectl describe`-equivalent API calls on demand
- Escape/q closes overlay, returns to list

---

## Release & Distribution

### GoReleaser + GitHub Releases

- [ ] Add `.goreleaser.yaml` — cross-compile for linux/darwin/windows (amd64, arm64)
- [ ] Add `.github/workflows/release.yml` — build on tag push
- [ ] Decide public vs private repo (public preferred — zero auth friction)
- [ ] Tag `v0.1.0` first release

### Homebrew Tap

- [ ] Create `sabirmohamed/homebrew-tap` repo
- [ ] Configure GoReleaser to auto-publish Homebrew formula
- [ ] Colleagues install via `brew tap sabirmohamed/tap && brew install kupgrade`

### Additional Package Managers (stretch)

- [ ] Scoop bucket for Windows
- [ ] Krew plugin (`kubectl upgrade watch`)

---

## Future Ideas (Not Committed)

- PDB to Node linking — show which PDB blocks which specific node
- Timing/velocity metrics on Stats screen — pods/min eviction rate, estimated completion
- Non-fatal error display in status bar
- Log export — dump events/state to file on exit
- Multi-cluster support — watch multiple contexts simultaneously
