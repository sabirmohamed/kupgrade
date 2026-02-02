package snapshot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindLatestPicksMostRecent(t *testing.T) {
	dir := t.TempDir()

	// Create snapshot files with different timestamps.
	files := []string{
		"prod-2024-01-26T14-30-00.json",
		"prod-2024-01-27T09-00-00.json",
		"prod-2024-01-25T08-00-00.json",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	path, err := FindLatest(dir, "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(dir, "prod-2024-01-27T09-00-00.json")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}

func TestFindLatestFiltersByContext(t *testing.T) {
	dir := t.TempDir()

	files := []string{
		"prod-2024-01-26T14-30-00.json",
		"staging-2024-01-27T09-00-00.json",
		"prod-2024-01-25T08-00-00.json",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	path, err := FindLatest(dir, "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(dir, "prod-2024-01-26T14-30-00.json")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}

func TestFindLatestNoSnapshots(t *testing.T) {
	dir := t.TempDir()

	_, err := FindLatest(dir, "prod")
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func TestFindLatestSanitizesContext(t *testing.T) {
	dir := t.TempDir()

	// Context with special characters should be sanitized.
	if err := os.WriteFile(filepath.Join(dir, "arn-aws-eks-us-east-1-123-cluster-prod-2024-01-26T14-30-00.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := FindLatest(dir, "arn:aws:eks:us-east-1:123:cluster/prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if path == "" {
		t.Error("expected a path, got empty string")
	}
}
