package snapshot

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	testContext   = "test-cluster"
	testNamespace = "default"
)

func boolPtr(value bool) *bool    { return &value }
func int32Ptr(value int32) *int32 { return &value }

func newPod(name, namespace, nodeName string, phase corev1.PodPhase, ownerRefs ...metav1.OwnerReference) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: ownerRefs,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
		Status: corev1.PodStatus{
			Phase: phase,
		},
	}
}

func controllerRef(name, kind string, uid types.UID) metav1.OwnerReference {
	return metav1.OwnerReference{
		Name:       name,
		Kind:       kind,
		UID:        uid,
		Controller: boolPtr(true),
	}
}

func TestCollectOwnerGrouping(t *testing.T) {
	deploymentUID := types.UID("deploy-uid-1")
	replicaSetUID := types.UID("rs-uid-1")

	objects := []runtime.Object{
		// Node
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.31.0"},
			},
		},
		// Deployment
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-app",
				Namespace: testNamespace,
				UID:       deploymentUID,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(3),
			},
			Status: appsv1.DeploymentStatus{
				ReadyReplicas: 3,
			},
		},
		// ReplicaSet owned by Deployment
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-app-abc123",
				Namespace: testNamespace,
				UID:       replicaSetUID,
				OwnerReferences: []metav1.OwnerReference{
					{Name: "web-app", Kind: "Deployment", UID: deploymentUID, Controller: boolPtr(true)},
				},
			},
		},
		// Pods owned by ReplicaSet (should resolve to Deployment)
		newPod("web-app-abc123-x1", testNamespace, "node-1", corev1.PodRunning,
			controllerRef("web-app-abc123", "ReplicaSet", replicaSetUID)),
		newPod("web-app-abc123-x2", testNamespace, "node-1", corev1.PodRunning,
			controllerRef("web-app-abc123", "ReplicaSet", replicaSetUID)),
		newPod("web-app-abc123-x3", testNamespace, "node-1", corev1.PodRunning,
			controllerRef("web-app-abc123", "ReplicaSet", replicaSetUID)),
	}

	clientset := fake.NewSimpleClientset(objects...)
	snapshot, warnings, err := Collect(context.Background(), clientset, testContext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}

	// Should have exactly one workload grouped under the Deployment.
	if len(snapshot.Workloads) != 1 {
		t.Fatalf("workloads count = %d, want 1", len(snapshot.Workloads))
	}

	workload := snapshot.Workloads[0]
	if workload.Kind != "Deployment" {
		t.Errorf("workload kind = %q, want %q", workload.Kind, "Deployment")
	}
	if workload.Name != "web-app" {
		t.Errorf("workload name = %q, want %q", workload.Name, "web-app")
	}
	if workload.PodStatuses["Running"] != 3 {
		t.Errorf("running pods = %d, want 3", workload.PodStatuses["Running"])
	}
	if workload.DesiredReplicas != 3 {
		t.Errorf("desired replicas = %d, want 3", workload.DesiredReplicas)
	}
}

func TestCollectBarePods(t *testing.T) {
	objects := []runtime.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.31.0"},
			},
		},
		// Bare pod with no owner
		newPod("standalone-pod", testNamespace, "node-1", corev1.PodRunning),
	}

	clientset := fake.NewSimpleClientset(objects...)
	snapshot, warnings, err := Collect(context.Background(), clientset, testContext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(warnings) != 1 {
		t.Fatalf("warnings count = %d, want 1", len(warnings))
	}

	if len(snapshot.Workloads) != 1 {
		t.Fatalf("workloads count = %d, want 1", len(snapshot.Workloads))
	}

	workload := snapshot.Workloads[0]
	if workload.Kind != "Pod" {
		t.Errorf("workload kind = %q, want %q", workload.Kind, "Pod")
	}
	if workload.Name != "standalone-pod" {
		t.Errorf("workload name = %q, want %q", workload.Name, "standalone-pod")
	}
	if !workload.BarePod {
		t.Error("expected BarePod = true")
	}
}

