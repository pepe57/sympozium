package apiserver

import (
	"encoding/json"
	"net/http"

	api "github.com/sympozium-ai/sympozium/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Server) listModelConnections(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "default"
	}
	var list api.ModelConnectionList
	if err := s.client.List(r.Context(), &list, client.InNamespace(ns)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list.Items == nil {
		list.Items = []api.ModelConnection{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list.Items)
}

func (s *Server) createModelConnection(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = "default"
	}
	var req struct {
		Name string                  `json:"name"`
		Spec api.ModelConnectionSpec `json:"spec"`
		// APIKey is optional. When set, the server writes a Kubernetes Secret
		// holding the credential and points the connection at it, so the Auth
		// step behaves exactly like an inline run credential.
		APIKey string `json:"apiKey,omitempty"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	d.DisallowUnknownFields()
	if err := d.Decode(&req); err != nil {
		http.Error(w, "invalid model connection JSON", http.StatusBadRequest)
		return
	}
	if len(validation.IsDNS1123Subdomain(req.Name)) != 0 {
		http.Error(w, "valid connection name required", http.StatusBadRequest)
		return
	}
	if req.APIKey != "" {
		if req.Spec.CredentialProfile != "" {
			http.Error(w, "a Kubernetes Secret and a host credential profile are mutually exclusive", http.StatusBadRequest)
			return
		}
		if req.Spec.SecretRef != "" {
			http.Error(w, "provide an API key or a Secret reference, not both", http.StatusBadRequest)
			return
		}
	}
	if err := req.Spec.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.APIKey != "" {
		secretName := defaultProviderSecretName(req.Name, req.Spec.Provider)
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: ns,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by":  "sympozium",
					"sympozium.ai/model-connection": req.Name,
				},
			},
			StringData: map[string]string{connectionSecretKey(req.Spec.Protocol): req.APIKey},
		}
		if err := createOrUpdateSecret(r.Context(), s.client, secret); err != nil {
			http.Error(w, "failed to create credentials secret: "+err.Error(), http.StatusInternalServerError)
			return
		}
		req.Spec.SecretRef = secretName
	}

	connection := &api.ModelConnection{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: ns}, Spec: req.Spec}
	created := true
	if err := s.client.Create(r.Context(), connection); err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		// Re-saving a connection (for example when the wizard is retried) is an
		// update, not an error, so the same provider/auth/model selection wins.
		created = false
		var existing api.ModelConnection
		if err := s.client.Get(r.Context(), types.NamespacedName{Namespace: ns, Name: req.Name}, &existing); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		existing.Spec = req.Spec
		if err := s.client.Update(r.Context(), &existing); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		connection = &existing
	}
	w.Header().Set("Content-Type", "application/json")
	if created {
		w.WriteHeader(http.StatusCreated)
	}
	json.NewEncoder(w).Encode(connection)
}

// connectionSecretKey returns the Secret key the harness adapters read. The
// maintained OpenAI-compatible adapter always reads OPENAI_API_KEY, even when
// the upstream provider label differs.
func connectionSecretKey(protocol string) string {
	if protocol == "anthropic-messages" {
		return "ANTHROPIC_API_KEY"
	}
	return "OPENAI_API_KEY"
}
