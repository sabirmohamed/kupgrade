package kube

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// NodeMetrics holds CPU and memory usage percentages for a node.
type NodeMetrics struct {
	CPUPercent int
	MemPercent int
}

// metricsResponse matches the metrics.k8s.io/v1beta1 NodeMetricsList schema.
type metricsResponse struct {
	Items []metricsItem `json:"items"`
}

type metricsItem struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Usage map[string]string `json:"usage"`
}

// FetchNodeMetrics queries the metrics-server API and computes CPU/Memory
// percentages for each node. Returns an empty map when metrics-server is
// unavailable (graceful degradation — the dashboard shows "—" columns).
func FetchNodeMetrics(ctx context.Context, clientset kubernetes.Interface) map[string]NodeMetrics {
	result := make(map[string]NodeMetrics)

	// Fetch usage from metrics-server
	data, err := clientset.CoreV1().RESTClient().
		Get().
		AbsPath("/apis/metrics.k8s.io/v1beta1/nodes").
		DoRaw(ctx)
	if err != nil {
		return result
	}

	var metrics metricsResponse
	if err := json.Unmarshal(data, &metrics); err != nil {
		return result
	}
	if len(metrics.Items) == 0 {
		return result
	}

	// Fetch allocatable from node status
	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return result
	}

	allocatable := make(map[string]corev1.ResourceList, len(nodeList.Items))
	for i := range nodeList.Items {
		allocatable[nodeList.Items[i].Name] = nodeList.Items[i].Status.Allocatable
	}

	// Compute percentages: usage / allocatable * 100
	for _, item := range metrics.Items {
		alloc, ok := allocatable[item.Metadata.Name]
		if !ok {
			continue
		}

		nm := NodeMetrics{}

		if cpuStr, ok := item.Usage["cpu"]; ok {
			cpuUsage, err := resource.ParseQuantity(cpuStr)
			if err == nil {
				if cpuAlloc := alloc[corev1.ResourceCPU]; cpuAlloc.MilliValue() > 0 {
					nm.CPUPercent = int(cpuUsage.MilliValue() * 100 / cpuAlloc.MilliValue())
				}
			}
		}

		if memStr, ok := item.Usage["memory"]; ok {
			memUsage, err := resource.ParseQuantity(memStr)
			if err == nil {
				if memAlloc := alloc[corev1.ResourceMemory]; memAlloc.Value() > 0 {
					nm.MemPercent = int(memUsage.Value() * 100 / memAlloc.Value())
				}
			}
		}

		result[item.Metadata.Name] = nm
	}

	return result
}
