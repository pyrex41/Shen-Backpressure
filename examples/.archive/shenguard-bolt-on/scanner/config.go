// Package scanner reads Argo/Crossplane YAML files, extracts structured
// data, and attempts to construct Shen guard type proofs from it.
//
// This is the adapter layer between "YAML files on disk" and "proof objects
// in shenguard/". If a proof constructor rejects, the scanner reports exactly
// which file, line, and field caused the failure.
package scanner

// Config represents shen-k8s.yaml — tells the scanner where to find things.
type Config struct {
	Spec    string        `yaml:"spec"`
	Sources SourcePaths   `yaml:"sources"`
	Known   KnownState    `yaml:"known_mutations"`
	Ingress map[string]IngressCap `yaml:"ingress_capabilities"`
	Live    LiveCluster   `yaml:"live_cluster"`
	Hints   map[string]SchemaHint `yaml:"schema_hints"`
}

type SourcePaths struct {
	Argo       ArgoSources       `yaml:"argo"`
	Crossplane CrossplaneSources `yaml:"crossplane"`
	Rollouts   []string          `yaml:"rollouts"`
	Workflows  []string          `yaml:"workflows"`
}

type ArgoSources struct {
	Applications []string `yaml:"applications"`
	Manifests    []string `yaml:"manifests"`
}

type CrossplaneSources struct {
	XRDs            []string `yaml:"xrds"`
	Compositions    []string `yaml:"compositions"`
	Claims          []string `yaml:"claims"`
	ProviderConfigs []string `yaml:"provider_configs"`
}

type KnownMutation struct {
	Operator string   `yaml:"operator"`
	Fields   []string `yaml:"fields"`
}

type KnownState struct {
	Mutations []KnownMutation `yaml:"mutations"`
}

type IngressCap struct {
	WeightedRouting bool `yaml:"weighted_routing"`
}

type LiveCluster struct {
	Enabled    bool   `yaml:"enabled"`
	Kubeconfig string `yaml:"kubeconfig"`
	Context    string `yaml:"context"`
}

type SchemaHint struct {
	Required []string `yaml:"required"`
	Fields   []string `yaml:"fields"`
}
