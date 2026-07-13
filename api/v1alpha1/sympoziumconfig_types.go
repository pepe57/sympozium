package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SympoziumConfigSpec defines the desired platform-wide configuration.
type SympoziumConfigSpec struct {
	// Gateway configures the shared Envoy Gateway infrastructure.
	// +optional
	Gateway *GatewaySpec `json:"gateway,omitempty"`

	// Canary configures the built-in system health canary.
	// When enabled, a canary Ensemble is created that periodically
	// validates end-to-end platform health.
	// +optional
	Canary *CanarySpec `json:"canary,omitempty"`

	// Pricing configures simulated model pricing. Simulated prices are a
	// display-only overlay applied by the apiserver at read time; they are
	// never persisted into AgentRun status, never exported as metrics, and
	// never feed budget enforcement.
	// +optional
	Pricing *PricingSpec `json:"pricing,omitempty"`
}

// PricingSpec configures user-defined simulated model pricing.
type PricingSpec struct {
	// SimulatedEnabled toggles the read-time simulated-cost overlay.
	// +optional
	SimulatedEnabled bool `json:"simulatedEnabled,omitempty"`

	// SimulatedPrices are user-defined rates, including for local/self-hosted
	// providers that are exempt from real cost estimation (e.g. internal
	// chargeback rates).
	// +optional
	// +kubebuilder:validation:MaxItems=500
	SimulatedPrices []SimulatedPrice `json:"simulatedPrices,omitempty"`

	// UpdatedAt records the last modification of this block.
	// +optional
	UpdatedAt *metav1.Time `json:"updatedAt,omitempty"`

	// UpdatedBy records the caller identity when apiserver auth is enabled.
	// +optional
	UpdatedBy string `json:"updatedBy,omitempty"`
}

// SimulatedPrice prices one provider/model-prefix pair in micro-USD per one
// million tokens.
type SimulatedPrice struct {
	// Provider is the model provider id; may name a local provider.
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,127}$`
	Provider string `json:"provider"`

	// Match is a literal prefix of the model identifier.
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9][a-zA-Z0-9._:/ -]{0,127}$`
	Match string `json:"match"`

	// InputPerMTokMicro is micro-USD per one million input tokens.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000000000
	InputPerMTokMicro int64 `json:"inputPerMTokMicro"`

	// OutputPerMTokMicro is micro-USD per one million output tokens.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000000000
	OutputPerMTokMicro int64 `json:"outputPerMTokMicro"`
}

// CanarySpec configures the built-in system health canary.
type CanarySpec struct {
	// Enabled is the master switch for the system canary.
	// When true, a canary Ensemble is created/enabled.
	// When false, the canary Ensemble is disabled (but not deleted).
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// Interval is the health check interval (e.g. "15m", "30m", "1h").
	// +kubebuilder:default="30m"
	// +optional
	Interval string `json:"interval,omitempty"`

	// Model is the LLM model to use (e.g. "gpt-4o", "claude-sonnet-4-20250514").
	// +optional
	Model string `json:"model,omitempty"`

	// Provider is the LLM provider (e.g. "openai", "anthropic", "ollama").
	// +optional
	Provider string `json:"provider,omitempty"`

	// BaseURL overrides the provider API endpoint (for local models or proxies).
	// +optional
	BaseURL string `json:"baseURL,omitempty"`

	// AuthSecretRef is the name of a Secret containing provider API keys.
	// +optional
	AuthSecretRef string `json:"authSecretRef,omitempty"`
}

// GatewaySpec defines the desired state of the shared Gateway.
type GatewaySpec struct {
	// Enabled is the master switch for the Gateway.
	// When false, Gateway and GatewayClass resources are removed.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// GatewayClassName is the name of the GatewayClass to create.
	// +kubebuilder:default="sympozium"
	// +optional
	GatewayClassName string `json:"gatewayClassName,omitempty"`

	// Name is the Gateway resource name.
	// +kubebuilder:default="sympozium-gateway"
	// +optional
	Name string `json:"name,omitempty"`

	// BaseDomain is the wildcard base domain for instance hostnames.
	// Instances get <name>.<baseDomain> as their hostname.
	// +optional
	BaseDomain string `json:"baseDomain,omitempty"`

	// TLS configures HTTPS on the Gateway.
	// +optional
	TLS *GatewayTLSSpec `json:"tls,omitempty"`
}

// GatewayTLSSpec configures TLS for the Gateway.
type GatewayTLSSpec struct {
	// Enabled turns on the HTTPS listener.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// CertManagerClusterIssuer is the cert-manager ClusterIssuer name.
	// When set, the Gateway is annotated for automatic certificate provisioning.
	// +optional
	CertManagerClusterIssuer string `json:"certManagerClusterIssuer,omitempty"`

	// SecretName is the TLS certificate Secret name.
	// +kubebuilder:default="sympozium-wildcard-cert"
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

// SympoziumConfigStatus defines the observed state of SympoziumConfig.
type SympoziumConfigStatus struct {
	// Phase is the current phase: Disabled, Pending, Ready, or Error.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Gateway reports the observed state of the Gateway.
	// +optional
	Gateway *GatewayStatusInfo `json:"gateway,omitempty"`

	// Canary reports the observed state of the system canary.
	// +optional
	Canary *CanaryStatusInfo `json:"canary,omitempty"`

	// Conditions represent the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// CanaryStatusInfo reports the observed canary state.
type CanaryStatusInfo struct {
	// EnsembleCreated indicates the canary Ensemble CR exists.
	EnsembleCreated bool `json:"ensembleCreated"`

	// LastRunPhase is the phase of the most recent canary run.
	// +optional
	LastRunPhase string `json:"lastRunPhase,omitempty"`

	// LastRunTime is the completion time of the most recent canary run.
	// +optional
	LastRunTime string `json:"lastRunTime,omitempty"`

	// HealthStatus is the overall health from the last report: healthy, degraded, unhealthy, unknown.
	// +optional
	HealthStatus string `json:"healthStatus,omitempty"`
}

// GatewayStatusInfo reports the observed Gateway state.
type GatewayStatusInfo struct {
	// Ready indicates whether the Gateway is accepting traffic.
	Ready bool `json:"ready"`

	// Address is the external IP or hostname of the Gateway.
	// +optional
	Address string `json:"address,omitempty"`

	// ListenerCount is the number of active listeners.
	// +optional
	ListenerCount int `json:"listenerCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Gateway",type="boolean",JSONPath=".spec.gateway.enabled"
// +kubebuilder:printcolumn:name="Domain",type="string",JSONPath=".spec.gateway.baseDomain"
// +kubebuilder:printcolumn:name="Canary",type="boolean",JSONPath=".spec.canary.enabled",priority=1
// +kubebuilder:printcolumn:name="Health",type="string",JSONPath=".status.canary.healthStatus",priority=1
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SympoziumConfig is the Schema for the sympoziumconfigs API.
// It holds platform-wide settings such as gateway configuration.
type SympoziumConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SympoziumConfigSpec   `json:"spec,omitempty"`
	Status SympoziumConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SympoziumConfigList contains a list of SympoziumConfig.
type SympoziumConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SympoziumConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SympoziumConfig{}, &SympoziumConfigList{})
}
