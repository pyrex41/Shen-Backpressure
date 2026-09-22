// Tests for the TypeScript GDP-brand inference and emission (W1).
//
// Mirrors cmd/shengen/brand_inference_test.go and
// cmd/shengen/brand_emit_test.go: the same golden tables (rendered with
// TypeScript's `<B>` instead of Go's `[B]`), the same rule-level unit
// tests, and emission assertions for the paired constructor signature,
// the witness placement, and the unbranded-output guarantee.

import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { SymbolTable, generateTs, parseSpecString } from "./shengen.ts";
import { BrandTable, inferBrands } from "./brand_inference.ts";

function brandTableFor(spec: string): BrandTable {
  const parsed = parseSpecString(spec);
  const st = new SymbolTable();
  st.build(parsed.datatypes);
  st.registerDefines(parsed.defines);
  return inferBrands(parsed.datatypes, st);
}

function specFile(...parts: string[]): string {
  return readFileSync(join(import.meta.dirname, "..", "..", ...parts), "utf8");
}

function assertGolden(got: string, want: string): void {
  const normalize = (s: string) =>
    s
      .trim()
      .split("\n")
      .map((l) => l.trim().split(/\s+/).join(" "))
      .filter((l) => l !== "")
      .join("\n");
  assert.equal(normalize(got), normalize(want));
}

function generateBoth(spec: string): { plain: string; branded: string } {
  const parsed = parseSpecString(spec);
  const build = () => {
    const st = new SymbolTable();
    st.build(parsed.datatypes);
    st.registerDefines(parsed.defines);
    return st;
  };
  const st1 = build();
  const plain = generateTs(parsed.datatypes, st1, "t.shen");
  const st2 = build();
  const branded = generateTs(parsed.datatypes, st2, "t.shen", {
    brands: inferBrands(parsed.datatypes, st2),
  });
  return { plain, branded };
}

const PAYMENT_SPEC = `(datatype account-id
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
  [Tx Check] : safe-transfer;)`;

// ============================================================================
// Golden brand tables — same rows as the Go implementation
// ============================================================================

test("brand table: payment spec golden", () => {
  const bt = brandTableFor(
    specFile("examples", "payment", "specs", "core.shen")
  );
  assertGolden(
    bt.toString(),
    `
account-id               unbranded  AccountId
account-state            unbranded  AccountState  premises(- -)
amount                   unbranded  Amount
balance-checked          inherited  BalanceChecked<B>  premises(- B)
safe-transfer            inherited  SafeTransfer<B>  premises(B B)
transaction              minted     Transaction<B>  premises(- - -)
`
  );
});

test("brand table: multi-tenant spec golden", () => {
  const bt = brandTableFor(
    specFile("examples", "multi-tenant-api", "specs", "core.shen")
  );
  assertGolden(
    bt.toString(),
    `
authenticated-principal  sum        AuthenticatedPrincipal<B>
authenticated-user       inherited  AuthenticatedUser<B>  premises(B -)
human-principal          inherited  HumanPrincipal<B>  premises(B)
jwt-audience             unbranded  JwtAudience
jwt-issuer               unbranded  JwtIssuer
parsed-claims            minted     ParsedClaims<B>  premises(- - - -)
resource-access          inherited  ResourceAccess<B>  premises(B - -)
resource-id              unbranded  ResourceId
service-credential       minted     ServiceCredential<B>  premises(- -)
service-id               unbranded  ServiceId
service-principal        inherited  ServicePrincipal<B>  premises(B)
tenant-access            inherited  TenantAccess<B>  premises(B - -)
tenant-id                unbranded  TenantId
user-id                  unbranded  UserId
verified-jwt             inherited  VerifiedJwt<B>  premises(B -)
`
  );
});

