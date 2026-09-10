package apiserver

import (
	"encoding/json"
	"net/http"

	api "github.com/sympozium-ai/sympozium/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	if err := req.Spec.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	connection := &api.ModelConnection{ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: ns}, Spec: req.Spec}
	if err := s.client.Create(r.Context(), connection); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(connection)
}
