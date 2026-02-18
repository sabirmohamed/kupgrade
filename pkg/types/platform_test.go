package types

import "testing"

func TestDetectPlatform(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		want       Platform
	}{
		{"AKS", "azure:///subscriptions/dbb8cb65/resourceGroups/rg/providers/Microsoft.Compute/virtualMachineScaleSets/vmss/virtualMachines/3", PlatformAKS},
		{"EKS", "aws:///eu-north-1a/i-0abc123def456", PlatformEKS},
		{"GKE", "gce://project-f7bba519/europe-north2-a/gke-node-abc123", PlatformGKE},
		{"unknown provider", "digitalocean://droplets/12345", PlatformUnknown},
		{"empty string", "", PlatformUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectPlatform(tt.providerID)
			if got != tt.want {
				t.Errorf("DetectPlatform(%q) = %q, want %q", tt.providerID, got, tt.want)
			}
		})
	}
}

func TestPlatform_NodeReregisters(t *testing.T) {
	tests := []struct {
		platform Platform
		want     bool
	}{
		{PlatformAKS, true},
		{PlatformEKS, false},
		{PlatformGKE, false},
		{PlatformUnknown, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.platform), func(t *testing.T) {
			if got := tt.platform.NodeReregisters(); got != tt.want {
				t.Errorf("%q.NodeReregisters() = %v, want %v", tt.platform, got, tt.want)
			}
		})
	}
}
