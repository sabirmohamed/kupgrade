package check

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Compile-time interface check.
var _ Checker = (*NodeConditionsChecker)(nil)

const nodeConditionsCheckerName = "Node Conditions"

// NodeConditionsChecker verifies all nodes are Ready.
type NodeConditionsChecker struct{}

// Name returns the checker name.
func (c *NodeConditionsChecker) Name() string {
	return nodeConditionsCheckerName
}

// Run checks that all nodes report a Ready condition.
func (c *NodeConditionsChecker) Run(ctx context.Context, clients Clients, targetVersion string) ([]Result, error) {
	nodeList, err := clients.Kubernetes.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("check: node conditions: list nodes: %w", err)
	}

	var unhealthy []string
	var unhealthyDetails []string

	for _, node := range nodeList.Items {
		ready := isNodeReady(&node)
		if !ready {
			unhealthy = append(unhealthy, node.Name)
			conditions := nodeConditionSummary(&node)
			unhealthyDetails = append(unhealthyDetails, fmt.Sprintf("%s: %s", node.Name, conditions))
		}
	}

	if len(unhealthy) > 0 {
		return []Result{{
			CheckName: nodeConditionsCheckerName,
			Severity:  SeverityBlocking,
			Message:   fmt.Sprintf("%d of %d nodes not Ready", len(unhealthy), len(nodeList.Items)),
			Details:   unhealthyDetails,
		}}, nil
	}

	return []Result{{
		CheckName: nodeConditionsCheckerName,
		Severity:  SeverityPass,
		Message:   fmt.Sprintf("All %d nodes Ready", len(nodeList.Items)),
	}}, nil
}

func isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func nodeConditionSummary(node *corev1.Node) string {
	var parts []string
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			parts = append(parts, fmt.Sprintf("Ready=%s (%s)", condition.Status, condition.Reason))
		} else if condition.Status == corev1.ConditionTrue {
			// Report active problem conditions (MemoryPressure, DiskPressure, etc.).
			parts = append(parts, fmt.Sprintf("%s=%s", condition.Type, condition.Status))
		}
	}
	if len(parts) == 0 {
		return "Ready condition missing"
	}
	return strings.Join(parts, ", ")
}
