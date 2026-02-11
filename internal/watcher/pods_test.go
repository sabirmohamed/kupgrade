package watcher

import (
	"testing"
	"time"

	"github.com/sabirmohamed/kupgrade/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mockMigrationTracker implements MigrationTracker for tests
type mockMigrationTracker struct{}

func (m *mockMigrationTracker) OnPodDelete(_ *corev1.Pod)               {}
func (m *mockMigrationTracker) OnPodAdd(_ *corev1.Pod) *types.Migration { return nil }
func (m *mockMigrationTracker) Cleanup(_ time.Duration)                 {}

func newTestPodWatcher() (*PodWatcher, *mockEmitter) {
	emitter := &mockEmitter{}
	return &PodWatcher{
		emitter:          emitter,
		stages:           &mockStageComputer{targetVersion: testTargetVersion},
		migrations:       &mockMigrationTracker{},
		lastEmittedEvent: make(map[string]types.EventType),
	}, emitter
}

func makePod(name, namespace, nodeName string, ready bool) *corev1.Pod {
	conditions := []corev1.PodCondition{}
	if ready {
		conditions = append(conditions, corev1.PodCondition{
			Type:   corev1.PodReady,
			Status: corev1.ConditionTrue,
		})
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
		Status: corev1.PodStatus{
			Conditions: conditions,
		},
	}
}

func TestPodWatcher_DuplicateReadyEventsSuppressed(t *testing.T) {
	w, emitter := newTestPodWatcher()

	oldPod := makePod("web-1", "default", "node-1", false)
	newPod := makePod("web-1", "default", "node-1", true)

	// First Ready transition should emit
	w.onUpdate(oldPod, newPod)

	readyCount := countEventType(emitter.events, types.EventPodReady)
	if readyCount != 1 {
		t.Fatalf("expected 1 POD_READY event, got %d", readyCount)
	}

	// Second identical transition (informer fires again) should be suppressed
	w.onUpdate(oldPod, newPod)

	readyCount = countEventType(emitter.events, types.EventPodReady)
	if readyCount != 1 {
		t.Errorf("expected 1 POD_READY event after dedup, got %d", readyCount)
	}
}

func TestPodWatcher_DifferentTransitionsNotSuppressed(t *testing.T) {
	w, emitter := newTestPodWatcher()

	notReadyPod := makePod("web-1", "default", "node-1", false)
	readyPod := makePod("web-1", "default", "node-1", true)
	failedPod := makePod("web-1", "default", "node-1", false)
	failedPod.Status.Phase = corev1.PodFailed

	// Ready transition
	w.onUpdate(notReadyPod, readyPod)
	if countEventType(emitter.events, types.EventPodReady) != 1 {
		t.Fatal("expected POD_READY event")
	}

	// Failed transition — different event type, should emit
	w.onUpdate(readyPod, failedPod)
	if countEventType(emitter.events, types.EventPodFailed) != 1 {
		t.Error("expected POD_FAILED event after POD_READY")
	}
}

func TestPodWatcher_DeleteClearsDedupTracking(t *testing.T) {
	w, emitter := newTestPodWatcher()

	oldPod := makePod("web-1", "default", "node-1", false)
	newPod := makePod("web-1", "default", "node-1", true)

	// First Ready
	w.onUpdate(oldPod, newPod)
	if countEventType(emitter.events, types.EventPodReady) != 1 {
		t.Fatal("expected 1 POD_READY event")
	}

	// Delete clears tracking
	w.onDelete(newPod)

	// Re-created pod with same name — Ready should emit again
	w.onUpdate(oldPod, newPod)
	if countEventType(emitter.events, types.EventPodReady) != 2 {
		t.Errorf("expected 2 POD_READY events after delete+recreate, got %d",
			countEventType(emitter.events, types.EventPodReady))
	}
}

func TestPodWatcher_DuplicateEvictionSuppressed(t *testing.T) {
	w, emitter := newTestPodWatcher()

	normalPod := makePod("web-1", "default", "node-1", true)
	evictedPod := makePod("web-1", "default", "node-1", true)
	evictedPod.Status.Reason = "Evicted"

	// First eviction
	w.onUpdate(normalPod, evictedPod)
	if countEventType(emitter.events, types.EventPodEvicted) != 1 {
		t.Fatal("expected 1 POD_EVICTED event")
	}

	// Duplicate eviction update
	w.onUpdate(normalPod, evictedPod)
	if countEventType(emitter.events, types.EventPodEvicted) != 1 {
		t.Errorf("expected 1 POD_EVICTED event after dedup, got %d",
			countEventType(emitter.events, types.EventPodEvicted))
	}
}

func TestPodWatcher_ReadyFlapCycleEmitsBoth(t *testing.T) {
	w, emitter := newTestPodWatcher()

	notReadyPod := makePod("web-1", "default", "node-1", false)
	readyPod := makePod("web-1", "default", "node-1", true)

	// First Ready
	w.onUpdate(notReadyPod, readyPod)
	if countEventType(emitter.events, types.EventPodReady) != 1 {
		t.Fatal("expected 1 POD_READY event")
	}

	// Pod becomes NotReady (flap) — clears dedup tracking
	w.onUpdate(readyPod, notReadyPod)

	// Pod becomes Ready again — should emit because tracking was cleared
	w.onUpdate(notReadyPod, readyPod)
	if countEventType(emitter.events, types.EventPodReady) != 2 {
		t.Errorf("expected 2 POD_READY events after flap, got %d",
			countEventType(emitter.events, types.EventPodReady))
	}
}

func countEventType(events []types.Event, eventType types.EventType) int {
	count := 0
	for _, e := range events {
		if e.Type == eventType {
			count++
		}
	}
	return count
}
