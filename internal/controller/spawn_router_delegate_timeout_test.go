package controller

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	sympoziumv1alpha1 "github.com/sympozium-ai/sympozium/api/v1alpha1"
	"github.com/sympozium-ai/sympozium/internal/eventbus"
	"github.com/sympozium-ai/sympozium/internal/ipc"
)

// edgePack builds an Ensemble whose "lead" persona delegates to "researcher".
// relTimeout and relType parameterise the single relationship under test.
func edgePack(relType, relTimeout string) *sympoziumv1alpha1.Ensemble {
	return &sympoziumv1alpha1.Ensemble{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pack", Namespace: "default"},
		Spec: sympoziumv1alpha1.EnsembleSpec{
			Relationships: []sympoziumv1alpha1.AgentConfigRelationship{
				{Source: "lead", Target: "researcher", Type: relType, Timeout: relTimeout},
			},
			SharedMemory: &sympoziumv1alpha1.SharedMemorySpec{
				Enabled: true,
				Membrane: &sympoziumv1alpha1.MembraneSpec{
					CircuitBreaker: &sympoziumv1alpha1.CircuitBreakerSpec{ConsecutiveFailures: 2},
				},
			},
		},
		Status: sympoziumv1alpha1.EnsembleStatus{
			InstalledAgentConfigs: []sympoziumv1alpha1.InstalledAgentConfig{
				{Name: "lead", InstanceName: "pack-lead"},
				{Name: "researcher", InstanceName: "pack-researcher"},
			},
		},
	}
}

func TestDelegationEdgeTimeout(t *testing.T) {
	tests := []struct {
		name         string
		relType      string
		relTimeout   string
		instanceName string
		target       string
		wantD        time.Duration
		wantOK       bool
	}{
		{
			name: "delegation edge with timeout", relType: "delegation", relTimeout: "15m",
			instanceName: "pack-lead", target: "researcher",
			wantD: 15 * time.Minute, wantOK: true,
		},
		{
			name: "edge declares no timeout", relType: "delegation", relTimeout: "",
			instanceName: "pack-lead", target: "researcher",
			wantOK: false,
		},
		{
			// The spawner authorizes sequential edges too, but they carry no
			// delegate timeout.
			name: "sequential edge is not a delegate timeout", relType: "sequential", relTimeout: "15m",
			instanceName: "pack-lead", target: "researcher",
			wantOK: false,
		},
		{
			name: "parent instance is not an installed persona", relType: "delegation", relTimeout: "15m",
			instanceName: "some-other-agent", target: "researcher",
			wantOK: false,
		},
		{
			name: "target has no edge from this source", relType: "delegation", relTimeout: "15m",
			instanceName: "pack-lead", target: "writer",
			wantOK: false,
		},
		{
			// Must not be read as zero, which would expire the delegation at once.
			name: "malformed duration is ignored", relType: "delegation", relTimeout: "15 minutes",
			instanceName: "pack-lead", target: "researcher",
			wantOK: false,
		},
		{
			name: "zero duration is ignored", relType: "delegation", relTimeout: "0s",
			instanceName: "pack-lead", target: "researcher",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sr := newTestSpawnRouter(t, edgePack(tt.relType, tt.relTimeout))
			d, ok := sr.delegationEdgeTimeout(context.Background(), "default", tt.instanceName, "my-pack", tt.target)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (d=%s)", ok, tt.wantOK, d)
			}
			if ok && d != tt.wantD {
				t.Errorf("d = %s, want %s", d, tt.wantD)
			}
		})
	}
}

func TestDelegationEdgeTimeout_MissingEnsemble(t *testing.T) {
	sr := newTestSpawnRouter(t)
	if _, ok := sr.delegationEdgeTimeout(context.Background(), "default", "pack-lead", "absent-pack", "researcher"); ok {
		t.Error("a missing Ensemble should not yield an edge timeout")
	}
}

// delegationFixture wires a parent run, a running child, and an ensemble with
// one pending delegation between them.
func delegationFixture(t *testing.T) (*SpawnRouter, *recordingEventBus) {
	t.Helper()

	parentRun := &sympoziumv1alpha1.AgentRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "parent-run",
			Namespace: "default",
			Labels:    map[string]string{"sympozium.ai/ensemble": "my-pack"},
		},
		Status: sympoziumv1alpha1.AgentRunStatus{
			Phase: sympoziumv1alpha1.AgentRunPhaseAwaitingDelegate,
			Delegates: []sympoziumv1alpha1.DelegateStatus{
				{ChildRunName: "child-run", TargetPersona: "researcher", Phase: sympoziumv1alpha1.AgentRunPhasePending},
			},
		},
	}
	childRun := &sympoziumv1alpha1.AgentRun{
		ObjectMeta: metav1.ObjectMeta{Name: "child-run", Namespace: "default"},
	}

	sr := newTestSpawnRouter(t, edgePack("delegation", "15m"), parentRun, childRun)
	bus := &recordingEventBus{}
	sr.EventBus = bus

	sr.pending.Store("child-run", &pendingDelegation{
		RequestID:       "req-1",
		ParentRunID:     "parent-run",
		ParentNamespace: "default",
	})
	return sr, bus
}

