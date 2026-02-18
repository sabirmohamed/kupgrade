package types

import "strings"

// Platform represents the cloud provider running the Kubernetes cluster.
type Platform string

const (
	PlatformAKS     Platform = "AKS"
	PlatformEKS     Platform = "EKS"
	PlatformGKE     Platform = "GKE"
	PlatformUnknown Platform = ""
)

// DetectPlatform identifies the cloud platform from a node's providerID.
// Provider IDs use prefixes: "azure://" (AKS), "aws://" (EKS), "gce://" (GKE).
func DetectPlatform(providerID string) Platform {
	switch {
	case strings.HasPrefix(providerID, "azure://"):
		return PlatformAKS
	case strings.HasPrefix(providerID, "aws://"):
		return PlatformEKS
	case strings.HasPrefix(providerID, "gce://"):
		return PlatformGKE
	default:
		return PlatformUnknown
	}
}

// NodeReregisters returns true if the platform reimages nodes in-place
// (same node name re-registers after deletion). Only AKS does this.
// EKS and GKE create replacement nodes with new names — deletions are terminal.
func (p Platform) NodeReregisters() bool {
	return p == PlatformAKS
}
