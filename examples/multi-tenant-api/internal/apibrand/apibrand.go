// Package apibrand declares this server's GDP brand.
//
// With `--brands` (W1) every generated proof type carries a phantom
// brand parameter, so a `TenantAccess[B]` is evidence about the JWT
// chain that minted B. Library code stays generic over B — see
// internal/verified, whose Check* functions are parameterized — but the
// HTTP plumbing cannot be: a principal travels from the middleware to a
// handler through `context.Value`, and recovering it needs a type
// assertion to a concrete type. `any` erases type parameters, so there
// is nothing to assert to unless the brand is fixed somewhere.
//
// API is that somewhere: one brand for the whole server process.
//
// What that buys, and what it does not:
//
//   - It does NOT distinguish two concurrent requests. Both mint at
//     API, so at the type level Alice's proof and Bob's proof are
//     interchangeable. The per-request binding in this example is still
//     carried by the spec's own cross-field premise
//     `(= User (head (head Jwt)))` on `authenticated-user`, plus the
//     tenant threaded out of the TenantAccess proof in
//     verified.CheckResourceAccess.
//   - It DOES stop the empty literal from being anonymous: forging
//     `shenguard.TenantAccess[...]{}` now requires naming a brand in
//     source, and the witness field makes the forged value panic on
//     first read regardless (see bypass_attempts/06_empty_literal).
//   - It DOES keep generic library code honest: the signatures in
//     internal/verified refuse to mix two brands, so a caller that
//     wants per-scope binding (a background job, a test, a second
//     tenant of the same process) declares its own brand and gets the
//     compile-time pairing for free — bypass_attempts/07_unpaired_proof
//     shows the error.
//
// Go has no existential types, so a constructor cannot mint a brand the
// caller is unable to name; brand freshness is a caller discipline.
// ../../docs/TRUST-MODEL.md says so in the trust-boundary section.
package apibrand

// API is the brand of every proof minted inside this server.
type API struct{}
