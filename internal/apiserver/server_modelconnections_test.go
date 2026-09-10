package apiserver

import (
	"context"
	api "github.com/sympozium-ai/sympozium/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateConnectionAndAgentWithoutProviderOrSecret(t *testing.T) {
	server, _ := newInstanceTestServer(t)
	handler := server.buildMux(nil, nil)
	for _, request := range []struct{ path, body string }{
		{"/api/v1/model-connections", `{"name":"framework","spec":{"provider":"llama-server","protocol":"openai-chat","endpoint":"http://framework:8080/v1/chat/completions","models":["qwen"]}}`},
		{"/api/v1/agents", `{"name":"hermes","model":"qwen","execution":{"backend":"job","modelConnectionRef":"framework"}}`},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, request.path, strings.NewReader(request.body)))
		if response.Code >= 300 {
			t.Fatalf("%s: %d %s", request.path, response.Code, response.Body.String())
		}
	}
	var agent api.Agent
	if err := server.client.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "hermes"}, &agent); err != nil {
		t.Fatal(err)
	}
	if agent.Spec.Execution.ModelConnectionRef != "framework" || agent.Spec.Agents.Default.BaseURL != "http://framework:8080/v1" || len(agent.Spec.AuthRefs) != 0 {
		t.Fatalf("wrong connection: %+v", agent.Spec)
	}
	var secrets corev1.SecretList
	_ = server.client.List(context.Background(), &secrets)
	if len(secrets.Items) != 0 {
		t.Fatal("unauthenticated model created a Secret")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"bad","model":"qwen","apiKey":"must-not-be-written","execution":{"backend":"job","modelConnectionRef":"framework"}}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatal("accepted connection and inline key")
	}
}
