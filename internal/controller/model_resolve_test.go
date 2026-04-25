package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	sympoziumv1alpha1 "github.com/sympozium-ai/sympozium/api/v1alpha1"
)

func newTestModel(ns, name string, phase sympoziumv1alpha1.ModelPhase) *sympoziumv1alpha1.Model {
	return &sympoziumv1alpha1.Model{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Status:     sympoziumv1alpha1.ModelStatus{Phase: phase},
	}
}

func TestResolveModelRef(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = sympoziumv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name           string
		modelRef       string
		localNamespace string
		models         []*sympoziumv1alpha1.Model
		wantNamespace  string
		wantErr        bool
	}{
		{
			name:           "found in local namespace",
			modelRef:       "my-model",
			localNamespace: "default",
			models:         []*sympoziumv1alpha1.Model{newTestModel("default", "my-model", sympoziumv1alpha1.ModelPhaseReady)},
			wantNamespace:  "default",
		},
		{
			name:           "fallback to system namespace",
			modelRef:       "my-model",
			localNamespace: "default",
			models:         []*sympoziumv1alpha1.Model{newTestModel(SystemNamespace, "my-model", sympoziumv1alpha1.ModelPhaseReady)},
			wantNamespace:  SystemNamespace,
		},
		{
			name:           "local takes precedence over system",
			modelRef:       "my-model",
			localNamespace: "default",
			models: []*sympoziumv1alpha1.Model{
				newTestModel("default", "my-model", sympoziumv1alpha1.ModelPhaseReady),
				newTestModel(SystemNamespace, "my-model", sympoziumv1alpha1.ModelPhaseReady),
			},
			wantNamespace: "default",
		},
		{
			name:           "explicit namespace/name",
			modelRef:       "other-ns/my-model",
			localNamespace: "default",
			models:         []*sympoziumv1alpha1.Model{newTestModel("other-ns", "my-model", sympoziumv1alpha1.ModelPhaseReady)},
			wantNamespace:  "other-ns",
		},
		{
			name:           "explicit namespace/name not found",
			modelRef:       "other-ns/missing",
			localNamespace: "default",
			models:         []*sympoziumv1alpha1.Model{},
			wantErr:        true,
		},
		{
			name:           "not found anywhere",
			modelRef:       "missing-model",
			localNamespace: "default",
			models:         []*sympoziumv1alpha1.Model{},
			wantErr:        true,
		},
		{
			name:           "local is system namespace no double lookup",
			modelRef:       "my-model",
			localNamespace: SystemNamespace,
			models:         []*sympoziumv1alpha1.Model{newTestModel(SystemNamespace, "my-model", sympoziumv1alpha1.ModelPhaseReady)},
			wantNamespace:  SystemNamespace,
		},
		{
			name:           "local is system namespace not found",
			modelRef:       "missing",
			localNamespace: SystemNamespace,
			models:         []*sympoziumv1alpha1.Model{},
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := make([]runtime.Object, len(tt.models))
			for i, m := range tt.models {
				objs[i] = m
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

			model, err := ResolveModelRef(context.Background(), c, tt.modelRef, tt.localNamespace)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if model.Namespace != tt.wantNamespace {
				t.Errorf("got namespace %q, want %q", model.Namespace, tt.wantNamespace)
			}
		})
	}
}
