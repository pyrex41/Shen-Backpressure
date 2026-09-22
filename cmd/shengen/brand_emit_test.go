package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// generateBoth renders a spec with brands off and on.
func generateBoth(t *testing.T, spec string) (plain, branded string) {
	t.Helper()
	types, defines, err := parseSpec_string(spec)
	if err != nil {
		t.Fatal(err)
	}
	build := func() (*SymbolTable, []Datatype) {
		st := newSymbolTable()
		st.Build(types)
		for i := range defines {
			st.Defines[defines[i].Name] = &defines[i]
		}
		return st, types
	}
	st1, ty1 := build()
	plain = generateGo(ty1, st1, "shenguard", "test.shen")
	st2, ty2 := build()
	branded = generateGoBrands(ty2, st2, "shenguard", "test.shen", InferBrands(ty2, st2))
	return plain, branded
}

const paymentSpecForBrands = `(datatype account-id
  X : string;
  ==============
  X : account-id;)

(datatype amount
  X : number;
  (>= X 0) : verified;
  ====================
  X : amount;)

(datatype transaction
  Amount : amount;
  From : account-id;
  To : account-id;
  ===================================
  [Amount From To] : transaction;)

(datatype balance-invariant
  Bal : number;
  Tx : transaction;
  (>= Bal (head Tx)) : verified;
  =======================================
  [Bal Tx] : balance-checked;)

(datatype safe-transfer
  Tx : transaction;
  Check : balance-checked;
  =============================
  [Tx Check] : safe-transfer;)`

// TestBrandsOffIsByteIdenticalToPreBrandEmitter is the compatibility
// guarantee: --no-brands (the default) must not move a single byte, so
// every example that has not opted in keeps passing its drift audit.
func TestBrandsOffIsByteIdenticalToPreBrandEmitter(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "examples", "payment", "specs", "core.shen"),
		filepath.Join("..", "..", "examples", "multi-tenant-api", "specs", "core.shen"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Skipf("spec not readable: %v", err)
		}
		types, defines, err := parseSpec_string(string(data))
		if err != nil {
			t.Fatal(err)
		}
		st := newSymbolTable()
		st.Build(types)
		for i := range defines {
			st.Defines[defines[i].Name] = &defines[i]
		}
		viaOldPath := generateGo(types, st, "shenguard", path)

		st2 := newSymbolTable()
		st2.Build(types)
		for i := range defines {
			st2.Defines[defines[i].Name] = &defines[i]
		}
		viaNilBrands := generateGoBrands(types, st2, "shenguard", path, nil)

		if viaOldPath != viaNilBrands {
			t.Errorf("%s: nil brand table changed the output", path)
		}
		if strings.Contains(viaOldPath, "witness") || strings.Contains(viaOldPath, "Brand]") {
			t.Errorf("%s: brand machinery leaked into the unbranded output", path)
		}
	}
}

// TestBrandedGoEmitsPairedConstructorSignature is the headline claim of
// W1: the pairing of a proof to its subject is in the signature.
func TestBrandedGoEmitsPairedConstructorSignature(t *testing.T) {
	_, branded := generateBoth(t, paymentSpecForBrands)

	want := []string{
		"type Brand interface{}",
		"type witness struct{ minted bool }",
		"func mint() witness { return witness{minted: true} }",
		"type Transaction[B Brand] struct {",
		"func NewTransaction[B Brand](amount Amount, from AccountId, to AccountId) Transaction[B] {",
		"type BalanceChecked[B Brand] struct {",
		"	tx Transaction[B]",
		"func NewBalanceChecked[B Brand](bal float64, tx Transaction[B]) (BalanceChecked[B], error) {",
		"func NewSafeTransfer[B Brand](tx Transaction[B], check BalanceChecked[B]) SafeTransfer[B] {",
	}
	for _, w := range want {
		if !strings.Contains(branded, w) {
			t.Errorf("branded output missing:\n\t%s", w)
		}
	}
}

// TestBrandedGoEmitsWitnessOnEveryGeneratedType covers the zero-value
// half: every generated struct carries the mint mark, every accessor
// checks it, and every consuming constructor checks its arguments.
func TestBrandedGoEmitsWitnessOnEveryGeneratedType(t *testing.T) {
	_, branded := generateBoth(t, paymentSpecForBrands)

	for _, typeName := range []string{"AccountId", "Amount", "Transaction", "BalanceChecked", "SafeTransfer"} {
		decl := "type " + typeName
		idx := strings.Index(branded, decl)
		if idx < 0 {
			t.Fatalf("no declaration for %s", typeName)
		}
		body := branded[idx:]
		if end := strings.Index(body, "\n}\n"); end > 0 {
			body = body[:end]
		}
		if !strings.Contains(body, "valid witness") {
			t.Errorf("%s has no witness field:\n%s", typeName, body)
		}
	}

	// Accessors check their own witness.
	for _, accessor := range []string{
		`func (t Amount) Val() float64 { t.valid.mustBeMinted("Amount"); return t.v }`,
		`func (t Transaction[B]) Amount() Amount { t.valid.mustBeMinted("Transaction"); return t.amount }`,
		`func (t SafeTransfer[B]) Check() BalanceChecked[B] { t.valid.mustBeMinted("SafeTransfer"); return t.check }`,
	} {
		if !strings.Contains(branded, accessor) {
			t.Errorf("missing guarded accessor:\n\t%s", accessor)
		}
	}

	// Consuming constructors check their arguments, so a forged value
	// cannot be laundered by passing it up the chain.
	for _, check := range []string{
		"func NewSafeTransfer[B Brand](tx Transaction[B], check BalanceChecked[B]) SafeTransfer[B] {\n\ttx.valid.mustBeMinted(\"Transaction\")\n\tcheck.valid.mustBeMinted(\"BalanceChecked\")",
		"func NewBalanceChecked[B Brand](bal float64, tx Transaction[B]) (BalanceChecked[B], error) {\n\ttx.valid.mustBeMinted(\"Transaction\")",
	} {
		if !strings.Contains(branded, check) {
			t.Errorf("missing consuming-constructor witness check:\n%s", check)
		}
	}
}

