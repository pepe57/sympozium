package apiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	sympoziumv1alpha1 "github.com/sympozium-ai/sympozium/api/v1alpha1"
	"github.com/sympozium-ai/sympozium/internal/llmprovider"
	"github.com/sympozium-ai/sympozium/internal/pricing"
)

// simulatedProviderModelRef is the reserved provider key that lets simulated
// prices target Model-CR-backed runs; it is matched against the Model name.
const simulatedProviderModelRef = "modelref"

// pricingWritesDisabledMsg is returned (403) for simulated-price writes when
// the apiserver runs without a bearer token. Prices are cluster-wide state;
// on an unauthenticated apiserver anyone on the network could set them.
const pricingWritesDisabledMsg = "pricing edits require apiserver authentication (apiserver.authToken)"

var (
	simulatedProviderRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,127}$`)
	simulatedMatchRe    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/ -]{0,127}$`)
)

// PricingResponse is the response for GET /api/v1/pricing.
type PricingResponse struct {
	Currency       string                         `json:"currency"`
	DefaultTable   []pricing.Entry                `json:"defaultTable"`
	Simulated      *sympoziumv1alpha1.PricingSpec `json:"simulated"`
	LocalProviders []string                       `json:"localProviders"`
	Writable       bool                           `json:"writable"`
}

// loadDefaultTable reads the cluster price-table ConfigMap. Fail-open: any
// error yields an empty table (the UI shows "no prices configured").
func (s *Server) loadDefaultTable(ctx context.Context) []pricing.Entry {
	name := os.Getenv("SYMPOZIUM_PRICING_CONFIGMAP")
	if name == "" || s.kube == nil {
		return nil
	}
	ns := os.Getenv("SYMPOZIUM_PRICING_NAMESPACE")
	if ns == "" {
		ns = "sympozium-system"
	}
	cm, err := s.kube.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	t, err := pricing.ParseTable([]byte(cm.Data[pricing.DataKey]))
	if err != nil {
		s.log.Info("Ignoring malformed price table", "configMap", name, "error", err.Error())
		return nil
	}
	return t.Entries
}

func (s *Server) getPricing(w http.ResponseWriter, r *http.Request) {
	resp := PricingResponse{
		Currency:       "USD",
		DefaultTable:   s.loadDefaultTable(r.Context()),
		LocalProviders: llmprovider.LocalProviders(),
		Writable:       s.authEnabled,
	}

	var config sympoziumv1alpha1.SympoziumConfig
	err := s.client.Get(r.Context(), types.NamespacedName{Name: "default", Namespace: configNamespace(r)}, &config)
	if err == nil {
		resp.Simulated = config.Spec.Pricing
	}
	writeJSON(w, resp)
}

// SimulatedPricesRequest is the request body for PUT /api/v1/pricing/simulated.
type SimulatedPricesRequest struct {
	SimulatedEnabled bool                               `json:"simulatedEnabled"`
	SimulatedPrices  []sympoziumv1alpha1.SimulatedPrice `json:"simulatedPrices"`
}

// validateSimulatedPrices mirrors the CRD kubebuilder markers so callers get
// field-level 400s instead of opaque admission errors. These strings render
// in the UI, so the patterns exclude control characters.
func validateSimulatedPrices(prices []sympoziumv1alpha1.SimulatedPrice) error {
	if len(prices) > 500 {
		return fmt.Errorf("at most 500 simulated prices allowed, got %d", len(prices))
	}
	for i, p := range prices {
		if !simulatedProviderRe.MatchString(p.Provider) {
			return fmt.Errorf("simulatedPrices[%d].provider %q is invalid", i, p.Provider)
		}
		if !simulatedMatchRe.MatchString(p.Match) {
			return fmt.Errorf("simulatedPrices[%d].match %q is invalid", i, p.Match)
		}
		if p.InputPerMTokMicro < 1 || p.InputPerMTokMicro > pricing.MaxRatePerMTokMicro {
			return fmt.Errorf("simulatedPrices[%d].inputPerMTokMicro must be 1..%d micro-USD per 1M tokens", i, int64(pricing.MaxRatePerMTokMicro))
		}
		if p.OutputPerMTokMicro < 1 || p.OutputPerMTokMicro > pricing.MaxRatePerMTokMicro {
			return fmt.Errorf("simulatedPrices[%d].outputPerMTokMicro must be 1..%d micro-USD per 1M tokens", i, int64(pricing.MaxRatePerMTokMicro))
		}
	}
	return nil
}