// decodeDelegateResult finds the delegate result published for parent-run.
func decodeDelegateResult(t *testing.T, bus *recordingEventBus) (ipc.DelegateResult, bool) {
	t.Helper()
	want := eventbus.TopicAgentDelegateResult + ".parent-run"
	for _, ev := range bus.published {
		if ev.Topic != want {
			continue
		}
		var res ipc.DelegateResult
		if err := json.Unmarshal(ev.Event.Data, &res); err != nil {
			t.Fatalf("delegate result is not decodable: %v", err)
		}
		return res, true
	}
	return ipc.DelegateResult{}, false
}

func TestExpireDelegation_UnblocksParentAndDeletesChild(t *testing.T) {
	sr, bus := delegationFixture(t)
	ctx := context.Background()

	sr.expireDelegation(ctx, "child-run", "researcher", 15*time.Minute)

	// The parent's blocking delegate_to_persona call must be released with an error.
	res, ok := decodeDelegateResult(t, bus)
	if !ok {
		t.Fatal("no delegate result published; the parent tool would block to its run deadline")
	}
	if res.Status != "error" {
		t.Errorf("status = %q, want error", res.Status)
	}
	if res.RequestID != "req-1" {
		t.Errorf("requestId = %q, want req-1", res.RequestID)
	}
	if res.Error != "timed out after 15m0s (Ensemble relationship timeout)" {
		t.Errorf("unexpected error text: %q", res.Error)
	}

	// The pending entry is claimed, so a late completion cannot double-publish.
	if _, still := sr.pending.Load("child-run"); still {
		t.Error("pending entry should be claimed by the expiry")
	}

	// The child is deleted rather than left burning tokens.
	var child sympoziumv1alpha1.AgentRun
	err := sr.Client.Get(ctx, types.NamespacedName{Name: "child-run", Namespace: "default"}, &child)
	if err == nil {
		t.Error("timed-out child run should be deleted")
	}

	// A timeout counts as a delegation failure for the circuit breaker.
	var pack sympoziumv1alpha1.Ensemble
	if err := sr.Client.Get(ctx, types.NamespacedName{Name: "my-pack", Namespace: "default"}, &pack); err != nil {
		t.Fatalf("get ensemble: %v", err)
	}
	if pack.Status.ConsecutiveDelegateFailures != 1 {
		t.Errorf("ConsecutiveDelegateFailures = %d, want 1", pack.Status.ConsecutiveDelegateFailures)
	}

	// The parent's delegate entry records the failure.
	var parent sympoziumv1alpha1.AgentRun
	if err := sr.Client.Get(ctx, types.NamespacedName{Name: "parent-run", Namespace: "default"}, &parent); err != nil {
		t.Fatalf("get parent: %v", err)
	}
	if len(parent.Status.Delegates) != 1 || parent.Status.Delegates[0].Phase != sympoziumv1alpha1.AgentRunPhaseFailed {
		t.Errorf("parent delegate status not marked failed: %+v", parent.Status.Delegates)
	}
}

func TestExpireDelegation_NoopAfterChildSettled(t *testing.T) {
	sr, bus := delegationFixture(t)
	ctx := context.Background()

	// The completion handler got there first and claimed the entry.
	sr.pending.LoadAndDelete("child-run")

	sr.expireDelegation(ctx, "child-run", "researcher", 15*time.Minute)

	if _, ok := decodeDelegateResult(t, bus); ok {
		t.Error("expiry must not publish a second delegate result")
	}
	var child sympoziumv1alpha1.AgentRun
	if err := sr.Client.Get(ctx, types.NamespacedName{Name: "child-run", Namespace: "default"}, &child); err != nil {
		t.Error("a settled child must not be deleted by a late timer")
	}
}

func TestHandleChildCompleted_StopsEdgeTimer(t *testing.T) {
	sr, bus := delegationFixture(t)

	// Arm a timer that would not otherwise fire during the test.
	fired := make(chan struct{})
	timer := time.AfterFunc(time.Hour, func() { close(fired) })
	val, _ := sr.pending.Load("child-run")
	val.(*pendingDelegation).timer = timer

	sr.handleChildCompleted(context.Background(), &eventbus.Event{
		Metadata: map[string]string{"agentRunID": "child-run"},
		Data:     json.RawMessage(`{"status":"success","response":"done"}`),
	})

	// Stop reports false once the timer is already stopped or fired. It has not
	// fired, so a false here means handleChildCompleted stopped it.
	if timer.Stop() {
		t.Error("edge timer still armed after the child completed")
	}

	res, ok := decodeDelegateResult(t, bus)
	if !ok || res.Status != "success" || res.Response != "done" {
		t.Errorf("child completion should deliver a success result, got %+v (found=%v)", res, ok)
	}
}

func TestHandleChildFailed_StopsEdgeTimer(t *testing.T) {
	sr, _ := delegationFixture(t)

	timer := time.AfterFunc(time.Hour, func() {})
	val, _ := sr.pending.Load("child-run")
	val.(*pendingDelegation).timer = timer

	sr.handleChildFailed(context.Background(), &eventbus.Event{
		Metadata: map[string]string{"agentRunID": "child-run"},
		Data:     json.RawMessage(`{"error":"boom"}`),
	})

	if timer.Stop() {
		t.Error("edge timer still armed after the child failed")
	}
}
