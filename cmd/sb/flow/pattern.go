package flow

import (
	"regexp"
	"strings"
	"sync"
)

// Canonical reduces a SCIP symbol to the readable path a spec author
// writes in a (flow …) premise.
//
// A SCIP symbol is `<scheme> <manager> <package> <version> <descriptors>`,
// where any of the first four fields may be backtick-quoted, and the
// descriptor suffix uses `/` for packages and namespaces, `#` for
// types, and `().` for methods. Canonical drops the four header
// fields (they carry a version hash, which would make every pattern
// commit-specific), strips backticks, and drops the trailing
// descriptor punctuation:
//
//	scip-go gomod multi-tenant-api abc123 `multi-tenant-api/internal/verified`/CheckTenantAccess().
//	→ multi-tenant-api/internal/verified/CheckTenantAccess
//
//	scip-typescript npm shen-web-tools 0.0.0 `src/guards.ts`/newTenantAccess().
//	→ src/guards.ts/newTenantAccess
//
// Local symbols ("local 3") and the empty symbol canonicalise to
// themselves, and never match a pattern with a path separator.
func Canonical(symbol string) string {
	desc := descriptorPart(symbol)
	desc = strings.ReplaceAll(desc, "`", "")
	desc = strings.TrimSuffix(desc, "().")
	desc = strings.TrimRight(desc, "./#")
	return desc
}

// descriptorPart returns everything after the four header fields,
// honouring backtick quoting so a package name containing a space
// does not shift the field count.
func descriptorPart(symbol string) string {
	fields := 0
	inTick := false
	for i := 0; i < len(symbol); i++ {
		switch symbol[i] {
		case '`':
			inTick = !inTick
		case ' ':
			if inTick {
				continue
			}
			fields++
			if fields == 4 {
				return symbol[i+1:]
			}
		}
	}
	// Not a full SCIP symbol (a local symbol, or already canonical).
	return symbol
}

// IsCallable reports whether the symbol names something invocable, by
// the SCIP method descriptor suffix `().`. This is the one piece of
// the vocabulary that is syntactic rather than semantic, and it holds
// for every SCIP indexer because the descriptor grammar is part of
// the protocol, not of a language.
func IsCallable(symbol string) bool {
	return strings.HasSuffix(symbol, "().")
}

// Pattern matches symbols by their canonical form.
//
// A pattern matches a canonical symbol when it equals it, or when it
// matches a suffix of it that begins at a segment boundary (`/` or
// `#`). So `internal/verified/CheckTenantAccess` matches
// `multi-tenant-api/internal/verified/CheckTenantAccess`, and
// `DB#Query` matches `database/sql/DB#Query`, without the spec author
// having to know the module path or the indexer's version hash.
//
// `*` in a pattern matches any run of characters, including
// separators, which is how a source pattern names a family of
// handlers: `internal/handlers/Server#handle*`.
type Pattern struct {
	raw string
	re  *regexp.Regexp
}

var patternCache sync.Map // string -> *regexp.Regexp

// MustPattern compiles a pattern. The grammar has no way to fail (any
// string is a valid pattern), so there is no error return.
func MustPattern(raw string) Pattern {
	if cached, ok := patternCache.Load(raw); ok {
		return Pattern{raw: raw, re: cached.(*regexp.Regexp)}
	}
	parts := strings.Split(raw, "*")
	for i, part := range parts {
		parts[i] = regexp.QuoteMeta(part)
	}
	// An empty leading or trailing part (from a leading or trailing
	// `*`) contributes nothing but the `.*` that joins it, which is
	// exactly the wanted meaning.
	re := regexp.MustCompile(`^(?:.*[/#])?` + strings.Join(parts, `.*`) + `$`)
	patternCache.Store(raw, re)
	return Pattern{raw: raw, re: re}
}

// String returns the pattern as written in the spec.
func (p Pattern) String() string { return p.raw }

// Matches reports whether the SCIP symbol matches the pattern.
func (p Pattern) Matches(symbol string) bool {
	if p.re == nil {
		return false
	}
	return p.re.MatchString(Canonical(symbol))
}

// MatchesAny reports whether any pattern matches the symbol.
func MatchesAny(pats []Pattern, symbol string) bool {
	for _, p := range pats {
		if p.Matches(symbol) {
			return true
		}
	}
	return false
}
