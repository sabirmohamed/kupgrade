package snapshot

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

// ownerKey uniquely identifies a workload owner.
type ownerKey struct {
	Namespace string
	Kind      string
	Name      string
}

func (k ownerKey) String() string {
	return fmt.Sprintf("%s/%s/%s", k.Namespace, k.Kind, k.Name)
}

// workloadInfo aggregates pod data for a single workload owner.
type workloadInfo struct {
	Key             ownerKey
	DesiredReplicas int
	ReadyReplicas   int
	PodStatuses     map[string]int
	TotalRestarts   int
	BarePod         bool
}

// Collect gathers a full cluster snapshot using direct API calls.
func Collect(ctx context.Context, clientset kubernetes.Interface, clusterContext string) (*types.Snapshot, []string, error) {
	serverVersion, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot: server version: %w", err)
	}

	var (
		nodes        *corev1.NodeList
		pods         *corev1.PodList
		deployments  *appsv1.DeploymentList
		replicaSets  *appsv1.ReplicaSetList
		statefulSets *appsv1.StatefulSetList
		daemonSets   *appsv1.DaemonSetList
		jobs         *batchv1.JobList
		cronJobs     *batchv1.CronJobList
		pdbs         *policyv1.PodDisruptionBudgetList

		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
		warnings []string
	)

	// Fetch all resources concurrently.
	type fetchFunc func()
	fetches := []fetchFunc{
		func() {
			result, fetchErr := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			mu.Lock()
			defer mu.Unlock()
			if fetchErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("snapshot: list nodes: %w", fetchErr)
			}
			nodes = result
		},
		func() {
			result, fetchErr := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
			mu.Lock()
			defer mu.Unlock()
			if fetchErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("snapshot: list pods: %w", fetchErr)
			}
			pods = result
		},
		func() {
			result, fetchErr := clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
			mu.Lock()
			defer mu.Unlock()
			if fetchErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("snapshot: list deployments: %w", fetchErr)
			}
			deployments = result
		},
		func() {
			result, fetchErr := clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
			mu.Lock()
			defer mu.Unlock()
			if fetchErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("snapshot: list replicasets: %w", fetchErr)
			}
			replicaSets = result
		},
		func() {
			result, fetchErr := clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
			mu.Lock()
			defer mu.Unlock()
			if fetchErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("snapshot: list statefulsets: %w", fetchErr)
			}
			statefulSets = result
		},
		func() {
			result, fetchErr := clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
			mu.Lock()
			defer mu.Unlock()
			if fetchErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("snapshot: list daemonsets: %w", fetchErr)
			}
			daemonSets = result
		},
		func() {
			result, fetchErr := clientset.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
			mu.Lock()
			defer mu.Unlock()
			if fetchErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("snapshot: list jobs: %w", fetchErr)
			}
			jobs = result
		},
		func() {
			result, fetchErr := clientset.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{})
			mu.Lock()
			defer mu.Unlock()
			if fetchErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("snapshot: list cronjobs: %w", fetchErr)
			}
			cronJobs = result
		},
		func() {
			result, fetchErr := clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
			mu.Lock()
			defer mu.Unlock()
			if fetchErr != nil && firstErr == nil {
				firstErr = fmt.Errorf("snapshot: list pdbs: %w", fetchErr)
			}
			pdbs = result
		},
	}

	wg.Add(len(fetches))
	for _, fn := range fetches {
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	wg.Wait()

	if firstErr != nil {
		return nil, nil, firstErr
	}

	// Build ownership graph: UID → ownerKey for top-level controllers.
	ownerGraph := buildOwnerGraph(deployments, replicaSets, statefulSets, daemonSets, jobs, cronJobs)

	// Build desired/ready replica counts from controllers.
	replicaCounts := buildReplicaCounts(deployments, statefulSets, daemonSets)

	// Group pods by top-level owner.
	workloads := make(map[ownerKey]*workloadInfo)

	for i := range pods.Items {
		pod := &pods.Items[i]
		key, barePod := resolveOwner(pod, ownerGraph)

		if barePod {
			warnings = append(warnings, fmt.Sprintf("Bare pod %s/%s has no owner controller — cannot track across upgrades", pod.Namespace, pod.Name))
		}

		info, exists := workloads[key]
		if !exists {
			info = &workloadInfo{
				Key:         key,
				PodStatuses: make(map[string]int),
				BarePod:     barePod,
			}
			workloads[key] = info
		}

		phase := podPhase(pod)
		info.PodStatuses[phase]++
		info.TotalRestarts += podRestarts(pod)
	}

	// Merge replica counts into workload info.
	for key, info := range workloads {
		if counts, ok := replicaCounts[key]; ok {
			info.DesiredReplicas = counts.desired
			info.ReadyReplicas = counts.ready
		} else {
			// For bare pods or untracked kinds, count from pod statuses.
			total := 0
			for _, count := range info.PodStatuses {
				total += count
			}
			info.DesiredReplicas = total
			info.ReadyReplicas = info.PodStatuses["Running"]
		}
	}

	// Build snapshot.
	snapshot := &types.Snapshot{
		SchemaVersion: types.SchemaVersion,
		Timestamp:     time.Now(),
		Context:       clusterContext,
		ServerVersion: serverVersion.GitVersion,
		Nodes:         buildNodeSnapshots(nodes),
		Workloads:     buildWorkloadSnapshots(workloads),
		PDBs:          buildPDBSnapshots(pdbs),
	}

	return snapshot, warnings, nil
}

