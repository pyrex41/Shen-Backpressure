package scanner

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// parseYAMLFile reads a YAML file (potentially multi-document with ---)
// and returns a slice of K8sResource structs with the basic fields extracted.
//
// Uses a lightweight parser that extracts apiVersion, kind, name, namespace
// without a full YAML library dependency. For production use, switch to
// sigs.k8s.io/yaml or gopkg.in/yaml.v3.
func parseYAMLFile(path string) ([]K8sResource, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var resources []K8sResource
	var current *K8sResource
	lineNum := 0

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Document separator
		if strings.TrimSpace(line) == "---" {
			if current != nil && current.Kind != "" {
				resources = append(resources, *current)
			}
			current = &K8sResource{
				FilePath: path,
				Line:     lineNum,
				RawYAML:  make(map[string]interface{}),
			}
			continue
		}

		if current == nil {
			current = &K8sResource{
				FilePath: path,
				Line:     1,
				RawYAML:  make(map[string]interface{}),
			}
		}

		// Extract top-level fields (simplified — real impl uses yaml.v3)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "apiVersion:") {
			current.APIVersion = extractValue(trimmed)
		} else if strings.HasPrefix(trimmed, "kind:") {
			current.Kind = extractValue(trimmed)
		} else if strings.HasPrefix(trimmed, "name:") && !strings.HasPrefix(line, "  ") {
			// top-level name only (not nested)
		}
	}

	if current != nil && current.Kind != "" {
		resources = append(resources, *current)
	}

	return resources, scanner.Err()
}

func extractValue(line string) string {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// ExtractSyncWave extracts the sync-wave annotation value from a resource.
// Returns 0 if not present (default wave).
func ExtractSyncWave(r K8sResource) int {
	meta, ok := r.RawYAML["metadata"].(map[string]interface{})
	if !ok {
		return 0
	}
	annotations, ok := meta["annotations"].(map[string]interface{})
	if !ok {
		return 0
	}
	wave, ok := annotations["argocd.argoproj.io/sync-wave"]
	if !ok {
		return 0
	}
	// Parse string to int
	var v int
	fmt.Sscanf(fmt.Sprint(wave), "%d", &v)
	return v
}

// ExtractPatchPaths extracts all fromFieldPath and toFieldPath values
// from a Crossplane Composition resource.
type PatchPath struct {
	FromFieldPath string
	ToFieldPath   string
	Line          int
}

// ExtractCanaryWeights extracts the setWeight values from an Argo Rollout.
func ExtractCanaryWeights(r K8sResource) []int {
	// Walks spec.strategy.canary.steps[].setWeight
	// Scaffold — real implementation uses yaml.v3 for full traversal
	return nil
}

// ExtractDAGEdges extracts dependency edges from an Argo Workflow DAG.
type DAGEdge struct {
	From string
	To   string
}

func ExtractDAGEdges(r K8sResource) []DAGEdge {
	// Walks spec.templates[].dag.tasks[].dependencies
	// Scaffold — real implementation uses yaml.v3 for full traversal
	return nil
}

// ExtractXRDFields extracts field paths from an XRD's openAPIV3Schema.
func ExtractXRDFields(r K8sResource) (fields []string, required []string) {
	// Walks spec.versions[].schema.openAPIV3Schema.properties recursively
	// Scaffold — real implementation uses yaml.v3 for full traversal
	return nil, nil
}
