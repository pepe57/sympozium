package pricing

import (
	"testing"

	sympoziumv1alpha1 "github.com/sympozium-ai/sympozium/api/v1alpha1"
)

func testTable(t *testing.T) *Table {
	t.Helper()
	tbl, err := ParseTable([]byte(`
version: 1
currency: USD
entries:
  - provider: openai
    match: gpt-4o
    inputPerMTokMicro: 2500000
    outputPerMTokMicro: 10000000
  - provider: openai
    match: gpt-4o-mini
    inputPerMTokMicro: 150000
    outputPerMTokMicro: 600000
  - provider: anthropic
    match: claude-sonnet-4-5
    inputPerMTokMicro: 3000000
    outputPerMTokMicro: 15000000
`))
	if err != nil {
		t.Fatalf("ParseTable: %v", err)
	}
	return tbl
}

func TestLookup_LongestPrefixWins(t *testing.T) {
	tbl := testTable(t)
	e := tbl.Lookup("openai", "gpt-4o-mini-2024-07-18")
	if e == nil || e.Match != "gpt-4o-mini" {
		t.Fatalf("lookup = %+v, want gpt-4o-mini entry", e)
	}
	e = tbl.Lookup("openai", "gpt-4o-2024-11-20")
	if e == nil || e.Match != "gpt-4o" {
		t.Fatalf("lookup = %+v, want gpt-4o entry", e)
	}
}

func TestLookup_ProviderCaseInsensitive_NoCrossProvider(t *testing.T) {
	tbl := testTable(t)
	if e := tbl.Lookup("OpenAI", "gpt-4o"); e == nil {
		t.Fatal("expected case-insensitive provider match")
	}
	if e := tbl.Lookup("anthropic", "gpt-4o"); e != nil {
		t.Fatalf("expected no cross-provider match, got %+v", e)
	}
	if e := tbl.Lookup("openai", "o3-mini"); e != nil {
		t.Fatalf("expected nil for unpriced model, got %+v", e)
	}
}

func TestLookup_LaterEntriesWinTies(t *testing.T) {
	tbl := testTable(t)
	tbl.Entries = append(tbl.Entries, Entry{
		Provider: "openai", Match: "gpt-4o",
		InputPerMTokMicro: 1, OutputPerMTokMicro: 1,
	})
	e := tbl.Lookup("openai", "gpt-4o")
	if e == nil || e.InputPerMTokMicro != 1 {
		t.Fatalf("lookup = %+v, want appended override to win the tie", e)
	}
}

func TestParseTable_WrongVersionAndCurrency(t *testing.T) {
	if _, err := ParseTable([]byte("version: 2\nentries: []")); err == nil {
		t.Fatal("expected error for version 2")
	}
	if _, err := ParseTable([]byte("version: 1\ncurrency: EUR\nentries: []")); err == nil {
		t.Fatal("expected error for non-USD currency")
	}
	if _, err := ParseTable([]byte("version: 1\nfutureField: x\nentries: []")); err != nil {
		t.Fatalf("unknown fields must be tolerated: %v", err)
	}
}

func TestCostMicro_BoundaryNoOverflow(t *testing.T) {
	// tokens=1e10 (parse-time clamp), rate=1e9 (table cap): the exact answer
	// is 1e13 micro-USD. A naive tokens*rate would be 1e19 > MaxInt64.
	got := CostMicro(10_000_000_000, MaxRatePerMTokMicro)
	if got != 10_000_000_000_000 {
		t.Fatalf("CostMicro = %d, want 10000000000000", got)
	}
}

func TestCostMicro_RoundHalfUp(t *testing.T) {
	// 1 token at $2.50/MTok = 2.5 micro-USD → rounds to 3.
	if got := CostMicro(1, 2_500_000); got != 3 {
		t.Fatalf("CostMicro = %d, want 3", got)
	}
	if got := CostMicro(0, 2_500_000); got != 0 {
		t.Fatalf("CostMicro(0) = %d, want 0", got)
	}
	if got := CostMicro(-5, 2_500_000); got != 0 {
		t.Fatalf("CostMicro(-5) = %d, want 0", got)
	}
}

func TestEstimate(t *testing.T) {
	tbl := testTable(t)
	usage := &sympoziumv1alpha1.TokenUsage{InputTokens: 1_000_000, OutputTokens: 500_000}
	est := Estimate(tbl, "openai", "gpt-4o", usage)
	if est == nil {
		t.Fatal("expected estimate")
	}
	// 1M input @ $2.50 + 0.5M output @ $10 = $2.50 + $5.00 = $7.50.
	if est.AmountMicro != 7_500_000 {
		t.Fatalf("amountMicro = %d, want 7500000", est.AmountMicro)
	}
	if est.InputAmountMicro != 2_500_000 || est.OutputAmountMicro != 5_000_000 {
		t.Fatalf("split = %d/%d, want 2500000/5000000", est.InputAmountMicro, est.OutputAmountMicro)
	}
	if est.PriceKey != "openai/gpt-4o" {
		t.Fatalf("priceKey = %q", est.PriceKey)
	}
	if got := Estimate(tbl, "openai", "unpriced-model", usage); got != nil {
		t.Fatalf("expected nil for unpriced model, got %+v", got)
	}
	if got := Estimate(nil, "openai", "gpt-4o", usage); got != nil {
		t.Fatalf("expected nil for nil table, got %+v", got)
	}
}

func TestExempt(t *testing.T) {
	if !Exempt(sympoziumv1alpha1.ModelSpec{Provider: "llama-server", Model: "Qwen3"}) {
		t.Fatal("llama-server must be exempt")
	}
	// modelRef runs keep provider empty in etcd (resolution is in-memory
	// only); exemption must key on modelRef, not the resolved provider.
	if !Exempt(sympoziumv1alpha1.ModelSpec{ModelRef: "my-model"}) {
		t.Fatal("modelRef runs must be exempt")
	}
	if Exempt(sympoziumv1alpha1.ModelSpec{Provider: "openai", Model: "gpt-4o"}) {
		t.Fatal("openai must not be exempt")
	}
	if Exempt(sympoziumv1alpha1.ModelSpec{Provider: "my-custom-gateway"}) {
		t.Fatal("unknown providers are treated as remote (priceable)")
	}
}
