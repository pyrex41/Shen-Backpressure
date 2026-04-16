package scanner

import "fmt"

// ProofResult represents the outcome of attempting to construct a proof.
type ProofResult struct {
	Category string // "argo", "crossplane", "rollouts", "workflows"
	Check    string // e.g. "wave-ordered", "patch-valid", "dag-acyclic"
	Passed   bool
	Message  string
	File     string
	Line     int
	Fix      string // suggested fix (if failed)
}

// ScanReport is the complete output of a scan.
type ScanReport struct {
	SpecFile    string
	TypeCount   int
	Results     []ProofResult
	PassCount   int
	FailCount   int
}

// Validate runs all proof checks against discovered files.
// This is where the scanner calls into the generated shenguard constructors.
//
// The pattern for each check:
//   1. Extract data from YAML (e.g., sync wave values)
//   2. Call the guard constructor (e.g., NewWaveOrdered(depWave, depndtWave))
//   3. If constructor returns error → record failure with file/line/fix
//   4. If constructor succeeds → record pass
func Validate(files *DiscoveredFiles, cfg *Config) *ScanReport {
	report := &ScanReport{}

	// --- ArgoCD checks ---
	report.Results = append(report.Results, validateSyncWaves(files)...)
	report.Results = append(report.Results, validateCRDOrdering(files)...)
	report.Results = append(report.Results, validateDriftCoverage(files, cfg)...)

	// --- Crossplane checks ---
	report.Results = append(report.Results, validatePatchPaths(files)...)
	report.Results = append(report.Results, validateXRDCompatibility(files)...)
	report.Results = append(report.Results, validateResourceCompleteness(files)...)
	report.Results = append(report.Results, validateCredentials(files, cfg)...)

	// --- Argo Rollouts checks ---
	report.Results = append(report.Results, validateRollouts(files, cfg)...)

	// --- Argo Workflows checks ---
	report.Results = append(report.Results, validateWorkflowDAGs(files)...)
	report.Results = append(report.Results, validateWorkflowParams(files)...)

	// Tally
	for _, r := range report.Results {
		if r.Passed {
			report.PassCount++
		} else {
			report.FailCount++
		}
	}

	return report
}

// --- ArgoCD validators ---

func validateSyncWaves(files *DiscoveredFiles) []ProofResult {
	var results []ProofResult

	// Build wave map from all manifests
	type waveEntry struct {
		Resource K8sResource
		Wave     int
	}
	var entries []waveEntry
	for _, m := range files.Manifests {
		entries = append(entries, waveEntry{m, ExtractSyncWave(m)})
	}

	// For every pair where B depends on A, check A.wave < B.wave
	// In practice, dependencies are inferred from:
	//   - CRD → CR relationships (the CRD's kind matches the CR's kind)
	//   - Explicit argocd.argoproj.io/sync-wave ordering

	// Simplified: check all CRDs are in waves <= all CRs
	for _, e := range entries {
		if e.Resource.Kind == "CustomResourceDefinition" {
			for _, other := range entries {
				if other.Resource.Kind != "CustomResourceDefinition" && other.Wave <= e.Wave {
					results = append(results, ProofResult{
						Category: "argo",
						Check:    "wave-ordered",
						Passed:   false,
						Message:  fmt.Sprintf("CRD %s (wave %d) not before %s %s (wave %d)", e.Resource.Name, e.Wave, other.Resource.Kind, other.Resource.Name, other.Wave),
						File:     other.Resource.FilePath,
						Line:     other.Resource.Line,
						Fix:      fmt.Sprintf("Set %s sync-wave to at least %d", other.Resource.Name, e.Wave+1),
					})
				}
			}
		}
	}

	if len(results) == 0 && len(entries) > 0 {
		results = append(results, ProofResult{
			Category: "argo",
			Check:    "wave-ordered",
			Passed:   true,
			Message:  fmt.Sprintf("%d resources across waves, all ordered", len(entries)),
		})
	}

	return results
}

func validateCRDOrdering(files *DiscoveredFiles) []ProofResult {
	// Check that for every CR, its CRD exists and is in an earlier wave
	// Uses: NewCrdBeforeCr(crdWave, crWave, crdKind)
	return nil // scaffold
}

