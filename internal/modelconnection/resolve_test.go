package modelconnection

import (
	"context"
	api "github.com/sympozium-ai/sympozium/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func TestConnectionPinsRouteAndRefusesMutation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.AddToScheme(scheme)
	connection := &api.ModelConnection{ObjectMeta: metav1.ObjectMeta{Name: "team", Namespace: "test", UID: "original"}, Spec: api.ModelConnectionSpec{Provider: "custom", Protocol: "openai-chat", Endpoint: "https://models.example/v1/chat/completions", CredentialProfile: "team-key", Models: []string{"chosen"}}}
	store := fake.NewClientBuilder().WithScheme(scheme).WithObjects(connection).Build()
	ctx := context.Background()
	resolved, err := Resolve(ctx, store, "test", api.ModelSpec{ConnectionRef: "team", Model: "chosen"})
	if err != nil || resolved.Provider != "custom" || resolved.BaseURL != connection.Spec.Endpoint || resolved.ConnectionRevision == "" || resolved.AuthSecretRef != "" {
		t.Fatalf("resolution: %+v %v", resolved, err)
	}
	for _, bad := range []api.ModelSpec{{ConnectionRef: "team", Model: "other"}, {ConnectionRef: "team", Model: "chosen", BaseURL: "https://attacker.example/api"}, {ConnectionRef: "team", Model: "chosen", AuthSecretRef: "key"}} {
		if _, err := Resolve(ctx, store, "test", bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	connection.Spec.Endpoint = "https://changed.example/v1/chat/completions"
	if err := store.Update(ctx, connection); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(ctx, store, "test", resolved); err == nil {
		t.Fatal("accepted changed connection")
	}
	if _, err := Resolve(ctx, store, "other", api.ModelSpec{ConnectionRef: "team", Model: "chosen"}); err == nil {
		t.Fatal("cross namespace resolution")
	}
}

func TestUnauthenticatedLlamaServerConnection(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.AddToScheme(scheme)
	connection := &api.ModelConnection{ObjectMeta: metav1.ObjectMeta{Name: "framework", Namespace: "test"}, Spec: api.ModelConnectionSpec{Provider: "llama-server", Protocol: "openai-chat", Endpoint: "http://framework:8080/v1/chat/completions", Models: []string{"qwen"}}}
	store := fake.NewClientBuilder().WithScheme(scheme).WithObjects(connection).Build()
	model, revision, err := ResolveHarness(context.Background(), store, "test", "framework", "qwen")
	if err != nil || model.BaseURL != "http://framework:8080/v1" || model.AuthSecretRef != "" || revision == "" {
		t.Fatalf("resolution: %+v %v", model, err)
	}
	if _, err := Resolve(context.Background(), store, "test", api.ModelSpec{ConnectionRef: "framework", Model: "qwen"}); err == nil {
		t.Fatal("native host accepted unauthenticated HTTP route")
	}
	connection.Spec.Disabled = true
	_ = store.Update(context.Background(), connection)
	if _, _, err := ResolveHarness(context.Background(), store, "test", "framework", "qwen"); err == nil {
		t.Fatal("disabled connection accepted")
	}
}
