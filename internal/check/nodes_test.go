package check

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	testNodeName    = "node-1"
	testNodeVersion = "v1.31.0"
)

func newNode(name string, ready corev1.ConditionStatus, opts ...func(*corev1.Node)) *corev1.Node {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: ready},
			},
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion: testNodeVersion,
			},
		},
	}
	for _, opt := range opts {
		opt(node)
	}
	return node
}

func withCondition(conditionType corev1.NodeConditionType, status corev1.ConditionStatus, reason string) func(*corev1.Node) {
	return func(node *corev1.Node) {
		node.Status.Conditions = append(node.Status.Conditions, corev1.NodeCondition{
			Type:   conditionType,
			Status: status,
			Reason: reason,
		})
	}
}

func TestNodeConditionsChecker(t *testing.T) {
	tests := []struct {
		name         string
		nodes        []*corev1.Node
		wantSeverity Severity
		wantCount    int
	}{
		{
			name: "all nodes ready",
			nodes: []*corev1.Node{
				newNode("node-1", corev1.ConditionTrue),
				newNode("node-2", corev1.ConditionTrue),
				newNode("node-3", corev1.ConditionTrue),
			},
			wantSeverity: SeverityPass,
			wantCount:    1,
		},
		{
			name: "one node not ready",
			nodes: []*corev1.Node{
				newNode("node-1", corev1.ConditionTrue),
				newNode("node-2", corev1.ConditionFalse),
			},
			wantSeverity: SeverityBlocking,
			wantCount:    1,
		},
		{
			name: "node with additional conditions",
			nodes: []*corev1.Node{
				newNode("node-1", corev1.ConditionFalse,
					withCondition(corev1.NodeMemoryPressure, corev1.ConditionTrue, "MemoryUnderPressure"),
				),
			},
			wantSeverity: SeverityBlocking,
			wantCount:    1,
		},
		{
			name:         "empty cluster",
			nodes:        nil,
			wantSeverity: SeverityPass,
			wantCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := make([]runtime.Object, 0, len(tt.nodes))
			for _, node := range tt.nodes {
				objects = append(objects, node)
			}

			clientset := fake.NewSimpleClientset(objects...)
			checker := &NodeConditionsChecker{}
			results, err := checker.Run(context.Background(), Clients{Kubernetes: clientset}, testTargetVersion)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Fatalf("results count = %d, want %d", len(results), tt.wantCount)
			}

			if results[0].Severity != tt.wantSeverity {
				t.Errorf("severity = %v, want %v", results[0].Severity, tt.wantSeverity)
			}
		})
	}
}

func TestNodeConditionsCheckerDetails(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		newNode("healthy-node", corev1.ConditionTrue),
		newNode("unhealthy-node", corev1.ConditionFalse),
	)

	checker := &NodeConditionsChecker{}
	results, err := checker.Run(context.Background(), Clients{Kubernetes: clientset}, testTargetVersion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.Severity != SeverityBlocking {
		t.Errorf("severity = %v, want %v", result.Severity, SeverityBlocking)
	}
	if len(result.Details) != 1 {
		t.Errorf("details count = %d, want 1", len(result.Details))
	}
}

func TestNodeConditionsCheckerShowsAllConditions(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		newNode("pressured-node", corev1.ConditionFalse,
			withCondition(corev1.NodeMemoryPressure, corev1.ConditionTrue, "MemoryUnderPressure"),
			withCondition(corev1.NodeDiskPressure, corev1.ConditionTrue, "DiskFull"),
		),
	)

	checker := &NodeConditionsChecker{}
	results, err := checker.Run(context.Background(), Clients{Kubernetes: clientset}, testTargetVersion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	detail := results[0].Details[0]
	if !strings.Contains(detail, "MemoryPressure") {
		t.Errorf("detail missing MemoryPressure: %s", detail)
	}
	if !strings.Contains(detail, "DiskPressure") {
		t.Errorf("detail missing DiskPressure: %s", detail)
	}
}