// buildOwnerGraph maps resource UIDs to their top-level owner keys.
// This allows walking ReplicaSet→Deployment and Job→CronJob chains.
func buildOwnerGraph(
	deployments *appsv1.DeploymentList,
	replicaSets *appsv1.ReplicaSetList,
	statefulSets *appsv1.StatefulSetList,
	daemonSets *appsv1.DaemonSetList,
	jobs *batchv1.JobList,
	cronJobs *batchv1.CronJobList,
) map[string]ownerKey {
	graph := make(map[string]ownerKey)

	// Map Deployment UIDs.
	deploymentUIDs := make(map[string]ownerKey)
	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		key := ownerKey{
			Namespace: deployment.Namespace,
			Kind:      "Deployment",
			Name:      deployment.Name,
		}
		graph[string(deployment.UID)] = key
		deploymentUIDs[string(deployment.UID)] = key
	}

	// Map ReplicaSet UIDs — walk up to Deployment if one exists.
	for i := range replicaSets.Items {
		replicaSet := &replicaSets.Items[i]
		rsKey := ownerKey{
			Namespace: replicaSet.Namespace,
			Kind:      "ReplicaSet",
			Name:      replicaSet.Name,
		}

		for _, ownerRef := range replicaSet.OwnerReferences {
			if ownerRef.Kind == "Deployment" {
				if deployKey, ok := deploymentUIDs[string(ownerRef.UID)]; ok {
					rsKey = deployKey
					break
				}
			}
		}

		graph[string(replicaSet.UID)] = rsKey
	}

	// Map StatefulSet UIDs.
	for i := range statefulSets.Items {
		statefulSet := &statefulSets.Items[i]
		graph[string(statefulSet.UID)] = ownerKey{
			Namespace: statefulSet.Namespace,
			Kind:      "StatefulSet",
			Name:      statefulSet.Name,
		}
	}

	// Map DaemonSet UIDs.
	for i := range daemonSets.Items {
		daemonSet := &daemonSets.Items[i]
		graph[string(daemonSet.UID)] = ownerKey{
			Namespace: daemonSet.Namespace,
			Kind:      "DaemonSet",
			Name:      daemonSet.Name,
		}
	}

	// Map CronJob UIDs (for Job→CronJob chain).
	cronJobUIDs := make(map[string]ownerKey)
	for i := range cronJobs.Items {
		cronJob := &cronJobs.Items[i]
		cronJobUIDs[string(cronJob.UID)] = ownerKey{
			Namespace: cronJob.Namespace,
			Kind:      "CronJob",
			Name:      cronJob.Name,
		}
	}

	// Map Job UIDs — walk up to CronJob if one exists.
	for i := range jobs.Items {
		job := &jobs.Items[i]
		jobKey := ownerKey{
			Namespace: job.Namespace,
			Kind:      "Job",
			Name:      job.Name,
		}

		// Check if this Job is owned by a CronJob.
		for _, ownerRef := range job.OwnerReferences {
			if ownerRef.Kind == "CronJob" {
				if cronJobKey, ok := cronJobUIDs[string(ownerRef.UID)]; ok {
					jobKey = cronJobKey
					break
				}
			}
		}

		graph[string(job.UID)] = jobKey
	}

	return graph
}

// resolveOwner walks a pod's owner references to find the top-level controller.
// Returns the owner key and whether this is a bare pod.
func resolveOwner(pod *corev1.Pod, graph map[string]ownerKey) (ownerKey, bool) {
	if len(pod.OwnerReferences) == 0 {
		// Bare pod — no owner.
		return ownerKey{
			Namespace: pod.Namespace,
			Kind:      "Pod",
			Name:      pod.Name,
		}, true
	}

	// Find the controller owner reference.
	var controllerRef *metav1.OwnerReference
	for i := range pod.OwnerReferences {
		ref := &pod.OwnerReferences[i]
		if ref.Controller != nil && *ref.Controller {
			controllerRef = ref
			break
		}
	}
	if controllerRef == nil {
		// Has owner refs but none is controller — treat as bare pod.
		return ownerKey{
			Namespace: pod.Namespace,
			Kind:      "Pod",
			Name:      pod.Name,
		}, true
	}

	// Look up in the ownership graph (handles ReplicaSet→Deployment, Job→CronJob).
	if key, ok := graph[string(controllerRef.UID)]; ok {
		return key, false
	}

	// Direct owner not in graph — use the owner reference directly.
	// This handles ReplicaSets owned by Deployments: the RS UID maps to its Deployment.
	// If we get here, it means the direct owner is something we didn't pre-index.
	return ownerKey{
		Namespace: pod.Namespace,
		Kind:      controllerRef.Kind,
		Name:      controllerRef.Name,
	}, false
}

