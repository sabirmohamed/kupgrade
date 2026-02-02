package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

const (
	snapshotDirName   = ".kupgrade"
	snapshotSubDir    = "snapshots"
	snapshotDirPerms  = 0o755
	snapshotFilePerms = 0o644
	timestampFormat   = "2006-01-02T15-04-05" // Dash-separated, Windows-safe
)

// DefaultDir returns the default snapshot directory: ~/.kupgrade/snapshots
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("snapshot: user home dir: %w", err)
	}
	return filepath.Join(home, snapshotDirName, snapshotSubDir), nil
}

// Filename generates a snapshot filename from context and timestamp.
// Format: <context>-<timestamp>.json
func Filename(clusterContext string, timestamp time.Time) string {
	// Sanitize context name for filesystem safety.
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '-'
		}
		return r
	}, clusterContext)
	return fmt.Sprintf("%s-%s.json", safe, timestamp.Format(timestampFormat))
}

// Save writes a snapshot to the given directory as JSON.
// Creates the directory if it does not exist.
func Save(snapshot *types.Snapshot, directory string) (string, error) {
	if err := os.MkdirAll(directory, snapshotDirPerms); err != nil {
		return "", fmt.Errorf("snapshot: create directory: %w", err)
	}

	filename := Filename(snapshot.Context, snapshot.Timestamp)
	path := filepath.Join(directory, filename)

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("snapshot: marshal: %w", err)
	}

	if err := os.WriteFile(path, data, snapshotFilePerms); err != nil {
		return "", fmt.Errorf("snapshot: write file: %w", err)
	}

	return path, nil
}

// Load reads a snapshot from the given file path.
func Load(path string) (*types.Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("snapshot: read file: %w", err)
	}

	var snapshot types.Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("snapshot: unmarshal: %w", err)
	}

	return &snapshot, nil
}
