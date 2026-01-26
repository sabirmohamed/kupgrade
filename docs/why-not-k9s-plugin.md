# Why kupgrade is a Standalone Tool (Not a k9s Plugin)

## TL;DR

k9s plugins are shell commands that display static output. kupgrade needs real-time state tracking, cross-resource correlation, and interactive navigation that plugins architecturally cannot provide.

---

## k9s Plugin Architecture

```
User presses shortcut → Shell command runs → Static output displayed → User presses Enter → Back to k9s
```

Plugins are defined in YAML and execute shell commands:

```yaml
plugins:
  my-plugin:
    shortCut: Shift-X
    scopes:
      - node
    command: bash
    args:
      - -c
      - "kubectl get node $NAME -o yaml"
```

This is powerful for quick utilities, but fundamentally limited for complex observability.

---

## Capability Comparison

| Capability | k9s Plugin | kupgrade |
|------------|-----------|----------|
| **Create custom screens** | ❌ Shell output only | ✅ Full TUI with multiple screens |
| **Real-time updates** | ❌ Static snapshot | ✅ Watch-based live updates |
| **State persistence** | ❌ Stateless between calls | ✅ Tracks state transitions over time |
| **Cross-resource correlation** | ❌ Single kubectl call | ✅ Multiple informers combined |
| **Interactive navigation** | ❌ Text dump, Enter to exit | ✅ Arrow keys, Enter for details, screen switching |
| **Computed metrics** | ❌ No computation | ✅ Velocity, ETA, stage durations |
| **Aggregate views** | ❌ One resource at a time | ✅ All nodes/pods/PDBs in unified view |

---

## Concrete Example: BLOCKERS Screen

kupgrade's BLOCKERS screen shows why drains are stuck. This requires:

1. **Watch all PDBs** - track `disruptionsAllowed` status in real-time
2. **Match PDB selectors to pods** - correlate PDB label selectors with pod labels
3. **Filter to draining nodes** - only show blockers for nodes currently draining
4. **Detect local storage** - scan pod specs for `emptyDir` and `hostPath` volumes
5. **Find orphan pods** - identify pods without `ownerReferences`
6. **Track stuck terminating** - monitor `deletionTimestamp` over time
7. **Correlate everything** - show which PDB blocks which pod on which node

**A k9s plugin can only do:**
```bash
kubectl get pdb -A
```

It cannot:
- Maintain state between invocations
- Correlate PDBs with pods and nodes
- Update in real-time as conditions change
- Provide interactive drill-down

---

## Why Informers Matter

kupgrade uses Kubernetes informers (watch-based):

```
┌─────────────────────┐     ┌────────────────────┐     ┌─────────────────┐
│   K8s Informers     │     │   Computed State   │     │     Screens     │
│   (real-time)       │ ──▶ │   (derived data)   │ ──▶ │   (interactive) │
├─────────────────────┤     ├────────────────────┤     ├─────────────────┤
│ • Node informer     │     │ • Node stages      │     │ • Overview      │
│ • Pod informer      │     │ • Blocker list     │     │ • Nodes         │
│ • PDB informer      │     │ • Eviction queue   │     │ • Drains        │
│ • Event informer    │     │ • Stage durations  │     │ • Pods          │
└─────────────────────┘     │ • Velocity/ETA     │     │ • Blockers      │
                            └────────────────────┘     │ • Events        │
                                                       │ • Stats         │
                                                       └─────────────────┘
```

**Benefits:**
- **Real-time**: Changes appear instantly (no polling)
- **Efficient**: Only deltas transmitted, not full lists
- **Correlated**: All resource types available simultaneously
- **Stateful**: Track transitions over time (e.g., "node was draining for 5 minutes")

---

## What k9s Plugins ARE Good For

k9s plugins excel at quick, single-resource utilities:

| Use Case | Good for Plugin? |
|----------|------------------|
| Show node details | ✅ Yes |
| Sync ArgoCD app | ✅ Yes |
| Restart deployment | ✅ Yes |
| **Track upgrade progress** | ❌ No |
| **Correlate blockers** | ❌ No |
| **Real-time drain monitoring** | ❌ No |

We provide k9s plugins for quick node info (`Shift-U`, `Shift-V`) as a complement to kupgrade, not a replacement.

---

## Summary

kupgrade exists because upgrade observability requires:

1. **Multiple resource types** watched simultaneously
2. **State tracking** over time (stage transitions, durations)
3. **Computed metrics** (velocity, ETA, blocker detection)
4. **Interactive navigation** between related data
5. **Real-time updates** without manual refresh

k9s plugins are architecturally limited to "run command, show output, exit." They're great for utilities but cannot provide the holistic, real-time upgrade dashboard that kupgrade delivers.

---

## See Also

- [OBSERVABILITY_PLAN.md](../OBSERVABILITY_PLAN.md) - Full screen designs and data architecture
- [k9s plugins documentation](https://k9scli.io/topics/plugins/) - Official k9s plugin reference
