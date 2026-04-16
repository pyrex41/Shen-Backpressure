// ShenPlane controller scaffold.
//
// This shows the structure of a controller that uses generated guard types
// to enforce invariants during reconciliation. The key property: if this
// code compiles, every resource it provisions satisfies the Shen spec.
//
// In production, this would use controller-runtime (sigs.k8s.io/controller-runtime)
// and register as a Kubernetes controller + admission webhook.
package main

import "fmt"

// --- These types would be imported from the generated shenguard package ---
// Shown inline for clarity. In practice: import "shenplane/shenguard"

// reconcileDatabaseClaim is the core reconciliation logic.
// It demonstrates how guard types make invalid provisioning impossible.
//
// The flow:
//   1. Parse YAML claim fields through guard constructors (validation)
//   2. Compose security proofs (encryption + network + IAM)
//   3. Expand claim into concrete cloud resources (typed, not patched)
//   4. Prove cross-resource consistency
//   5. Build deployment-safe proof
//   6. Only then: actually provision
func reconcileDatabaseClaim() {
	fmt.Println("ShenPlane Controller — Database Reconciler")
	fmt.Println()
	fmt.Println("Reconciliation flow:")
	fmt.Println()
	fmt.Println("  1. PARSE CLAIM (guard constructors validate every field)")
	fmt.Println("     id       := NewResourceId(claim.Name)")
	fmt.Println("     engine   := NewDbEngine(claim.Spec.Engine)")
	fmt.Println("     size     := NewSizeTier(claim.Spec.Size)")
	fmt.Println("     storage  := NewStorageGb(claim.Spec.StorageGB)")
	fmt.Println("     region   := NewCloudRegion(claim.Spec.Region)")
	fmt.Println("     tags     := NewOrgTags(team, costCenter, env)")
	fmt.Println("     dbClaim  := NewDbClaim(id, engine, size, storage, region, tags)")
	fmt.Println()
	fmt.Println("  2. COMPOSE SECURITY PROOFS")
	fmt.Println("     enc     := NewEncryptionProof(NewEncryptionAlgo(\"aes-256\"), true, true)")
	fmt.Println("     net     := NewNetworkProof(NewNetworkMode(\"private\"), false)")
	fmt.Println("     iam     := NewIamProof(NewIamScope(\"least-privilege\"), false)")
	fmt.Println("     sec     := NewSecurityProof(enc, net, iam)")
	fmt.Println()
	fmt.Println("  3. BUILD SECURE-DB (claim + security)")
	fmt.Println("     secureDb := NewSecureDb(dbClaim, sec)")
	fmt.Println("     // Cannot reach this line without ALL proofs passing.")
	fmt.Println()
	fmt.Println("  4. EXPAND INTO CLOUD RESOURCES")
	fmt.Println("     instance     := createRDSInstance(engine, size, region)")
	fmt.Println("     subnetGroup  := createSubnetGroup(region)")
	fmt.Println("     secGroup     := createSecurityGroup(net)")
	fmt.Println("     iamRole      := createIAMRole(iam)")
	fmt.Println()
	fmt.Println("  5. PROVE CONSISTENCY")
	fmt.Println("     regionProof  := NewSameRegion(instance, subnetGroup)")
	fmt.Println("     networkProof := NewInNetwork(instance, network)")
	fmt.Println()
	fmt.Println("  6. BUILD EXPANSION PROOF")
	fmt.Println("     expansion := NewDbExpansion(")
	fmt.Println("         secureDb, instance, subnetGroup, secGroup, iamRole,")
	fmt.Println("         regionProof, networkProof,")
	fmt.Println("     )")
	fmt.Println()
	fmt.Println("  7. BUILD DEPLOYMENT-SAFE PROOF")
	fmt.Println("     plan     := NewReconcilePlan(handle, action, provenance)")
	fmt.Println("     provider := NewProviderReady(providerType, cred, expiry, true)")
	fmt.Println("     safe     := NewDeploymentSafe(plan, sec, provider)")
	fmt.Println("     // If we reach here: deployment is provably safe.")
	fmt.Println("     // Actually provision the cloud resources.")
	fmt.Println()
	fmt.Println("If ANY constructor rejects at any step, reconciliation halts")
	fmt.Println("with a precise error. No partial provisioning. No silent failures.")
}

// reconcileAdmission shows how the webhook works.
// Same guard constructors, but called at admission time (before storage).
func reconcileAdmission() {
	fmt.Println()
	fmt.Println("Admission Webhook flow:")
	fmt.Println()
	fmt.Println("  1. Receive AdmissionReview (raw YAML)")
	fmt.Println("  2. Unmarshal spec fields")
	fmt.Println("  3. Call guard constructors for every field")
	fmt.Println("  4. Collect ALL errors (don't stop at first)")
	fmt.Println("  5. If any errors: deny with proof-derived messages")
	fmt.Println("  6. If no errors: admit")
	fmt.Println()
	fmt.Println("Error messages are derived from the constructor failures:")
	fmt.Println("  NewDbEngine(\"sqlite\")     → \"sqlite\" not in {postgres, mysql, mariadb}")
	fmt.Println("  NewStorageGb(5)           → 5 < minimum 10")
	fmt.Println("  NewCloudRegion(\"moon-1\")  → \"moon-1\" not in valid regions")
	fmt.Println("  NewOrgTags(\"\", ...)       → costCenter must not be empty")
}

func main() {
	reconcileDatabaseClaim()
	reconcileAdmission()
}
