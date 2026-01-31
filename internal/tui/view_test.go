package tui

import (
	"testing"

	"github.com/sabirmohamed/kupgrade/pkg/types"
)

func TestCalcScrollOffset(t *testing.T) {
	tests := []struct {
		name        string
		selected    int
		visibleRows int
		totalItems  int
		want        int
	}{
		{"zero items", 0, 10, 0, 0},
		{"fits in view", 2, 10, 5, 0},
		{"at top", 0, 5, 20, 0},
		{"middle", 7, 5, 20, 3},
		{"near bottom", 18, 5, 20, 14},
		{"at bottom", 19, 5, 20, 15},
		{"selected equals visible", 5, 5, 20, 1},
		{"single visible row", 3, 1, 10, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calcScrollOffset(tt.selected, tt.visibleRows, tt.totalItems)
			if got != tt.want {
				t.Errorf("calcScrollOffset(%d, %d, %d) = %d, want %d",
					tt.selected, tt.visibleRows, tt.totalItems, got, tt.want)
			}
		})
	}
}

func TestStatusColor(t *testing.T) {
	tests := []struct {
		phase string
		want  string // color hex
	}{
		{"Running", string(colorComplete)},
		{"Pending", string(colorCordoned)},
		{"Completed", string(colorTextMuted)},
		{"Succeeded", string(colorTextMuted)},
		{"CrashLoopBackOff", string(colorError)},
		{"Error", string(colorError)},
		{"Failed", string(colorError)},
		{"ImagePullBackOff", string(colorError)},
		{"ErrImagePull", string(colorError)},
		{"OOMKilled", string(colorError)},
		{"RunContainerError", string(colorError)},
		{"CreateContainerError", string(colorError)},
		{"Terminating", string(colorBrightYellow)},
		{"Unknown", string(colorBrightRed)},
		{"Init:0/2", string(colorCyan)},
		{"Init:Error", string(colorCyan)},
		{"PodInitializing", string(colorCyan)},
		{"SomeUnknownPhase", string(colorText)},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			got := string(statusColor(tt.phase))
			if got != tt.want {
				t.Errorf("statusColor(%q) = %q, want %q", tt.phase, got, tt.want)
			}
		})
	}
}

func TestReadyColor(t *testing.T) {
	tests := []struct {
		name string
		pod  types.PodState
		want string
	}{
		{"no containers", types.PodState{TotalContainers: 0}, string(colorTextMuted)},
		{"all ready", types.PodState{ReadyContainers: 3, TotalContainers: 3}, string(colorComplete)},
		{"none ready", types.PodState{ReadyContainers: 0, TotalContainers: 2}, string(colorError)},
		{"partial", types.PodState{ReadyContainers: 1, TotalContainers: 3}, string(colorCordoned)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(readyColor(tt.pod))
			if got != tt.want {
				t.Errorf("readyColor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRestartColor(t *testing.T) {
	tests := []struct {
		name     string
		restarts int
		want     string
	}{
		{"zero", 0, string(colorTextMuted)},
		{"low", 2, string(colorCordoned)},
		{"five", 5, string(colorCordoned)},
		{"high", 10, string(colorError)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(restartColor(tt.restarts))
			if got != tt.want {
				t.Errorf("restartColor(%d) = %q, want %q", tt.restarts, got, tt.want)
			}
		})
	}
}

func TestProbeColor(t *testing.T) {
	tests := []struct {
		name string
		pod  types.PodState
		want string
	}{
		{"no probes", types.PodState{}, string(colorTextMuted)},
		{"readiness ok", types.PodState{HasReadiness: true, ReadinessOK: true}, string(colorComplete)},
		{"readiness fail", types.PodState{HasReadiness: true, ReadinessOK: false}, string(colorError)},
		{"liveness ok", types.PodState{HasLiveness: true, LivenessOK: true}, string(colorComplete)},
		{"liveness fail", types.PodState{HasLiveness: true, LivenessOK: false}, string(colorError)},
		{"both ok", types.PodState{HasReadiness: true, ReadinessOK: true, HasLiveness: true, LivenessOK: true}, string(colorComplete)},
		{"mixed fail", types.PodState{HasReadiness: true, ReadinessOK: true, HasLiveness: true, LivenessOK: false}, string(colorError)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(probeColor(tt.pod))
			if got != tt.want {
				t.Errorf("probeColor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNodeGroupStarts(t *testing.T) {
	pods := []types.PodState{
		{NodeName: "node-a", Name: "pod-1"},
		{NodeName: "node-a", Name: "pod-2"},
		{NodeName: "node-b", Name: "pod-3"},
		{NodeName: "node-b", Name: "pod-4"},
		{NodeName: "node-c", Name: "pod-5"},
	}

	starts := nodeGroupStarts(pods)
	expected := []bool{true, false, true, false, true}

	for i, want := range expected {
		if starts[i] != want {
			t.Errorf("nodeGroupStarts[%d] = %v, want %v", i, starts[i], want)
		}
	}
}

func TestPlainProgressBar(t *testing.T) {
	tests := []struct {
		percent int
		width   int
	}{
		{0, 10},
		{50, 10},
		{100, 10},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := plainProgressBar(tt.percent, tt.width)
			if result == "" {
				t.Error("plainProgressBar returned empty string")
			}
		})
	}
}