func validateDriftCoverage(files *DiscoveredFiles, cfg *Config) []ProofResult {
	var results []ProofResult
	if cfg == nil {
		return results
	}

	// For each known mutation, check that an ignoreDifferences rule exists
	// in the relevant Argo Application
	for _, app := range files.ArgoApps {
		ignoredFields := extractIgnoreDifferences(app)
		for _, mutation := range cfg.Known.Mutations {
			for _, field := range mutation.Fields {
				covered := false
				for _, ignored := range ignoredFields {
					if ignored == field {
						covered = true
						break
					}
				}
				if !covered {
					results = append(results, ProofResult{
						Category: "argo",
						Check:    "drift-covered",
						Passed:   false,
						Message:  fmt.Sprintf("%s mutates %s but no ignoreDifferences rule covers it", mutation.Operator, field),
						File:     app.FilePath,
						Line:     app.Line,
						Fix:      fmt.Sprintf("Add ignoreDifferences entry for jsonPointers: [/%s]", field),
					})
				}
			}
		}
	}

	return results
}

func extractIgnoreDifferences(app K8sResource) []string {
	// Extract ignoreDifferences.jsonPointers from Argo Application YAML
	// Scaffold — real impl traverses the parsed YAML
	return nil
}

// --- Crossplane validators ---

func validatePatchPaths(files *DiscoveredFiles) []ProofResult {
	// For each Composition:
	//   1. Find the referenced XRD (compositeTypeRef)
	//   2. Extract XRD field paths
	//   3. For each patch, check fromFieldPath exists in XRD
	//   4. For each patch, check toFieldPath exists in MR schema
	// Uses: NewPatchSourceValid, NewPatchTargetValid, NewPatchValid
	return nil // scaffold
}

func validateXRDCompatibility(files *DiscoveredFiles) []ProofResult {
	// Compare XRD versions: no removed fields, no new required fields
	// Uses: NewXrdCompatible
	return nil // scaffold
}

func validateResourceCompleteness(files *DiscoveredFiles) []ProofResult {
	// For each Claim, trace through Composition patches to verify
	// all required MR fields are populated
	// Uses: NewResourceComplete
	return nil // scaffold
}

func validateCredentials(files *DiscoveredFiles, cfg *Config) []ProofResult {
	// Check every MR's providerConfigRef points to a valid ProviderConfig
	// If live_cluster enabled, also check credential expiry
	// Uses: NewCredentialValid, NewMrCredentialed
	return nil // scaffold
}

// --- Argo Rollouts validators ---

func validateRollouts(files *DiscoveredFiles, cfg *Config) []ProofResult {
	var results []ProofResult

	for _, rollout := range files.Rollouts {
		weights := ExtractCanaryWeights(rollout)

		// Check weight monotonicity
		for i := 1; i < len(weights); i++ {
			if weights[i] < weights[i-1] {
				results = append(results, ProofResult{
					Category: "rollouts",
					Check:    "weight-monotonic",
					Passed:   false,
					Message:  fmt.Sprintf("Canary weight decreases: %d → %d", weights[i-1], weights[i]),
					File:     rollout.FilePath,
					Line:     rollout.Line,
					Fix:      "Ensure setWeight steps are non-decreasing",
				})
			}
		}

		// Check ingress capability
		if cfg != nil {
			// Detect ingress type from cluster or annotations
			// Check cfg.Ingress[type].WeightedRouting == true
		}
	}

	return results
}

// --- Argo Workflows validators ---

func validateWorkflowDAGs(files *DiscoveredFiles) []ProofResult {
	var results []ProofResult

	for _, wf := range files.Workflows {
		edges := ExtractDAGEdges(wf)

		// Attempt topological sort — if it fails, there's a cycle
		if hasCycle(edges) {
			results = append(results, ProofResult{
				Category: "workflows",
				Check:    "dag-acyclic",
				Passed:   false,
				Message:  "DAG contains a cycle",
				File:     wf.FilePath,
				Line:     wf.Line,
				Fix:      "Remove circular dependency in DAG tasks",
			})
		}
	}

	return results
}

func hasCycle(edges []DAGEdge) bool {
	// Kahn's algorithm for cycle detection
	inDegree := map[string]int{}
	adj := map[string][]string{}

	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
		inDegree[e.To]++
		if _, ok := inDegree[e.From]; !ok {
			inDegree[e.From] = 0
		}
	}

	var queue []string
	for node, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, node)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, neighbor := range adj[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	return visited != len(inDegree)
}

func validateWorkflowParams(files *DiscoveredFiles) []ProofResult {
	// For each parameter handoff between steps, check type match
	// Uses: NewParamTypeMatch
	return nil // scaffold
}
