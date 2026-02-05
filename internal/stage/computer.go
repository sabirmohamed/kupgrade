package stage

import (
	"sync"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	"golang.org/x/mod/semver"
	corev1 "k8s.io/api/core/v1"
)

// Computer implements StageComputer for node stage detection
type Computer struct {
	targetVersion    string
	lowestVersion    string
	upgradeWasActive bool // true once mixed versions detected
	upgradeCompleted bool // true once versions converge after being mixed
	nodePodCounts    map[string]int
	mu               sync.RWMutex
}

// New creates a new stage computer
func New(targetVersion string) *Computer {
	return &Computer{
		targetVersion: targetVersion,
		nodePodCounts: make(map[string]int),
	}
}

// ComputeStage returns current stage for a node
func (c *Computer) ComputeStage(node *corev1.Node) types.NodeStage {
	c.mu.RLock()
	target := c.targetVersion
	lowest := c.lowestVersion
	c.mu.RUnlock()

	version := node.Status.NodeInfo.KubeletVersion
	schedulable := !node.Spec.Unschedulable
	ready := isNodeReady(node)

	// Check if upgrade is active (mixed versions exist) or was completed
	upgradeActive := lowest != "" && target != "" && lowest != target
	completed := c.upgradeCompleted

	switch {
	case (upgradeActive || completed) && version == target && ready && schedulable:
		return types.StageComplete
	case !ready:
		return types.StageReimaging
	case !schedulable:
		// NodeWatcher will correct to DRAINING when pods are actually evicted
		return types.StageCordoned
	default:
		return types.StageReady
	}
}

// UpdatePodCount updates the pod count for a node
func (c *Computer) UpdatePodCount(nodeName string, delta int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nodePodCounts[nodeName] += delta
	if c.nodePodCounts[nodeName] < 0 {
		c.nodePodCounts[nodeName] = 0
	}
}

// SetTargetVersion updates target (highest) and tracks lowest version
func (c *Computer) SetTargetVersion(version string) {
	if version == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Keep highest version as target
	if c.targetVersion == "" || semver.Compare(version, c.targetVersion) > 0 {
		c.targetVersion = version
	}

	// Track lowest version to detect active upgrades
	if c.lowestVersion == "" || semver.Compare(version, c.lowestVersion) < 0 {
		c.lowestVersion = version
	}

	c.checkUpgradeCompletion()
}

// checkUpgradeCompletion detects upgrade lifecycle transitions.
// Must be called with lock held.
func (c *Computer) checkUpgradeCompletion() {
	if c.lowestVersion == "" || c.targetVersion == "" {
		return
	}
	if c.lowestVersion != c.targetVersion {
		// Mixed versions: upgrade is active
		c.upgradeWasActive = true
		c.upgradeCompleted = false // Reset if new upgrade started
	} else if c.upgradeWasActive {
		// Versions converged after being mixed: upgrade complete
		c.upgradeCompleted = true
	}
}

// UpgradeCompleted returns true if an upgrade has completed (versions converged after being mixed)
func (c *Computer) UpgradeCompleted() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.upgradeCompleted
}

// TargetVersion returns the current target version
func (c *Computer) TargetVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.targetVersion
}

// RecomputeVersions recalculates target and lowest versions from the given list
func (c *Computer) RecomputeVersions(versions []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lowestVersion = ""
	c.targetVersion = ""
	for _, v := range versions {
		if v == "" {
			continue
		}
		if c.targetVersion == "" || semver.Compare(v, c.targetVersion) > 0 {
			c.targetVersion = v
		}
		if c.lowestVersion == "" || semver.Compare(v, c.lowestVersion) < 0 {
			c.lowestVersion = v
		}
	}

	c.checkUpgradeCompletion()
}

// LowestVersion returns the lowest version seen across nodes
func (c *Computer) LowestVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lowestVersion
}

// PodCount returns the pod count for a node
func (c *Computer) PodCount(nodeName string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nodePodCounts[nodeName]
}

// DetectTargetVersion auto-detects target from node versions
func (c *Computer) DetectTargetVersion(nodes []*corev1.Node) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.targetVersion != "" {
		return c.targetVersion
	}

	var highest string
	for _, node := range nodes {
		v := node.Status.NodeInfo.KubeletVersion
		if highest == "" || semver.Compare(v, highest) > 0 {
			highest = v
		}
	}

	c.targetVersion = highest
	return highest
}

// isNodeReady checks if node has Ready condition True
func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
