package snapshot

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

func TestSaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()

	original := &types.Snapshot{
		SchemaVersion: types.SchemaVersion,
		Timestamp:     time.Date(2026, 1, 31, 14, 30, 0, 0, time.UTC),
		Context:       "prod-cluster",
		ServerVersion: "v1.31.2",
		Nodes: []types.NodeSnapshot{
			{Name: "node-1", Version: "v1.31.2", Ready: true},
			{Name: "node-2", Version: "v1.31.2", Ready: true, Conditions: []string{"MemoryPressure"}},
		},
		Workloads: []types.WorkloadSnapshot{
			{
				Namespace:       "default",
				Kind:            "Deployment",
				Name:            "web-app",
				DesiredReplicas: 3,
				ReadyReplicas:   3,
				PodStatuses:     map[string]int{"Running": 3},
				TotalRestarts:   0,
			},
			{
				Namespace:       "default",
				Kind:            "Pod",
				Name:            "bare-pod",
				DesiredReplicas: 1,
				ReadyReplicas:   1,
				PodStatuses:     map[string]int{"Running": 1},
				BarePod:         true,
			},
		},
		PDBs: []types.PDBSnapshot{
			{
				Name:               "web-pdb",
				Namespace:          "default",
				DisruptionsAllowed: 1,
				CurrentHealthy:     3,
				ExpectedPods:       3,
			},
		},
	}

	// Save.
	path, err := Save(original, tempDir)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("snapshot file not created: %s", path)
	}

	// Load.
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	// Verify round-trip.
	if loaded.SchemaVersion != original.SchemaVersion {
		t.Errorf("schema version = %d, want %d", loaded.SchemaVersion, original.SchemaVersion)
	}
	if loaded.Context != original.Context {
		t.Errorf("context = %q, want %q", loaded.Context, original.Context)
	}
	if loaded.ServerVersion != original.ServerVersion {
		t.Errorf("server version = %q, want %q", loaded.ServerVersion, original.ServerVersion)
	}
	if len(loaded.Nodes) != len(original.Nodes) {
		t.Errorf("nodes count = %d, want %d", len(loaded.Nodes), len(original.Nodes))
	}
	if len(loaded.Workloads) != len(original.Workloads) {
		t.Errorf("workloads count = %d, want %d", len(loaded.Workloads), len(original.Workloads))
	}
	if len(loaded.PDBs) != len(original.PDBs) {
		t.Errorf("pdbs count = %d, want %d", len(loaded.PDBs), len(original.PDBs))
	}

	// Verify workload details.
	for i, workload := range loaded.Workloads {
		orig := original.Workloads[i]
		if workload.Kind != orig.Kind {
			t.Errorf("workload[%d] kind = %q, want %q", i, workload.Kind, orig.Kind)
		}
		if workload.Name != orig.Name {
			t.Errorf("workload[%d] name = %q, want %q", i, workload.Name, orig.Name)
		}
		if workload.BarePod != orig.BarePod {
			t.Errorf("workload[%d] barePod = %v, want %v", i, workload.BarePod, orig.BarePod)
		}
		for status, count := range orig.PodStatuses {
			if loadedCount, ok := workload.PodStatuses[status]; !ok || loadedCount != count {
				t.Errorf("workload[%d] podStatuses[%s] = %d, want %d", i, status, loadedCount, count)
			}
		}
	}
}

func TestFilename(t *testing.T) {
	tests := []struct {
		name      string
		context   string
		timestamp time.Time
		want      string
	}{
		{
			name:      "simple context",
			context:   "prod-cluster",
			timestamp: time.Date(2026, 1, 31, 14, 30, 5, 0, time.UTC),
			want:      "prod-cluster-2026-01-31T14-30-05.json",
		},
		{
			name:      "context with colons",
			context:   "gke_project_zone_cluster",
			timestamp: time.Date(2026, 1, 31, 14, 30, 5, 0, time.UTC),
			want:      "gke_project_zone_cluster-2026-01-31T14-30-05.json",
		},
		{
			name:      "context with slashes",
			context:   "arn:aws:eks:us-east-1:123:cluster/prod",
			timestamp: time.Date(2026, 1, 31, 14, 30, 5, 0, time.UTC),
			want:      "arn-aws-eks-us-east-1-123-cluster-prod-2026-01-31T14-30-05.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Filename(tt.context, tt.timestamp)
			if got != tt.want {
				t.Errorf("Filename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	nestedDir := filepath.Join(tempDir, "deep", "nested", "dir")

	snapshot := &types.Snapshot{
		SchemaVersion: types.SchemaVersion,
		Timestamp:     time.Date(2026, 1, 31, 14, 30, 0, 0, time.UTC),
		Context:       "test",
	}

	path, err := Save(snapshot, nestedDir)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("snapshot file not created: %s", path)
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/snapshot.json")
	if err == nil {
		t.Fatal("expected error loading nonexistent file")
	}
}

func TestSaveJSON(t *testing.T) {
	tempDir := t.TempDir()

	snapshot := &types.Snapshot{
		SchemaVersion: types.SchemaVersion,
		Timestamp:     time.Date(2026, 1, 31, 14, 30, 0, 0, time.UTC),
		Context:       "test",
		ServerVersion: "v1.31.0",
		Nodes:         []types.NodeSnapshot{},
		Workloads:     []types.WorkloadSnapshot{},
		PDBs:          []types.PDBSnapshot{},
	}

	path, err := Save(snapshot, tempDir)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Verify the JSON is human-readable (indented).
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}

	content := string(data)
	// MarshalIndent uses 2-space indentation.
	if len(content) < 10 {
		t.Fatal("JSON content too short")
	}
	// Verify it starts with a { and contains indentation.
	if content[0] != '{' {
		t.Errorf("expected JSON to start with {, got %c", content[0])
	}
	if content[1] != '\n' {
		t.Errorf("expected newline after opening brace (indented format)")
	}
}
