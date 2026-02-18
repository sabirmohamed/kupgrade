package tui

import "github.com/sabirmohamed/kupgrade/pkg/types"

// detectPlatform returns the cloud platform from a node's providerID.
// Returns "AKS", "EKS", "GKE", or "" for unknown.
func detectPlatform(providerID string) string {
	return string(types.DetectPlatform(providerID))
}
