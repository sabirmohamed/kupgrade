package snapshot

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// FindLatest scans the snapshot directory for the most recent snapshot
// matching the given context name. Filenames contain dash-separated
// timestamps that sort lexicographically in chronological order.
//
// Uses a digit-anchored glob pattern (context-[0-9]*) to avoid matching
// contexts that share a prefix (e.g., "prod" vs "prod-cluster").
func FindLatest(directory string, contextName string) (string, error) {
	safe := sanitizeContext(contextName)

	// Glob with digit anchor: timestamps always start with a digit (year).
	// This prevents "prod-*" from matching "prod-cluster-2024-...".
	pattern := filepath.Join(directory, safe+"-[0-9]*.json")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("snapshot: find latest: glob: %w", err)
	}

	// Additional safety: verify each match has the exact context prefix
	// followed by a dash and timestamp, not a longer context name.
	var filtered []string
	prefix := safe + "-"
	for _, match := range matches {
		base := filepath.Base(match)
		if strings.HasPrefix(base, prefix) {
			filtered = append(filtered, match)
		}
	}

	if len(filtered) == 0 {
		return "", fmt.Errorf("snapshot: no snapshots found for context %q in %s", contextName, directory)
	}

	// Lexicographic sort gives chronological order due to timestamp format.
	sort.Strings(filtered)
	return filtered[len(filtered)-1], nil
}
