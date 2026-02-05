package stage

import (
	"testing"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testCurrentVersion = "v1.32.9"
	testTargetVersion  = "v1.33.2"
)

func newTestNode(name, version string, ready, schedulable bool) *corev1.Node {
	conditions := []corev1.NodeCondition{
		{
			Type:   corev1.NodeReady,
			Status: corev1.ConditionFalse,
		},
	}
	if ready {
		conditions[0].Status = corev1.ConditionTrue
	}

	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.NodeSpec{
			Unschedulable: !schedulable,
		},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion: version,
			},
			Conditions: conditions,
		},
	}
}

func TestStageReimaging_ConstantValue(t *testing.T) {
	if types.StageReimaging != "REIMAGING" {
		t.Errorf("StageReimaging = %q, want %q", types.StageReimaging, "REIMAGING")
	}
}

func TestAllStages_IncludesReimaging(t *testing.T) {
	stages := types.AllStages()
	found := false
	for _, s := range stages {
		if s == types.StageReimaging {
			found = true
			break
		}
	}
	if !found {
		t.Error("AllStages() does not include StageReimaging")
	}

	// Verify no StageUpgrading reference
	for _, s := range stages {
		if s == "UPGRADING" {
			t.Error("AllStages() still contains UPGRADING")
		}
	}
}

func TestComputeStage_Reimaging(t *testing.T) {
	c := New("")
	c.SetTargetVersion(testCurrentVersion)
	c.SetTargetVersion(testTargetVersion)

	tests := []struct {
		name      string
		node      *corev1.Node
		wantStage types.NodeStage
	}{
		{
			name:      "ready schedulable at current version",
			node:      newTestNode("node-1", testCurrentVersion, true, true),
			wantStage: types.StageReady,
		},
		{
			name:      "ready schedulable at target version",
			node:      newTestNode("node-2", testTargetVersion, true, true),
			wantStage: types.StageComplete,
		},
		{
			name:      "not ready returns REIMAGING",
			node:      newTestNode("node-3", testCurrentVersion, false, true),
			wantStage: types.StageReimaging,
		},
		{
			name:      "unschedulable returns CORDONED",
			node:      newTestNode("node-4", testCurrentVersion, true, false),
			wantStage: types.StageCordoned,
		},
		{
			name:      "not ready and unschedulable returns REIMAGING",
			node:      newTestNode("node-5", testCurrentVersion, false, false),
			wantStage: types.StageReimaging,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.ComputeStage(tt.node)
			if got != tt.wantStage {
				t.Errorf("ComputeStage() = %v, want %v", got, tt.wantStage)
			}
		})
	}
}
