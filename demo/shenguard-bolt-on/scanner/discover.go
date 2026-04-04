package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

// K8sResource represents a parsed Kubernetes resource from YAML.
type K8sResource struct {
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
	FilePath   string
	Line       int
	RawYAML    map[string]interface{}
}

// DiscoveredFiles groups files by their K8s resource type.
type DiscoveredFiles struct {
	ArgoApps       []K8sResource
	Manifests      []K8sResource // with sync-wave annotations
	XRDs           []K8sResource
	Compositions   []K8sResource
	Claims         []K8sResource
	ProviderConfigs []K8sResource
	Rollouts       []K8sResource
	Workflows      []K8sResource
}

// Discover finds all relevant K8s YAML files in the given root directory.
// If a Config is provided, uses explicit paths. Otherwise, auto-detects
// by reading every YAML file and classifying by kind.
func Discover(root string, cfg *Config) (*DiscoveredFiles, error) {
	if cfg != nil && hasExplicitPaths(cfg) {
		return discoverFromConfig(root, cfg)
	}
	return autoDiscover(root)
}

// autoDiscover walks the directory tree, reads every .yaml/.yml file,
// and classifies by kind field.
func autoDiscover(root string) (*DiscoveredFiles, error) {
	result := &DiscoveredFiles{}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable files
		}
		if info.IsDir() {
			// skip common non-relevant dirs
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		resources, err := parseYAMLFile(path)
		if err != nil {
			return nil // skip unparseable files
		}

		for _, r := range resources {
			classify(result, r)
		}
		return nil
	})

	return result, err
}

// classify places a K8sResource into the correct bucket based on Kind.
func classify(files *DiscoveredFiles, r K8sResource) {
	switch r.Kind {
	case "Application", "ApplicationSet":
		files.ArgoApps = append(files.ArgoApps, r)
	case "CompositeResourceDefinition":
		files.XRDs = append(files.XRDs, r)
	case "Composition":
		files.Compositions = append(files.Compositions, r)
	case "ProviderConfig":
		files.ProviderConfigs = append(files.ProviderConfigs, r)
	case "Rollout":
		files.Rollouts = append(files.Rollouts, r)
	case "Workflow", "WorkflowTemplate":
		files.Workflows = append(files.Workflows, r)
	default:
		// Check if it has a sync-wave annotation → it's a manifest
		if hasSyncWaveAnnotation(r) {
			files.Manifests = append(files.Manifests, r)
		}
		// Check if it looks like a Crossplane Claim (has compositionRef or compositionSelector)
		if isLikelyClaim(r) {
			files.Claims = append(files.Claims, r)
		}
	}
}

func hasExplicitPaths(cfg *Config) bool {
	return len(cfg.Sources.Argo.Applications) > 0 ||
		len(cfg.Sources.Crossplane.XRDs) > 0 ||
		len(cfg.Sources.Rollouts) > 0 ||
		len(cfg.Sources.Workflows) > 0
}

func discoverFromConfig(root string, cfg *Config) (*DiscoveredFiles, error) {
	result := &DiscoveredFiles{}
	// Expand globs from config and classify
	// Implementation: for each glob pattern, filepath.Glob, parse, classify
	_ = root
	_ = cfg
	return result, nil // scaffold — glob expansion goes here
}

func hasSyncWaveAnnotation(r K8sResource) bool {
	meta, ok := r.RawYAML["metadata"].(map[string]interface{})
	if !ok {
		return false
	}
	annotations, ok := meta["annotations"].(map[string]interface{})
	if !ok {
		return false
	}
	_, ok = annotations["argocd.argoproj.io/sync-wave"]
	return ok
}

func isLikelyClaim(r K8sResource) bool {
	spec, ok := r.RawYAML["spec"].(map[string]interface{})
	if !ok {
		return false
	}
	_, hasRef := spec["compositionRef"]
	_, hasSel := spec["compositionSelector"]
	return hasRef || hasSel
}
