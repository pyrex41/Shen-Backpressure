package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// brandTableFor parses a spec string and returns the inferred table.
func brandTableFor(t *testing.T, spec string) *BrandTable {
	t.Helper()
	types, err := parseFile_string(spec)
	if err != nil {
		t.Fatal(err)
	}
	st := newSymbolTable()
	st.Build(types)
	return InferBrands(types, st)
}

// brandTableForFile parses a spec on disk (used for the golden tables
// of the committed example specs).
func brandTableForFile(t *testing.T, path string) *BrandTable {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("spec not readable (%v)", err)
	}
	return brandTableFor(t, string(data))
}

func assertGolden(t *testing.T, got, want string) {
	t.Helper()
	normalize := func(s string) string {
		var out []string
		for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
			if l := strings.Join(strings.Fields(line), " "); l != "" {
				out = append(out, l)
			}
		}
		return strings.Join(out, "\n")
	}
	if normalize(got) != normalize(want) {
		t.Errorf("brand table mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// ============================================================================
// Golden brand tables for the committed example specs
// ============================================================================

// TestBrandTablePaymentGolden pins the inferred brand table for
// examples/payment/specs/core.shen.
//
// The load-bearing rows:
//
//   - transaction is `minted`: nothing inside it is branded, but
//     balance-invariant and safe-transfer reason about it, so it is the
//     head of a proof chain and mints its own brand.
//   - balance-checked is `inherited` at transaction's brand — the proof
//     is about that transaction, not about transactions in general.
//   - safe-transfer's two premises (Tx, Check) unify, because
//     balance-checked reaches transaction along a nested field path. One
//     surviving brand, so NewSafeTransfer demands a matching pair.
//   - amount / account-id stay unbranded: they are values, not evidence.
//   - account-state stays unbranded: no rule consumes it and none of
//     its constituents is branded.
func TestBrandTablePaymentGolden(t *testing.T) {
	bt := brandTableForFile(t, filepath.Join("..", "..", "examples", "payment", "specs", "core.shen"))
	assertGolden(t, bt.String(), `
account-id               unbranded  AccountId
account-state            unbranded  AccountState  premises(- -)
amount                   unbranded  Amount
balance-checked          inherited  BalanceChecked[B]  premises(- B)
safe-transfer            inherited  SafeTransfer[B]  premises(B B)
transaction              minted     Transaction[B]  premises(- - -)
`)
}

// TestBrandTableMultiTenantGolden pins the inferred brand table for
// examples/multi-tenant-api/specs/core.shen — the full authorization
// chain. One brand threads the whole chain:
//
//	parsed-claims (minted) → verified-jwt → authenticated-user
//	  → human-principal ─┐
//	                     ├→ authenticated-principal (sum)
//	  service-credential (minted) → service-principal ─┘
//	  → tenant-access → resource-access
//
// So a ResourceAccess[B] is evidence about the *specific* JWT that B
// came from, and cannot be paired with another chain's TenantAccess.
func TestBrandTableMultiTenantGolden(t *testing.T) {
	bt := brandTableForFile(t, filepath.Join("..", "..", "examples", "multi-tenant-api", "specs", "core.shen"))
	assertGolden(t, bt.String(), `
authenticated-principal  sum        AuthenticatedPrincipal[B]
authenticated-user       inherited  AuthenticatedUser[B]  premises(B -)
human-principal          inherited  HumanPrincipal[B]  premises(B)
jwt-audience             unbranded  JwtAudience
jwt-issuer               unbranded  JwtIssuer
parsed-claims            minted     ParsedClaims[B]  premises(- - - -)
resource-access          inherited  ResourceAccess[B]  premises(B - -)
resource-id              unbranded  ResourceId
service-credential       minted     ServiceCredential[B]  premises(- -)
service-id               unbranded  ServiceId
service-principal        inherited  ServicePrincipal[B]  premises(B)
tenant-access            inherited  TenantAccess[B]  premises(B - -)
tenant-id                unbranded  TenantId
user-id                  unbranded  UserId
verified-jwt             inherited  VerifiedJwt[B]  premises(B -)
`)
}

// ============================================================================
// Rule-level unit tests
// ============================================================================

// TestBrandUnificationViaFieldPath is the safe-transfer shape reduced to
// its essentials: two premises, one of which structurally contains the
// other, must share a brand.
func TestBrandUnificationViaFieldPath(t *testing.T) {
	bt := brandTableFor(t, `
(datatype thing
  X : string;
  ==============
  [X] : thing;)

(datatype proof
  T : thing;
  Ok : boolean;
  (= Ok true) : verified;
  ==============
  [T Ok] : proof;)

(datatype paired
  T : thing;
  P : proof;
  ==============
  [T P] : paired;)
`)
	if got := bt.Params("thing"); len(got) != 1 {
		t.Fatalf("thing should mint one brand, got %v", got)
	}
	if !bt.Lookup("thing").Minted {
		t.Errorf("thing should be minted (head of the chain)")
	}
	pairedArgs := bt.Lookup("paired").PremiseArgs
	if len(pairedArgs) != 2 || len(pairedArgs[0]) != 1 || len(pairedArgs[1]) != 1 {
		t.Fatalf("paired premises should both be branded: %v", pairedArgs)
	}
	if pairedArgs[0][0] != pairedArgs[1][0] {
		t.Errorf("paired premises should unify to one brand, got %v", pairedArgs)
	}
	if got := len(bt.Params("paired")); got != 1 {
		t.Errorf("paired should have 1 surviving brand, got %d", got)
	}
}

// TestBrandUnificationViaVerifiedPremise covers rule 2's other half:
// two premises of unrelated types that a verified premise relates.
func TestBrandUnificationViaVerifiedPremise(t *testing.T) {
	bt := brandTableFor(t, `
(datatype left
  X : string;
  ==============
  [X] : left;)

(datatype right
  Y : string;
  ==============
  [Y] : right;)

(datatype joined
  L : left;
  R : right;
  (= (head L) (head R)) : verified;
  ==============
  [L R] : joined;)
`)
	args := bt.Lookup("joined").PremiseArgs
	if len(args) != 2 || len(args[0]) != 1 || len(args[1]) != 1 {
		t.Fatalf("both premises should be branded: %v", args)
	}
	if args[0][0] != args[1][0] {
		t.Errorf("verified premise relating L and R should unify their brands: %v", args)
	}
	if got := len(bt.Params("joined")); got != 1 {
		t.Errorf("joined should have 1 brand, got %d", got)
	}
}

// TestBrandsStayDistinctWhenUnrelated is the negative case: two
// premises of the same branded type that no premise relates keep two
// brands, so the conclusion is parameterized by both. Without this,
// "the check for tx1" and "the check for tx2" would be interchangeable.
func TestBrandsStayDistinctWhenUnrelated(t *testing.T) {
	bt := brandTableFor(t, `
(datatype thing
  X : string;
  ==============
  [X] : thing;)

(datatype two-things
  A : thing;
  B : thing;
  ==============
  [A B] : two-things;)
`)
	params := bt.Params("two-things")
	if len(params) != 2 {
		t.Fatalf("two unrelated thing premises should survive as 2 brands, got %v", params)
	}
	args := bt.Lookup("two-things").PremiseArgs
	if args[0][0] == args[1][0] {
		t.Errorf("unrelated premises must not share a brand: %v", args)
	}
}

// TestPrimitiveWrappersAreNotBranded pins the decision that value
// wrappers stay unparameterized.
func TestPrimitiveWrappersAreNotBranded(t *testing.T) {
	bt := brandTableFor(t, `
(datatype plain
  X : string;
  ==============
  X : plain;)

(datatype constrained-thing
  X : number;
  (>= X 0) : verified;
  ==============
  X : constrained-thing;)
`)
	for _, n := range []string{"plain", "constrained-thing"} {
		if bt.IsBranded(n) {
			t.Errorf("%s should be unbranded, got %v", n, bt.Params(n))
		}
	}
}

// TestUnconsumedCompositeIsNotBranded pins account-state's shape: a
// composite nobody reasons about needs no brand, so the host program is
// not infected with type parameters for nothing.
func TestUnconsumedCompositeIsNotBranded(t *testing.T) {
	bt := brandTableFor(t, `
(datatype leaf
  X : string;
  ==============
  X : leaf;)

(datatype lonely
  A : leaf;
  B : leaf;
  ==============
  [A B] : lonely;)
`)
	if bt.IsBranded("lonely") {
		t.Errorf("unconsumed composite should be unbranded, got %v", bt.Params("lonely"))
	}
}

// TestSumTypeBrandsPropagateToVariants checks the generic-interface
// lowering: the interface and every variant carry the same arity, so
// `HumanPrincipal[B]` satisfies `AuthenticatedPrincipal[B]`.
func TestSumTypeBrandsPropagateToVariants(t *testing.T) {
	bt := brandTableFor(t, `
(datatype inner
  X : string;
  ==============
  [X] : inner;)

(datatype variant-a
  I : inner;
  ==============
  I : either;)

(datatype variant-b
  X : string;
  ==============
  X : either;)

(datatype consumer
  E : either;
  Ok : boolean;
  (= Ok true) : verified;
  ==============
  [E Ok] : consumer;)
`)
	if n := len(bt.Params("either")); n != 1 {
		t.Fatalf("sum type should carry 1 brand, got %d", n)
	}
	for _, v := range []string{"variant-a", "variant-b"} {
		if n := len(bt.Params(v)); n != 1 {
			t.Errorf("variant %s should carry the interface's brand arity, got %d", v, n)
		}
	}
	if n := len(bt.Params("consumer")); n != 1 {
		t.Errorf("consumer should inherit the sum type's brand, got %d", n)
	}
}

// TestInferBrandsIsDeterministic guards against map-iteration order
// leaking into the table (the emitters are byte-compared by the audit
// gate, so this matters).
func TestInferBrandsIsDeterministic(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "examples", "multi-tenant-api", "specs", "core.shen"))
	if err != nil {
		t.Skipf("spec not readable: %v", err)
	}
	first := brandTableFor(t, string(spec)).String()
	for i := 0; i < 20; i++ {
		if got := brandTableFor(t, string(spec)).String(); got != first {
			t.Fatalf("brand table is not deterministic on run %d", i)
		}
	}
}