func (s *Server) putSimulatedPrices(w http.ResponseWriter, r *http.Request) {
	if !s.authEnabled {
		http.Error(w, pricingWritesDisabledMsg, http.StatusForbidden)
		return
	}

	var req SimulatedPricesRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateSimulatedPrices(req.SimulatedPrices); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ns := configNamespace(r)
	var config sympoziumv1alpha1.SympoziumConfig
	created := false
	if err := s.client.Get(r.Context(), types.NamespacedName{Name: "default", Namespace: ns}, &config); err != nil {
		if !k8serrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		config = sympoziumv1alpha1.SympoziumConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: ns},
		}
		created = true
	}

	now := metav1.Now()
	config.Spec.Pricing = &sympoziumv1alpha1.PricingSpec{
		SimulatedEnabled: req.SimulatedEnabled,
		SimulatedPrices:  req.SimulatedPrices,
		UpdatedAt:        &now,
	}

	var err error
	if created {
		err = s.client.Create(r.Context(), &config)
	} else {
		err = s.client.Update(r.Context(), &config)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, config.Spec.Pricing)
}

func (s *Server) deleteSimulatedPrices(w http.ResponseWriter, r *http.Request) {
	if !s.authEnabled {
		http.Error(w, pricingWritesDisabledMsg, http.StatusForbidden)
		return
	}

	ns := configNamespace(r)
	var config sympoziumv1alpha1.SympoziumConfig
	if err := s.client.Get(r.Context(), types.NamespacedName{Name: "default", Namespace: ns}, &config); err != nil {
		if k8serrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if config.Spec.Pricing != nil {
		config.Spec.Pricing = nil
		if err := s.client.Update(r.Context(), &config); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// runWithCostOverlay decorates a raw AgentRun with the read-time simulated
// cost estimate. The embedded CR marshals its fields at the top level, so
// existing clients are unaffected.
type runWithCostOverlay struct {
	sympoziumv1alpha1.AgentRun
	SimulatedCostEstimate *sympoziumv1alpha1.CostEstimate `json:"simulatedCostEstimate,omitempty"`
}

// simulatedTable loads the simulated price table, or nil when the overlay is
// disabled. Simulated estimates are computed at read time only and are never
// persisted anywhere — that structural split is what keeps them from ever
// masquerading as real cost data.
func (s *Server) simulatedTable(ctx context.Context) *pricing.Table {
	var config sympoziumv1alpha1.SympoziumConfig
	if err := s.client.Get(ctx, types.NamespacedName{Name: "default", Namespace: "sympozium-system"}, &config); err != nil {
		return nil
	}
	p := config.Spec.Pricing
	if p == nil || !p.SimulatedEnabled || len(p.SimulatedPrices) == 0 {
		return nil
	}
	t := &pricing.Table{Version: 1, Currency: "USD"}
	for _, sp := range p.SimulatedPrices {
		t.Entries = append(t.Entries, pricing.Entry{
			Provider:           sp.Provider,
			Match:              sp.Match,
			InputPerMTokMicro:  sp.InputPerMTokMicro,
			OutputPerMTokMicro: sp.OutputPerMTokMicro,
		})
	}
	return t
}

// overlaySimulatedCost computes the simulated estimate for one run against a
// pre-loaded table. Model-CR runs match the reserved "modelref" provider by
// Model name; all other runs match on their spec provider/model.
func overlaySimulatedCost(t *pricing.Table, run *sympoziumv1alpha1.AgentRun) *sympoziumv1alpha1.CostEstimate {
	if t == nil || run.Status.TokenUsage == nil {
		return nil
	}
	provider, model := run.Spec.Model.Provider, run.Spec.Model.Model
	if run.Spec.Model.ModelRef != "" {
		provider, model = simulatedProviderModelRef, run.Spec.Model.ModelRef
	}
	est := pricing.Estimate(t, provider, model, run.Status.TokenUsage)
	if est == nil {
		return nil
	}
	est.Source = pricing.SourceSimulated
	return est
}
