package stage

import (
	"sync"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	"golang.org/x/mod/semver"
	corev1 "k8s.io/api/core/v1"
)

// Computer implements StageComputer for node stage detection
type Computer struct {
	targetVersion  string
	lowestVersion  string
	nodePodCounts  map[string]int
	mu             sync.RWMutex
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
	podCount := c.nodePodCounts[node.Name]
	target := c.targetVersion
	lowest := c.lowestVersion
	c.mu.RUnlock()

	version := node.Status.NodeInfo.KubeletVersion
	schedulable := !node.Spec.Unschedulable
	ready := isNodeReady(node)

	// Check if upgrade is active (mixed versions exist)
	upgradeActive := lowest != "" && target != "" && lowest != target

	switch {
	case upgradeActive && version == target && ready && schedulable:
		return types.StageComplete
	case !ready:
		return types.StageUpgrading
	case !schedulable && podCount == 0:
		return types.StageDraining
	case !schedulable:
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
}

// TargetVersion returns the current target version
func (c *Computer) TargetVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.targetVersion
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
