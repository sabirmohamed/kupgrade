package watcher

import (
	"testing"
)

func TestUpgradeRelevantReasons_AKSEvents(t *testing.T) {
	aksReasons := []string{"Upgrade", "RemovingNode", "RegisteredNode", "Surge"}
	for _, reason := range aksReasons {
		if !upgradeRelevantReasons[reason] {
			t.Errorf("upgradeRelevantReasons missing AKS reason %q", reason)
		}
	}
}

func TestSurgeCreatedPattern(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		wantNode string
		wantPool string
		wantOK   bool
	}{
		{
			name:     "standard surge created",
			message:  "Created a surge node aks-agentpool-32099259-vmss000005 for agentpool agentpool",
			wantNode: "aks-agentpool-32099259-vmss000005",
			wantPool: "agentpool",
			wantOK:   true,
		},
		{
			name:     "different pool name",
			message:  "Created a surge node aks-stdpool-12345678-vmss000002 for agentpool stdpool",
			wantNode: "aks-stdpool-12345678-vmss000002",
			wantPool: "stdpool",
			wantOK:   true,
		},
		{
			name:    "non-matching message",
			message: "Some other message about nodes",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := surgeCreatedPattern.FindStringSubmatch(tt.message)
			if tt.wantOK {
				if len(matches) < 3 {
					t.Fatalf("expected match with pool name, got %v for %q", matches, tt.message)
				}
				if matches[1] != tt.wantNode {
					t.Errorf("got node name %q, want %q", matches[1], tt.wantNode)
				}
				if matches[2] != tt.wantPool {
					t.Errorf("got pool name %q, want %q", matches[2], tt.wantPool)
				}
			} else {
				if len(matches) >= 2 {
					t.Errorf("expected no match for %q, got %v", tt.message, matches)
				}
			}
		})
	}
}

func TestSurgeRemovingPattern(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		wantNode string
		wantOK   bool
	}{
		{
			name:     "standard surge removing",
			message:  "Removing surge node: aks-agentpool-32099259-vmss000005",
			wantNode: "aks-agentpool-32099259-vmss000005",
			wantOK:   true,
		},
		{
			name:    "non-matching message",
			message: "Removing node from controller",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := surgeRemovingPattern.FindStringSubmatch(tt.message)
			if tt.wantOK {
				if len(matches) < 2 {
					t.Fatalf("expected match, got none for %q", tt.message)
				}
				if matches[1] != tt.wantNode {
					t.Errorf("got node name %q, want %q", matches[1], tt.wantNode)
				}
			} else {
				if len(matches) >= 2 {
					t.Errorf("expected no match for %q, got %v", tt.message, matches)
				}
			}
		})
	}
}