type replicaCount struct {
	desired int
	ready   int
}

func buildReplicaCounts(
	deployments *appsv1.DeploymentList,
	statefulSets *appsv1.StatefulSetList,
	daemonSets *appsv1.DaemonSetList,
) map[ownerKey]replicaCount {
	counts := make(map[ownerKey]replicaCount)

	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		desired := 0
		if deployment.Spec.Replicas != nil {
			desired = int(*deployment.Spec.Replicas)
		}
		counts[ownerKey{
			Namespace: deployment.Namespace,
			Kind:      "Deployment",
			Name:      deployment.Name,
		}] = replicaCount{
			desired: desired,
			ready:   int(deployment.Status.ReadyReplicas),
		}
	}

	for i := range statefulSets.Items {
		statefulSet := &statefulSets.Items[i]
		desired := 0
		if statefulSet.Spec.Replicas != nil {
			desired = int(*statefulSet.Spec.Replicas)
		}
		counts[ownerKey{
			Namespace: statefulSet.Namespace,
			Kind:      "StatefulSet",
			Name:      statefulSet.Name,
		}] = replicaCount{
			desired: desired,
			ready:   int(statefulSet.Status.ReadyReplicas),
		}
	}

	for i := range daemonSets.Items {
		daemonSet := &daemonSets.Items[i]
		counts[ownerKey{
			Namespace: daemonSet.Namespace,
			Kind:      "DaemonSet",
			Name:      daemonSet.Name,
		}] = replicaCount{
			desired: int(daemonSet.Status.DesiredNumberScheduled),
			ready:   int(daemonSet.Status.NumberReady),
		}
	}

	return counts
}

func podPhase(pod *corev1.Pod) string {
	// Check init containers first — a pod stuck in Init:CrashLoopBackOff
	// should report the init failure, not just "Pending".
	for _, containerStatus := range pod.Status.InitContainerStatuses {
		if containerStatus.State.Waiting != nil && containerStatus.State.Waiting.Reason != "" {
			return "Init:" + containerStatus.State.Waiting.Reason
		}
	}

	// Check regular container statuses for CrashLoopBackOff or other waiting reasons.
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.State.Waiting != nil && containerStatus.State.Waiting.Reason != "" {
			return containerStatus.State.Waiting.Reason
		}
	}
	return string(pod.Status.Phase)
}

func podRestarts(pod *corev1.Pod) int {
	total := 0
	for _, containerStatus := range pod.Status.ContainerStatuses {
		total += int(containerStatus.RestartCount)
	}
	return total
}

func buildNodeSnapshots(nodes *corev1.NodeList) []types.NodeSnapshot {
	snapshots := make([]types.NodeSnapshot, 0, len(nodes.Items))
	for i := range nodes.Items {
		node := &nodes.Items[i]
		ready := false
		var conditions []string

		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady {
				ready = condition.Status == corev1.ConditionTrue
			} else if condition.Status == corev1.ConditionTrue {
				conditions = append(conditions, string(condition.Type))
			}
		}

		snapshots = append(snapshots, types.NodeSnapshot{
			Name:       node.Name,
			Version:    node.Status.NodeInfo.KubeletVersion,
			Ready:      ready,
			Conditions: conditions,
		})
	}
	return snapshots
}

func buildWorkloadSnapshots(workloads map[ownerKey]*workloadInfo) []types.WorkloadSnapshot {
	snapshots := make([]types.WorkloadSnapshot, 0, len(workloads))
	for _, info := range workloads {
		snapshots = append(snapshots, types.WorkloadSnapshot{
			Namespace:       info.Key.Namespace,
			Kind:            info.Key.Kind,
			Name:            info.Key.Name,
			DesiredReplicas: info.DesiredReplicas,
			ReadyReplicas:   info.ReadyReplicas,
			PodStatuses:     info.PodStatuses,
			TotalRestarts:   info.TotalRestarts,
			BarePod:         info.BarePod,
		})
	}

	// Sort for deterministic JSON output — critical for before/after comparison.
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].Namespace != snapshots[j].Namespace {
			return snapshots[i].Namespace < snapshots[j].Namespace
		}
		if snapshots[i].Kind != snapshots[j].Kind {
			return snapshots[i].Kind < snapshots[j].Kind
		}
		return snapshots[i].Name < snapshots[j].Name
	})

	return snapshots
}

func buildPDBSnapshots(pdbs *policyv1.PodDisruptionBudgetList) []types.PDBSnapshot {
	snapshots := make([]types.PDBSnapshot, 0, len(pdbs.Items))
	for i := range pdbs.Items {
		pdb := &pdbs.Items[i]
		snapshots = append(snapshots, types.PDBSnapshot{
			Name:               pdb.Name,
			Namespace:          pdb.Namespace,
			DisruptionsAllowed: pdb.Status.DisruptionsAllowed,
			CurrentHealthy:     pdb.Status.CurrentHealthy,
			ExpectedPods:       pdb.Status.ExpectedPods,
		})
	}
	return snapshots
}
