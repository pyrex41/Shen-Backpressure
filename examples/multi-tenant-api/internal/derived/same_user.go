// Package derived holds implementations whose correctness is checked by
// shen-derive against the Shen spec at specs/core.shen.
//
// This file's sole purpose is to provide a target for the `[[derive.specs]]`
// gate so the discharge report enumerates the structural rules of
// specs/core.shen. The "real" hardening from W2.1 (the structural
// `(= User (head (head Jwt)))` cross-field binding in
// `authenticated-user`) is discharged statically by the shengen
// constructor — see internal/shenguard/guards_gen.go's `NewAuthenticatedUser`.
// shen-derive's role here is to make the static discharge auditor-visible
// via transcript/audit_report.md.
package derived

import "multi-tenant-api/internal/shenguard"

// SameUser reports whether two UserId wrappers carry the same string.
//
// Correctness is checked against the Shen spec
// `(define same-user? {user-id --> user-id --> boolean} A B -> (= A B))`
// in specs/core.shen via shen-derive (see same_user_spec_test.go).
//
// This is the same equality the W2.1 cross-field premise
// `(= User (head (head Jwt))) : verified` inside `authenticated-user`
// asserts at the type level. The spec-equivalence test pins the Go
// `==` semantics on `UserId` against the Shen `=` on `user-id`. A
// refactor that switched to `strings.EqualFold` or a substring check
// would fail the gate.
func SameUser(a, b shenguard.UserId) bool {
	return a == b
}
