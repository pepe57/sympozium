// Package llmprovider is the canonical taxonomy of LLM provider identifiers.
// The runner, controller, and apiserver all key behavior off provider strings
// (timeouts, retries, cost-estimation exemption), so the category lists live
// here to prevent drift between binaries.
package llmprovider

import "strings"

// localProviders are providers that run inference locally / self-hosted
// (single-GPU, request queuing, no metered billing).
var localProviders = []string{"ollama", "lm-studio", "llama-server", "unsloth", "vllm", "llamacpp", "local"}

// LocalProviders returns the canonical list of self-hosted provider ids.
func LocalProviders() []string {
	out := make([]string, len(localProviders))
	copy(out, localProviders)
	return out
}

// IsLocal reports whether the provider runs inference locally/self-hosted.
// Matching is case-insensitive; unknown providers are treated as remote so
// that OpenAI-compatible gateways with arbitrary provider strings stay
// priceable.
func IsLocal(provider string) bool {
	p := strings.ToLower(strings.TrimSpace(provider))
	for _, l := range localProviders {
		if p == l {
			return true
		}
	}
	return false
}
