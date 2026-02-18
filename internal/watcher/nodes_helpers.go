package watcher

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// isNodeReady checks if a node has the Ready condition True.
func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// extractConditions returns non-Ready conditions that are True (problems).
func extractConditions(node *corev1.Node) []string {
	var conditions []string
	for _, cond := range node.Status.Conditions {
		// Skip Ready condition (handled separately) and False conditions
		if cond.Type == corev1.NodeReady {
			continue
		}
		// These conditions are problems when True
		if cond.Status == corev1.ConditionTrue {
			conditions = append(conditions, string(cond.Type))
		}
	}
	return conditions
}

// extractPoolInfo reads pool name and mode from node labels.
// Supports AKS, EKS, and GKE label conventions.
func extractPoolInfo(node *corev1.Node) (pool, poolMode string) {
	labels := node.Labels
	if labels == nil {
		return "", ""
	}

	// AKS: kubernetes.azure.com/agentpool, kubernetes.azure.com/mode
	if p, ok := labels["kubernetes.azure.com/agentpool"]; ok {
		pool = p
		poolMode = labels["kubernetes.azure.com/mode"]
		return pool, poolMode
	}

	// EKS: eks.amazonaws.com/nodegroup
	if p, ok := labels["eks.amazonaws.com/nodegroup"]; ok {
		return p, ""
	}

	// GKE: cloud.google.com/gke-nodepool
	if p, ok := labels["cloud.google.com/gke-nodepool"]; ok {
		return p, ""
	}

	return "", ""
}

// extractTaints returns taint effects (NoSchedule, NoExecute, etc.).
func extractTaints(node *corev1.Node) []string {
	var taints []string
	seen := make(map[string]bool)
	for _, taint := range node.Spec.Taints {
		effect := string(taint.Effect)
		if !seen[effect] {
			taints = append(taints, effect)
			seen[effect] = true
		}
	}
	return taints
}

// formatAge returns human-readable age matching kubectl format (e.g., "5d2h", "3h14m", "30m").
func formatAge(created time.Time) string {
	d := time.Since(created)

	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	switch {
	case days > 0:
		if hours > 0 {
			return fmt.Sprintf("%dd%dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	case hours > 0:
		if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