// TestBrandedFailedConstructorReturnsUnmintedZero pins the
// failed-constructor decision from the roadmap: the shape of the call
// site does not change, but the value handed back with the error is
// unminted, so a dropped error becomes a loud panic instead of a
// silent forgery.
func TestBrandedGuardedFailurePathReturnsUnmintedZero(t *testing.T) {
	_, branded := generateBoth(t, paymentSpecForBrands)
	if !strings.Contains(branded, `return BalanceChecked[B]{}, fmt.Errorf("bal must be >= tx.amount")`) {
		t.Errorf("guarded failure path should return the unminted zero value at the branded type")
	}
	if strings.Contains(branded, "valid: mint()}, fmt.Errorf") {
		t.Errorf("a failure path minted its return value")
	}
}

// TestBrandedSumTypeIsGenericInterface covers the multi-tenant shape:
// the sum-type interface is parameterized, and each variant's marker
// method is declared on the variant at its own brand, so
// HumanPrincipal[B] satisfies AuthenticatedPrincipal[B].
func TestBrandedSumTypeIsGenericInterface(t *testing.T) {
	_, branded := generateBoth(t, `(datatype user-id
  X : string;
  ==============
  X : user-id;)

(datatype parsed-claims
  Sub : user-id;
  Exp : number;
  (> Exp 0) : verified;
  ==================================
  [Sub Exp] : parsed-claims;)

(datatype human-principal
  Claims : parsed-claims;
  ===========================
  Claims : authenticated-principal;)

(datatype service-principal
  Secret : string;
  (not (= Secret "")) : verified;
  ============================
  Secret : authenticated-principal;)

(datatype tenant-access
  Principal : authenticated-principal;
  IsMember : boolean;
  (= IsMember true) : verified;
  ================================
  [Principal IsMember] : tenant-access;)`)

	for _, w := range []string{
		"type AuthenticatedPrincipal[B Brand] interface {",
		"func (t HumanPrincipal[B]) isAuthenticatedPrincipal() {}",
		"func (t ServicePrincipal[B]) isAuthenticatedPrincipal() {}",
		"	principal AuthenticatedPrincipal[B]",
		"func NewTenantAccess[B Brand](principal AuthenticatedPrincipal[B], isMember bool) (TenantAccess[B], error) {",
	} {
		if !strings.Contains(branded, w) {
			t.Errorf("branded sum-type output missing:\n\t%s", w)
		}
	}
	// An interface has no field to reach, so no witness check is
	// emitted for a sum-typed parameter.
	if strings.Contains(branded, `principal.valid.mustBeMinted`) {
		t.Errorf("emitted a witness check on an interface-typed parameter")
	}
}

// TestBrandedGoCompiles type-checks the generated package with the real
// toolchain: generics plus phantom parameters plus a generic interface
// is exactly where Go's method-set rules bite.
func TestBrandedGoCompiles(t *testing.T) {
	specs := map[string]string{
		"payment":      paymentSpecForBrands,
		"multi-tenant": mustReadSpec(t, filepath.Join("..", "..", "examples", "multi-tenant-api", "specs", "core.shen")),
	}
	for name, spec := range specs {
		_, branded := generateBoth(t, spec)
		// The fixtures carry no `:runtime-via` annotation, so the
		// generated package must be self-contained.
		if strings.Contains(branded, "evalhost") {
			t.Fatalf("%s: fixture should not need the evalhost import", name)
		}
		if err := buildGoPackage(t, branded, "shenguard", ""); err != nil {
			t.Errorf("%s: branded output does not compile: %v", name, err)
		}
	}
}

// TestBrandedWitnessPanicsOnEmptyLiteral is the runtime half of the
// zero-value defence, exercised end to end: `BalanceChecked[b]{}` is
// legal Go from any package (it names no unexported field), but reading
// it panics instead of passing for a proof.
func TestBrandedWitnessPanicsOnEmptyLiteral(t *testing.T) {
	_, branded := generateBoth(t, paymentSpecForBrands)

	const probe = `package shenguard

type probeBrand struct{}

// ForgeAndRead builds a BalanceChecked by empty literal — no
// constructor, no premise check — and reads it.
func ForgeAndRead() (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = r.(string)
		}
	}()
	forged := BalanceChecked[probeBrand]{}
	_ = forged.Bal()
	return "NO PANIC"
}
`
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "shenguard")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "go.mod"), "module brandprobe\n\ngo 1.24\n")
	write(filepath.Join(pkgDir, "guards_gen.go"), branded)
	write(filepath.Join(pkgDir, "probe.go"), probe)
	write(filepath.Join(dir, "main.go"), `package main

import (
	"fmt"

	"brandprobe/shenguard"
)

func main() { fmt.Println(shenguard.ForgeAndRead()) }
`)

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run failed: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); !strings.Contains(got, "forged BalanceChecked value") {
		t.Errorf("empty literal was readable; got %q", got)
	}
}

func mustReadSpec(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("spec not readable: %v", err)
	}
	return string(data)
}