test("brand table: a (list X) premise carries X's brand", () => {
  const bt = brandTableFor(
    specFile("examples", "multilang-paired", "specs", "core.shen")
  );
  assertGolden(
    bt.toString(),
    `
cart                     inherited  Cart<B>  premises(- - B)
cart-item                minted     CartItem<B>  premises(- -)
customer-id              unbranded  CustomerId
discount-eligible        inherited  DiscountEligible<B>  premises(B - - -)
sku                      unbranded  Sku
`
  );
});

// ============================================================================
// Rule-level unit tests
// ============================================================================

test("brands unify across a nested field path", () => {
  const bt = brandTableFor(PAYMENT_SPEC);
  const args = bt.lookup("safe-transfer")!.premiseArgs;
  assert.equal(args.length, 2);
  assert.equal(args[0][0], args[1][0], "Tx and Check must share a brand");
  assert.equal(bt.params("safe-transfer").length, 1);
  assert.equal(bt.lookup("transaction")!.minted, true);
  assert.equal(bt.lookup("balance-checked")!.minted, false);
});

test("brands unify when a verified premise mentions both premises", () => {
  const bt = brandTableFor(`(datatype left
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
  [L R] : joined;)`);
  const args = bt.lookup("joined")!.premiseArgs;
  assert.equal(args[0][0], args[1][0]);
  assert.equal(bt.params("joined").length, 1);
});

test("unrelated premises of the same type keep distinct brands", () => {
  const bt = brandTableFor(`(datatype thing
  X : string;
  ==============
  [X] : thing;)

(datatype two-things
  A : thing;
  B : thing;
  ==============
  [A B] : two-things;)`);
  assert.equal(bt.params("two-things").length, 2);
  const args = bt.lookup("two-things")!.premiseArgs;
  assert.notEqual(args[0][0], args[1][0]);
});

test("value wrappers and unconsumed composites stay unbranded", () => {
  const bt = brandTableFor(PAYMENT_SPEC);
  for (const n of ["account-id", "amount"]) {
    assert.equal(bt.isBranded(n), false, `${n} should be unbranded`);
  }
  const lonely = brandTableFor(`(datatype leaf
  X : string;
  ==============
  X : leaf;)

(datatype lonely
  A : leaf;
  B : leaf;
  ==============
  [A B] : lonely;)`);
  assert.equal(lonely.isBranded("lonely"), false);
});

test("inferBrands is deterministic", () => {
  const spec = specFile("examples", "multi-tenant-api", "specs", "core.shen");
  const first = brandTableFor(spec).toString();
  for (let i = 0; i < 10; i++) {
    assert.equal(brandTableFor(spec).toString(), first);
  }
});

// ============================================================================
// Emission
// ============================================================================

test("brands off: output is byte-identical to the pre-brand emitter", () => {
  for (const parts of [
    ["examples", "payment", "specs", "core.shen"],
    ["examples", "multi-tenant-api", "specs", "core.shen"],
  ]) {
    const spec = specFile(...parts);
    const parsed = parseSpecString(spec);
    const st = new SymbolTable();
    st.build(parsed.datatypes);
    st.registerDefines(parsed.defines);
    const viaOldPath = generateTs(parsed.datatypes, st, "t.shen");

    const st2 = new SymbolTable();
    st2.build(parsed.datatypes);
    st2.registerDefines(parsed.defines);
    const viaNullBrands = generateTs(parsed.datatypes, st2, "t.shen", {
      brands: null,
    });

    assert.equal(viaOldPath, viaNullBrands, parts.join("/"));
    assert.ok(!viaOldPath.includes("_witness"), "witness leaked into unbranded output");
    assert.ok(!viaOldPath.includes("extends Brand"), "brand leaked into unbranded output");
  }
});

