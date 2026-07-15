// Package pricing computes dollar cost estimates for AgentRuns from token
// usage and a per-model price table. All amounts are integer micro-USD
// (1e-6 USD): Kubernetes API conventions forbid floats in CRD types, and
// integer arithmetic keeps estimates exact and comparable.
//
// Estimates are display-only. Token usage originates from the agent pod's own
// result marker, which an adversarial agent can distort, so nothing in this
// package may ever feed admission or budget enforcement.
package pricing

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"

	sympoziumv1alpha1 "github.com/sympozium-ai/sympozium/api/v1alpha1"
	"github.com/sympozium-ai/sympozium/internal/llmprovider"
)

// SourceDefaultTable marks estimates computed from the chart-shipped price
// table; it is the only source ever persisted into AgentRun.status.
const SourceDefaultTable = "defaultTable"

// SourceSimulated marks read-time estimates computed by the apiserver from
// user-defined simulated prices. Never persisted.
const SourceSimulated = "simulated"

// MaxRatePerMTokMicro bounds price-table rates ($1000/MTok). Together with
// the token clamp applied at result-marker parse time (1e10) this makes the
// split-formula arithmetic in CostMicro provably overflow-free.
const MaxRatePerMTokMicro = 1_000_000_000

// Entry prices one provider/model-prefix pair.
type Entry struct {
	// Provider is matched exactly, case-insensitively.
	Provider string `json:"provider"`
	// Match is a literal prefix of the model identifier; the longest
	// matching prefix within a provider wins.
	Match string `json:"match"`
	// InputPerMTokMicro is micro-USD per one million input tokens.
	InputPerMTokMicro int64 `json:"inputPerMTokMicro"`
	// OutputPerMTokMicro is micro-USD per one million output tokens.
	OutputPerMTokMicro int64 `json:"outputPerMTokMicro"`
}

// Table is a parsed price table (schema version 1).
type Table struct {
	Version  int     `json:"version"`
	Currency string  `json:"currency,omitempty"`
	Entries  []Entry `json:"entries"`
}

// ParseTable parses pricing.yaml. Unknown fields are tolerated so newer
// tables load on older controllers; an unknown schema version is an error and
// callers must treat the table as absent (fail open), never as $0.
func ParseTable(data []byte) (*Table, error) {
	var t Table
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parsing price table: %w", err)
	}
	if t.Version != 1 {
		return nil, fmt.Errorf("unsupported price table version %d (want 1)", t.Version)
	}
	if t.Currency != "" && t.Currency != "USD" {
		return nil, fmt.Errorf("unsupported currency %q (want USD)", t.Currency)
	}
	return &t, nil
}

// Lookup returns the table entry for provider/model: exact provider match
// (case-insensitive), then the longest Match that is a literal prefix of the
// model id. Later entries win ties so appended overrides beat shipped
// defaults. Returns nil when nothing matches.
func (t *Table) Lookup(provider, model string) *Entry {
	if t == nil {
		return nil
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	var best *Entry
	for i := range t.Entries {
		e := &t.Entries[i]
		if strings.ToLower(e.Provider) != p {
			continue
		}
		if !strings.HasPrefix(model, e.Match) {
			continue
		}
		if best == nil || len(e.Match) >= len(best.Match) {
			best = e
		}
	}
	return best
}

// Exempt reports whether a run's model must never receive a persisted cost
// estimate: cluster-local Model CR runs and local/self-hosted providers.
// It must be evaluated on the spec as stored in etcd — modelRef resolution
// sets spec.model.provider in-memory only during reconcile, so keying on a
// resolved provider string would misprice local inference.
func Exempt(m sympoziumv1alpha1.ModelSpec) bool {
	return m.ModelRef != "" || llmprovider.IsLocal(m.Provider)
}

// CostMicro applies a micro-USD-per-MTok rate to a token count, rounding half
// up. The split formula is mandatory: with tokens ≤ 1e10 and rate ≤ 1e9 a
// naive tokens*rate reaches 1e19 and overflows int64, while here the largest
// intermediate is rate*(tokens%1e6) < 1e15.
func CostMicro(tokens, ratePerMTokMicro int64) int64 {
	if tokens <= 0 || ratePerMTokMicro <= 0 {
		return 0
	}
	q, r := tokens/1_000_000, tokens%1_000_000
	return ratePerMTokMicro*q + (ratePerMTokMicro*r+500_000)/1_000_000
}

// Estimate computes a cost estimate for the given usage, or nil when the
// table has no entry for provider/model. Absence — never zero — is the
// contract for unpriced runs. Callers stamp Source and EstimatedAt.
func Estimate(t *Table, provider, model string, usage *sympoziumv1alpha1.TokenUsage) *sympoziumv1alpha1.CostEstimate {
	if t == nil || usage == nil {
		return nil
	}
	e := t.Lookup(provider, model)
	if e == nil {
		return nil
	}
	in := CostMicro(int64(usage.InputTokens), e.InputPerMTokMicro)
	out := CostMicro(int64(usage.OutputTokens), e.OutputPerMTokMicro)
	return &sympoziumv1alpha1.CostEstimate{
		AmountMicro:       in + out,
		InputAmountMicro:  in,
		OutputAmountMicro: out,
		Currency:          "USD",
		PriceKey:          fmt.Sprintf("%s/%s", strings.ToLower(e.Provider), e.Match),
	}
}
