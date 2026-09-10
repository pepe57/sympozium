package controller

import (
	"context"
	api "github.com/sympozium-ai/sympozium/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func TestHarnessConnectionPinsPersistentSession(t *testing.T) {
	ctx := context.Background()
	runtime := readySessionRuntime()
	runtime.Spec.Model = nil
	connection := &api.ModelConnection{ObjectMeta: metav1.ObjectMeta{Name: "framework", Namespace: "default"}, Spec: api.ModelConnectionSpec{Provider: "llama-server", Protocol: "openai-chat", Endpoint: "http://framework:8080/v1/chat/completions", Models: []string{"qwen"}}}
	agent := &api.Agent{ObjectMeta: metav1.ObjectMeta{Name: "hermes", Namespace: "default"}, Spec: api.AgentSpec{Execution: &api.AgentExecutionDefaults{Backend: "job", ModelConnectionRef: "framework"}, Agents: api.AgentsSpec{Default: api.AgentConfig{Model: "qwen"}}}}
	session := &api.HarnessSession{ObjectMeta: metav1.ObjectMeta{Name: "hermes-chat", Namespace: "default"}, Spec: api.HarnessSessionSpec{AgentRef: "hermes", RuntimeRef: runtime.Name}}
	store := fake.NewClientBuilder().WithScheme(harnessSessionTestScheme(t)).WithObjects(runtime, connection, agent, session).Build()
	reconciler := &HarnessSessionReconciler{Client: store}
	_, resolved, reason, err := reconciler.resolveInputs(ctx, session)
	if err != nil || reason != "" || resolved.Spec.Model.BaseURL != "http://framework:8080/v1" || resolved.Spec.Model.AuthSecretRef != "" {
		t.Fatalf("resolve: %s %v %+v", reason, err, resolved)
	}
	if session.Annotations["sympozium.ai/model-connection-revision"] == "" {
		t.Fatal("session did not pin connection")
	}
	connection.Spec.Endpoint = "http://another-host:8080/v1/chat/completions"
	_ = store.Update(ctx, connection)
	if _, _, reason, err := reconciler.resolveInputs(ctx, session); err != nil || reason == "" {
		t.Fatalf("changed connection admitted: %s %v", reason, err)
	}
}