test("branded emission: paired constructor signature", () => {
  const { branded } = generateBoth(PAYMENT_SPEC);
  for (const want of [
    "export type Brand = unknown;",
    "type Witness = typeof MINTED;",
    "export class Transaction<B extends Brand> {",
    "  static createOrThrow<B extends Brand>(amount: Amount, from: AccountId, to: AccountId): Transaction<B> {",
    "  private readonly _tx: Transaction<B>;",
    "  static createOrThrow<B extends Brand>(bal: number, tx: Transaction<B>): BalanceChecked<B> {",
    "  static createOrThrow<B extends Brand>(tx: Transaction<B>, check: BalanceChecked<B>): SafeTransfer<B> {",
    "export function mustSafeTransfer<B extends Brand>(tx: Transaction<B>, check: BalanceChecked<B>): SafeTransfer<B> {",
  ]) {
    assert.ok(branded.includes(want), `branded output missing:\n\t${want}\n`);
  }
});

test("branded emission: witness on every class, checked in accessors and consumers", () => {
  const { branded } = generateBoth(PAYMENT_SPEC);
  for (const cls of ["AccountId", "Amount", "Transaction", "BalanceChecked", "SafeTransfer"]) {
    const idx = branded.indexOf(`export class ${cls}`);
    assert.ok(idx >= 0, `no class ${cls}`);
    const body = branded.slice(idx, branded.indexOf("\n}\n", idx));
    assert.ok(
      body.includes("private readonly _witness: Witness = mint();"),
      `${cls} has no witness`
    );
  }
  for (const want of [
    'val(): number { mustBeMinted(this._witness, "Amount"); return this._v; }',
    'amount(): Amount { mustBeMinted(this._witness, "Transaction"); return this._amount; }',
    'mustBeMinted((tx as unknown as { _witness?: Witness })._witness, "Transaction");',
  ]) {
    assert.ok(branded.includes(want), `branded output missing:\n\t${want}\n`);
  }
});

test("branded emission: phantom marker makes the brand invariant", () => {
  const { branded } = generateBoth(PAYMENT_SPEC);
  // `(b: B) => B` is what stops Transaction<A> being assignable to
  // Transaction<C>; an unused type parameter would be bivariant and the
  // pairing guarantee would evaporate silently.
  assert.ok(
    branded.includes("declare private readonly _brandB: (b: B) => B;"),
    "missing invariant phantom brand marker"
  );
});

test("branded emission: sum type is a generic union over the same brand", () => {
  const spec = specFile("examples", "multi-tenant-api", "specs", "core.shen");
  const { branded } = generateBoth(spec);
  for (const want of [
    "export type AuthenticatedPrincipal<B> = HumanPrincipal<B> | ServicePrincipal<B>;",
    "  private readonly _principal: AuthenticatedPrincipal<B>;",
    "  static createOrThrow<B extends Brand>(principal: AuthenticatedPrincipal<B>, tenant: TenantId, isMember: boolean): TenantAccess<B> {",
  ]) {
    assert.ok(branded.includes(want), `branded output missing:\n\t${want}\n`);
  }
});

test("branded emission: a (list X) premise is rendered at X's brand", () => {
  const spec = specFile("examples", "multilang-paired", "specs", "core.shen");
  const { branded } = generateBoth(spec);
  assert.ok(
    branded.includes("private readonly _items: CartItem<B>[];"),
    `list field should carry the element brand:\n${branded.slice(0, 200)}`
  );
  assert.ok(
    branded.includes("export function hasBulkLine<B extends Brand>(arg0: CartItem<B>[]): boolean {"),
    "a helper over branded types should be generic over their brands"
  );
});

test("branded emission: failing factory throws instead of returning a value", () => {
  const { branded } = generateBoth(PAYMENT_SPEC);
  assert.ok(
    branded.includes("if (!(bal >= tx.amount().val())) throw new Error(`bal must be >= tx.amount()`);"),
    "guarded factory should throw on a failed premise"
  );
  // tryCreate is the only non-throwing path and it returns Error, never
  // an unminted instance.
  assert.ok(
    branded.includes("static tryCreate<B extends Brand>(bal: number, tx: Transaction<B>): BalanceChecked<B> | Error {"),
    "tryCreate should return Error, not a forged value"
  );
});
