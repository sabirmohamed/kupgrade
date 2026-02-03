# kupgrade

Watch your Kubernetes cluster while it upgrades. One binary, one command, everything you need to see.

[![Go Report Card](https://goreportcard.com/badge/github.com/sabirmohamed/kupgrade)](https://goreportcard.com/report/github.com/sabirmohamed/kupgrade)
[![golangci-lint](https://img.shields.io/badge/golangci--lint-A+-brightgreen?logo=go)](https://github.com/sabirmohamed/kupgrade/actions/workflows/go.yml)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev/)

<!-- SCREENSHOT: overview of the TUI during a live upgrade (docs/images/overview.png) -->
![kupgrade overview](docs/images/overview.png)

---

## Why

When nodes start draining, you're switching between `kubectl get nodes -o wide`, `kubectl get pods -A`, and `kubectl get events` across terminals — or flipping through k9s views trying to keep up.

If you're managing a handful of clusters and upgrading them by hand, kupgrade gives you one screen with the full picture and lets you drill into details when you need them.

Real-time updates via Kubernetes informers — no polling

---

## What it does

Three commands for three phases:

```bash
# Before: is my cluster ready?
kupgrade check

# During: what's happening right now?
kupgrade watch

# After: what broke?
kupgrade report
```

### `kupgrade watch` — live upgrade monitoring

Point it at your cluster and see what's happening: which nodes are draining, which PDBs are blocking, where pods are moving.

<!-- SCREENSHOT: TUI during active drain showing nodes in different stages (docs/images/watch.png) -->

### `kupgrade snapshot` — capture cluster state

Take a snapshot of your workloads before you start. You'll use this to compare afterwards.

```bash
$ kupgrade snapshot
  Collecting cluster state from context "prod-cluster"...
  Snapshot saved: ~/.kupgrade/snapshots/prod-cluster-2026-02-02T14-54-32.json
  64 workloads, 9 nodes, 23 PDBs across 21 namespaces
```

### `kupgrade report` — post-upgrade diff

Compare your snapshot against the live cluster. See what broke during the upgrade and what was already broken before you started.

<!-- SCREENSHOT: terminal output of kupgrade report showing NEW_ISSUE, PRE_EXISTING, RESOLVED sections (docs/images/report.png) -->

This is the part you paste into Slack. The output works without colors — `[NEW_ISSUE]`, `[PRE_EXISTING]`, `[RESOLVED]` read fine in plain text.

### `kupgrade check` — pre-flight validation

Run this before you upgrade. It checks node health and flags anything that could block you.

```bash
$ kupgrade check
  ✅ Node Conditions    All 9 nodes Ready
  ⚠️  Deprecations      2 deprecated APIs found
  ❌ PDB Health         1 PDB blocking: default/web-pdb (0 disruptions allowed)

  Exit code: 2 (blocking issues found)
```

---

## Install

```bash
go install github.com/sabirmohamed/kupgrade@latest
```

Or build from source:

```bash
git clone https://github.com/sabirmohamed/kupgrade
cd kupgrade
go build -o kupgrade ./cmd/kupgrade
```

---

## Usage

```bash
kupgrade                    # Full TUI experience
kupgrade --context prod     # Specific cluster

# For scripting/CI
kupgrade check              # Pre-flight validation (exit code 0/1)
kupgrade snapshot           # Save pre-upgrade baseline
kupgrade report             # Post-upgrade diff
kupgrade report --format json
```

All standard kubectl flags work (`--context`, `--namespace`, `--kubeconfig`).

---

## TUI screens

The `watch` command has 6 screens. Press the number keys to switch between them.

| Key | Screen | Shows |
|-----|--------|-------|
| `0` | Overview | Progress, stages, blockers, active drains |
| `1` | Nodes | All nodes with stage, version, pod count |
| `2` | Pods | Pod states, migrations, restarts |
| `3` | Drains | Active drain progress with elapsed time |
| `4` | Blockers | PDBs preventing evictions |
| `5` | Events | Upgrade-relevant events with filters |

<!-- SCREENSHOT: nodes screen showing table with stages and versions (docs/images/nodes.png) -->

### Navigation

| Key | Action |
|-----|--------|
| `↑/↓` or `j/k` | Navigate lists |
| `Enter` | Show details |
| `?` | Help overlay |
| `q` | Back / Quit |
| `/` | Fuzzy search pods (Enter commit, Esc clear) |
| `a` | Cycle pod filter (disrupting/rescheduled/all) |
| `u/w/a` | Event filter (upgrade/warnings/all) |

---

## What it tracks

| Resource | What |
|----------|------|
| Nodes | Stage transitions (Ready → Cordoned → Draining → Upgrading → Complete), version changes |
| Pods | Evictions, migrations across nodes, restarts, probe failures |
| PDBs | Disruption budgets blocking drains |
| Events | Filtered to upgrade-relevant activity |

---

## What this isn't

- **Not a cluster browser** — use k9s for that. kupgrade only shows what matters during an upgrade.
- **Not an upgrade orchestrator** — your platform handles that (AKS, EKS, GKE, kOps). kupgrade just watches.
- **Not a deprecation scanner** — kupgrade focuses on the live upgrade. For API deprecation scanning before you upgrade, use [pluto](https://github.com/FairwindsOps/pluto), [kubepug](https://github.com/kubepug/kubepug), or [kubent](https://github.com/doitintl/kube-no-trouble).

---

## Platform support

kupgrade started as an AKS upgrade observer and runs in AKS production today. We're actively working on GKE and EKS support.

| Platform | Status | Stage Pipeline | Notes |
|----------|--------|---------------|-------|
| **AKS** | Fully supported | 5 stages (Ready → Cordoned → Draining → Upgrading → Complete) | AKS reimages nodes in place — you see the full lifecycle |
| **GKE** | Works, validating | 4 stages (Upgrading stage N/A) | GKE replaces nodes — no Upgrading stage |
| **EKS** | Works, validating | 4 stages (Upgrading stage N/A) | EKS replaces nodes — no Upgrading stage |
| **Other** | Untested | 4 stages | Should work on any platform with rolling node upgrades |

**Why the difference?** Each platform handles node upgrades differently. AKS reimages your existing nodes — the node name stays the same, the kubelet restarts with the new version, and the node comes back. You can follow a single node through the entire upgrade. GKE and EKS take a different approach: they delete old nodes and create new ones with different names. You'll see old nodes disappear and new ones show up at the target version.

Everything else — PDB blockers, pod migrations, snapshot/report, pre-flight checks — works the same on every platform.

### Tested versions

| Platform | Versions |
|----------|----------|
| AKS | 1.28 – 1.33 |

If you try it on another platform, open an issue and tell us how it went.

## Under the hood

`kupgrade` uses:

- [bubbletea](https://github.com/charmbracelet/bubbletea), [lipgloss](https://github.com/charmbracelet/lipgloss), and [bubbles](https://github.com/charmbracelet/bubbles) for the terminal UI
- [cobra](https://github.com/spf13/cobra) and [cli-runtime](https://github.com/kubernetes/cli-runtime) for kubectl-compatible CLI flags
- [client-go](https://github.com/kubernetes/client-go) informers for efficient real-time watching (no polling)
- [kubectl](https://github.com/kubernetes/kubectl) describe SDK for the detail overlay
- [fuzzy](https://github.com/sahilm/fuzzy) for pod search

---

## Requirements

- kubectl access to your cluster
- Read permissions: nodes, pods, events, poddisruptionbudgets

---

## Contributing

Contributions welcome — open an issue first so we can talk about it.

```bash
git clone https://github.com/sabirmohamed/kupgrade
cd kupgrade
go build -o kupgrade ./cmd/kupgrade
./kupgrade watch --context my-cluster
```

---

## Development

Built with assitance of [Claude Opus 4.5](https://claude.ai) using the [BMAD Method](https://github.com/bmadcode/BMAD-METHOD), followwing [Google's Go Style Guide](https://google.github.io/styleguide/go) and https://go.dev/doc/effective_go.

---

## License

Apache 2.0 — See [LICENSE](LICENSE)
