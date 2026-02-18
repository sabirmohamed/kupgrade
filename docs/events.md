# Events Monitoring

kupgrade monitors Kubernetes events that are relevant during cluster upgrades. Normal pod lifecycle events (Scheduled, Pulling, Pulled, Created, Started) are excluded to reduce noise — during a rolling upgrade with 100+ pods, these events are expected and not actionable.

## Event Sources

kupgrade collects events from three watchers:

### 1. Node Watcher

Watches node objects directly. Emits events when node state changes.

| Event Type | Severity | When it fires |
|-----------|----------|---------------|
| `NODE_CORDON` | warning | Node marked unschedulable (upgrade starting on this node) |
| `NODE_UNCORDON` | info | Node marked schedulable again |
| `NODE_READY` | info | Node condition becomes Ready (includes reimage detection) |
| `NODE_NOTREADY` | warning | Node condition becomes NotReady |
| `NODE_VERSION` | info | Node kubelet version changed |

### 2. Pod Watcher

Watches pod objects directly. Emits events when pod state changes during upgrade.

| Event Type | Severity | When it fires |
|-----------|----------|---------------|
| `POD_EVICTED` | warning | Pod evicted from a node being drained |
| `POD_SCHEDULED` | info | Pod assigned to a node |
| `POD_READY` | info | Pod becomes Ready on its node |
| `POD_FAILED` | error | Pod enters Failed/Error phase |
| `POD_DELETED` | info | Pod deleted (usually during drain) |
| `MIGRATION` | info | Pod rescheduled to a different node after eviction |

### 3. Kubernetes Event Watcher

Watches the Kubernetes Events API (`v1.Event` objects). Filters to 32 upgrade-relevant reasons from the full event stream.

## Watched K8s Event Reasons (32)

### Node Lifecycle

| Reason | K8s Event Type | Description |
|--------|---------------|-------------|
| `NodeReady` | Normal | Node condition transitioned to Ready |
| `NodeNotReady` | Warning | Node condition transitioned to NotReady |
| `NodeSchedulable` | Normal | Node marked as schedulable |
| `NodeNotSchedulable` | Normal | Node marked as unschedulable (cordoned) |
| `Rebooted` | Warning | Node rebooted |
| `NodeAllocatableEnforced` | Warning | Node allocatable resources enforced |

### AKS Upgrade Events

| Reason | K8s Event Type | Description |
|--------|---------------|-------------|
| `Upgrade` | Normal | AKS upgrade operation — "Deleting node X from API server" or "Successfully reimaged/upgraded node: X" |
| `RemovingNode` | Normal | "Removing Node X from Controller" |
| `RegisteredNode` | Normal | "Registered Node X in Controller" — new or reimaged node discovered |
| `Surge` | Normal | Surge node created or removed — "Created a surge node X for agentpool Y" |

### Drain Operations

| Reason | K8s Event Type | Description |
|--------|---------------|-------------|
| `Drain` | Warning | Node drain initiated — evicting pods |
| `FailedDrain` | Warning | Drain operation failed (usually PDB blocked) |
| `Cordon` | Normal | Node cordoned |
| `Uncordon` | Normal | Node uncordoned |
| `Killing` | Normal | Kubelet killing container during drain/eviction |

### Pod Failures

| Reason | K8s Event Type | Description |
|--------|---------------|-------------|
| `Failed` | Warning | Image pull failure (ErrImagePull, ImagePullBackOff) |
| `FailedCreate` | Warning | Controller (DaemonSet, ReplicaSet) can't create pod on node |
| `FailedKillPod` | Warning | Kubelet failed to kill a pod |
| `FailedScheduling` | Warning | Scheduler can't place pod — no nodes available, resource constraints, taints |
| `FailedBinding` | Warning | Scheduler failed to bind pod to node |

### PDB / Disruption Blockers

| Reason | K8s Event Type | Description |
|--------|---------------|-------------|
| `DisruptionBlocked` | Warning | PodDisruptionBudget is blocking eviction |
| `CalculateExpectedPodCountFailed` | Warning | PDB controller can't calculate expected pod count |
| `FailedEviction` | Warning | Eviction blocked by PDB — pod can't be removed from draining node |

### Volume Issues

| Reason | K8s Event Type | Description |
|--------|---------------|-------------|
| `FailedMount` | Warning | Volume mount failed (configmap, secret, PVC) |
| `FailedAttachVolume` | Warning | Volume attach failed (cloud disk, CSI) |
| `FailedDetachVolume` | Warning | Volume detach failed — blocks node drain completion |
| `VolumeFailedDelete` | Warning | PersistentVolume deletion failed |

### Health Issues

| Reason | K8s Event Type | Description |
|--------|---------------|-------------|
| `Unhealthy` | Warning | Readiness or liveness probe failed |
| `ProbeWarning` | Warning | Probe returned warning (non-fatal) |
| `BackOff` | Warning | Container in CrashLoopBackOff — back-off restarting failed container |
| `SystemOOM` | Warning | System OOM killer invoked on node |
| `FreeDiskSpaceFailed` | Warning | Node failed to free disk space (eviction threshold) |
| `ContainerGCFailed` | Warning | Container garbage collection failed |
| `ImageGCFailed` | Warning | Image garbage collection failed |

## Events Not Watched

These K8s event reasons are intentionally excluded because they are normal pod lifecycle events that generate high volume during upgrades without being actionable:

| Reason | Why excluded |
|--------|-------------|
| `Scheduled` | Normal — every rescheduled pod triggers this |
| `Pulling` | Normal — image being pulled |
| `Pulled` | Normal — image successfully pulled |
| `Created` | Normal — container created |
| `Started` | Normal — container started |
| `SuccessfulCreate` | Normal — controller created a pod |
| `SuccessfulDelete` | Normal — controller deleted a pod |
| `ScalingReplicaSet` | Normal — deployment scaled a ReplicaSet |

## Severity Mapping

kupgrade does **not** invent its own severity — it maps directly from what Kubernetes reports:

| Source | K8s Type | kupgrade Severity | Icon |
|--------|----------|-------------------|------|
| K8s Event Watcher | `Warning` | warning | ⚠ |
| K8s Event Watcher | `Normal` | info | • |
| Pod Watcher | pod enters `Failed` phase | error | ✖ |
| Pod Watcher | pod evicted | warning | ⚠ |
| Pod Watcher | pod ready / scheduled | info | • |
| Node Watcher | node NotReady | warning | ⚠ |
| Node Watcher | node Ready / version change | info | • |

The only "error" severity events come from pods actually entering the Failed phase — not from K8s events being promoted. K8s events are either Warning or Normal, and kupgrade preserves that 1:1.

## How Events Are Displayed

### Severity Sorting

Events are sorted by severity — errors on top, warnings in the middle, info at the bottom. Within each severity level, newest events appear first.

### Aggregation

Identical events (same reason) are grouped with a count badge:
- `FailedScheduling ×4` — 4 pods can't be placed
- `Unhealthy ×11` — 11 pods failing probes

Press `e` to expand a group and see individual events. Press `d` or `Enter` to open the full event detail.

### Aggregation Key

Events are grouped by this priority:
1. `Event.Reason` field (populated from K8s event reason for K8S_WARNING/K8S_NORMAL events)
2. Bracket-parsed reason from message (e.g., `[BackOff]` → `BackOff`)
3. Event type string fallback (e.g., `NODE_CORDON`, `POD_EVICTED`)