func TestCollectCronJobChain(t *testing.T) {
	cronJobUID := types.UID("cron-uid-1")
	jobUID := types.UID("job-uid-1")

	objects := []runtime.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.31.0"},
			},
		},
		// CronJob
		&batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nightly-backup",
				Namespace: testNamespace,
				UID:       cronJobUID,
			},
		},
		// Job owned by CronJob
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nightly-backup-12345",
				Namespace: testNamespace,
				UID:       jobUID,
				OwnerReferences: []metav1.OwnerReference{
					{Name: "nightly-backup", Kind: "CronJob", UID: cronJobUID, Controller: boolPtr(true)},
				},
			},
		},
		// Pod owned by Job (should resolve to CronJob)
		newPod("nightly-backup-12345-abc", testNamespace, "node-1", corev1.PodSucceeded,
			controllerRef("nightly-backup-12345", "Job", jobUID)),
	}

	clientset := fake.NewSimpleClientset(objects...)
	snapshot, _, err := Collect(context.Background(), clientset, testContext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(snapshot.Workloads) != 1 {
		t.Fatalf("workloads count = %d, want 1", len(snapshot.Workloads))
	}

	workload := snapshot.Workloads[0]
	if workload.Kind != "CronJob" {
		t.Errorf("workload kind = %q, want %q", workload.Kind, "CronJob")
	}
	if workload.Name != "nightly-backup" {
		t.Errorf("workload name = %q, want %q", workload.Name, "nightly-backup")
	}
}

func TestCollectNodeSnapshots(t *testing.T) {
	objects := []runtime.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				},
				NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.31.0"},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
					{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue},
				},
				NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.30.0"},
			},
		},
	}

	clientset := fake.NewSimpleClientset(objects...)
	snapshot, _, err := Collect(context.Background(), clientset, testContext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(snapshot.Nodes) != 2 {
		t.Fatalf("nodes count = %d, want 2", len(snapshot.Nodes))
	}

	// Verify schema version.
	if snapshot.SchemaVersion != 1 {
		t.Errorf("schema version = %d, want 1", snapshot.SchemaVersion)
	}

	// Verify context.
	if snapshot.Context != testContext {
		t.Errorf("context = %q, want %q", snapshot.Context, testContext)
	}
}

func TestCollectPDBSnapshots(t *testing.T) {
	objects := []runtime.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.31.0"},
			},
		},
	}

	clientset := fake.NewSimpleClientset(objects...)
	snapshot, _, err := Collect(context.Background(), clientset, testContext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(snapshot.PDBs) != 0 {
		t.Errorf("pdbs count = %d, want 0", len(snapshot.PDBs))
	}
}

func TestCollectInitContainerFailure(t *testing.T) {
	deploymentUID := types.UID("deploy-uid-init")
	replicaSetUID := types.UID("rs-uid-init")

	objects := []runtime.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
				NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.31.0"},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "init-app",
				Namespace: testNamespace,
				UID:       deploymentUID,
			},
			Spec: appsv1.DeploymentSpec{Replicas: int32Ptr(1)},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "init-app-abc",
				Namespace: testNamespace,
				UID:       replicaSetUID,
				OwnerReferences: []metav1.OwnerReference{
					{Name: "init-app", Kind: "Deployment", UID: deploymentUID, Controller: boolPtr(true)},
				},
			},
		},
		// Pod with init container stuck in CrashLoopBackOff.
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "init-app-abc-x1",
				Namespace: testNamespace,
				OwnerReferences: []metav1.OwnerReference{
					controllerRef("init-app-abc", "ReplicaSet", replicaSetUID),
				},
			},
			Spec: corev1.PodSpec{NodeName: "node-1"},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				InitContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "init-db",
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason: "CrashLoopBackOff",
							},
						},
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(objects...)
	snapshot, _, err := Collect(context.Background(), clientset, testContext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(snapshot.Workloads) != 1 {
		t.Fatalf("workloads count = %d, want 1", len(snapshot.Workloads))
	}

	workload := snapshot.Workloads[0]
	if _, ok := workload.PodStatuses["Init:CrashLoopBackOff"]; !ok {
		t.Errorf("expected Init:CrashLoopBackOff in pod statuses, got %v", workload.PodStatuses)
	}
}
