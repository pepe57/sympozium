package controller

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sympoziumv1alpha1 "github.com/sympozium-ai/sympozium/api/v1alpha1"
)

// SystemNamespace is the default namespace for platform-managed resources.
const SystemNamespace = "sympozium-system"

// ResolveModelRef resolves a modelRef string to a Model CR.
//
// Resolution order:
//  1. If modelRef contains "/", treat as "namespace/name" (explicit).
//  2. Otherwise, look in localNamespace first.
//  3. Fall back to SystemNamespace if not found locally.
func ResolveModelRef(ctx context.Context, c client.Reader, modelRef, localNamespace string) (*sympoziumv1alpha1.Model, error) {
	// Explicit namespace/name syntax.
	if parts := strings.SplitN(modelRef, "/", 2); len(parts) == 2 {
		ns, name := parts[0], parts[1]
		var model sympoziumv1alpha1.Model
		if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &model); err != nil {
			return nil, fmt.Errorf("model %q not found in namespace %q: %w", name, ns, err)
		}
		return &model, nil
	}

	// Try local namespace first.
	var model sympoziumv1alpha1.Model
	err := c.Get(ctx, client.ObjectKey{Namespace: localNamespace, Name: modelRef}, &model)
	if err == nil {
		return &model, nil
	}
	if !errors.IsNotFound(err) {
		return nil, err
	}

	// Fall back to system namespace (skip if already checked).
	if localNamespace == SystemNamespace {
		return nil, fmt.Errorf("model %q not found in namespace %q", modelRef, localNamespace)
	}

	if err := c.Get(ctx, client.ObjectKey{Namespace: SystemNamespace, Name: modelRef}, &model); err != nil {
		return nil, fmt.Errorf("model %q not found in namespace %q or %q", modelRef, localNamespace, SystemNamespace)
	}
	return &model, nil
}
