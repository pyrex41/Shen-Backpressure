package main

// symtab.go — type classification. A line-for-line port of cmd/shengen's
// SymbolTable.Build so that every emitter agrees on which datatypes are
// wrappers, constrained wrappers, aliases, composites, guarded composites
// and sum types. parity_test.go diffs printSymbolTable against
// `shengen --dry-run` for every spec in the repository.

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
)

type FieldInfo struct {
	Index    int
	ShenName string
	ShenType string
}

type TypeInfo struct {
	ShenName    string
	Category    string // wrapper | constrained | alias | composite | guarded | sumtype
	Fields      []FieldInfo
	WrappedPrim string
	WrappedType string
	Rule        *Rule
	Block       string
}

type SymbolTable struct {
	Types     map[string]*TypeInfo
	ConcCount map[string]int
	SumTypes  map[string][]string
	Defines   map[string]*Define
	Order     []string // emission order of concrete (non-sum) type names
	Namespace string
}

func newSymbolTable(ns string) *SymbolTable {
	return &SymbolTable{
		Types:     map[string]*TypeInfo{},
		ConcCount: map[string]int{},
		SumTypes:  map[string][]string{},
		Defines:   map[string]*Define{},
		Namespace: ns,
	}
}

func isPrimitive(t string) bool {
	return t == "string" || t == "number" || t == "boolean" || t == "symbol"
}

// resolvedName applies shengen's naming rule: a contested conclusion type
// (several blocks conclude it) is named after the block instead.
func (st *SymbolTable) resolvedName(dt Datatype, r Rule) string {
	typeName := r.Conc.TypeName
	if dt.Name != typeName && st.ConcCount[typeName] > 1 {
		typeName = dt.Name
	}
	return typeName
}

func (st *SymbolTable) Build(types []Datatype) {
	for i := range types {
		for _, r := range types[i].Rules {
			st.ConcCount[r.Conc.TypeName]++
		}
	}
	for i := range types {
		dt := &types[i]
		for ri := range dt.Rules {
			r := dt.Rules[ri]
			typeName := r.Conc.TypeName
			if dt.Name != typeName && st.ConcCount[typeName] > 1 {
				typeName = dt.Name
				st.SumTypes[r.Conc.TypeName] = append(st.SumTypes[r.Conc.TypeName], typeName)
			}
			rc := r
			info := &TypeInfo{ShenName: typeName, Rule: &rc, Block: dt.Name}
			isSumVariant := dt.Name != r.Conc.TypeName && st.ConcCount[r.Conc.TypeName] > 1

			switch {
			case r.Conc.IsWrapped && len(r.Verified) == 0 && len(r.Premises) == 1 && isPrimitive(r.Premises[0].TypeName):
				info.Category = "wrapper"
				info.WrappedPrim = r.Premises[0].TypeName
			case r.Conc.IsWrapped && len(r.Verified) > 0 && len(r.Premises) >= 1 && isPrimitive(r.Premises[0].TypeName):
				info.Category = "constrained"
				info.WrappedPrim = r.Premises[0].TypeName
			case r.Conc.IsWrapped && len(r.Premises) == 1 && !isPrimitive(r.Premises[0].TypeName) && !isSumVariant:
				info.Category = "alias"
				info.WrappedType = r.Premises[0].TypeName
			case !r.Conc.IsWrapped && len(r.Verified) > 0:
				info.Category = "guarded"
			default:
				info.Category = "composite"
			}

			if !r.Conc.IsWrapped || isSumVariant {
				premMap := map[string]string{}
				for _, p := range r.Premises {
					premMap[p.VarName] = p.TypeName
				}
				for i, fieldName := range r.Conc.Fields {
					shenType := premMap[fieldName]
					if shenType == "" {
						shenType = "unknown"
					}
					info.Fields = append(info.Fields, FieldInfo{Index: i, ShenName: fieldName, ShenType: shenType})
				}
			}
			if _, dup := st.Types[typeName]; !dup {
				st.Order = append(st.Order, typeName)
			}
			st.Types[typeName] = info
		}
	}
	for concType := range st.SumTypes {
		if _, exists := st.Types[concType]; !exists {
			st.Types[concType] = &TypeInfo{ShenName: concType, Category: "sumtype"}
		}
	}
}

func (st *SymbolTable) Lookup(name string) *TypeInfo { return st.Types[name] }

// IsWrapper reports wrapper/constrained types (single `val` over a primitive).
func (st *SymbolTable) IsWrapper(t string) bool {
	info := st.Lookup(t)
	return info != nil && (info.Category == "wrapper" || info.Category == "constrained")
}

// IsStruct reports whether values of Shen type t are generated structs.
func (st *SymbolTable) IsStruct(t string) bool {
	info := st.Lookup(t)
	return info != nil && info.Category != "sumtype"
}

// SumTypeNames returns the sum-type conclusions in deterministic order.
func (st *SymbolTable) SumTypeNames() []string {
	var out []string
	for k := range st.SumTypes {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// printSymbolTable writes the same dump shengen --dry-run prints, byte for
// byte, so parity can be asserted mechanically.
func printSymbolTable(w io.Writer, types []Datatype, st *SymbolTable, path string) {
	fmt.Fprintf(w, "Parsed %d datatypes from %s\n\n", len(types), path)
	fmt.Fprintf(w, "Symbol table:\n")
	for _, dt := range types {
		for _, r := range dt.Rules {
			typeName := st.resolvedName(dt, r)
			info := st.Lookup(typeName)
			if info == nil {
				continue
			}
			label := typeName
			if dt.Name != typeName {
				label = fmt.Sprintf("%s (block: %s)", typeName, dt.Name)
			}
			fmt.Fprintf(w, "  %-28s [%-11s]", label, info.Category)
			if len(info.Fields) > 0 {
				names := make([]string, len(info.Fields))
				for i, f := range info.Fields {
					names[i] = fmt.Sprintf("%s:%s", f.ShenName, f.ShenType)
				}
				fmt.Fprintf(w, " {%s}", strings.Join(names, ", "))
			}
			if info.WrappedPrim != "" {
				fmt.Fprintf(w, " wraps=%s", info.WrappedPrim)
			}
			if info.WrappedType != "" {
				fmt.Fprintf(w, " alias=%s", info.WrappedType)
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintln(w)
}

// ----------------------------------------------------------------------------
// Naming
// ----------------------------------------------------------------------------

func toPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	var b strings.Builder
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		runes := []rune(p)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	return b.String()
}

var elixirReserved = map[string]bool{
	"do": true, "end": true, "fn": true, "when": true, "nil": true, "true": true,
	"false": true, "in": true, "not": true, "and": true, "or": true, "after": true,
	"catch": true, "else": true, "rescue": true, "cond": true, "case": true,
	"if": true, "unless": true, "receive": true, "try": true, "with": true,
	"for": true, "quote": true, "unquote": true, "import": true, "require": true,
	"alias": true, "super": true, "__MODULE__": true, "__ENV__": true,
}

// toSnake converts a Shen variable (IsMember) or type (tenant-id) into an
// Elixir snake_case identifier, escaping reserved words.
func toSnake(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i, r := range rs {
		switch {
		case r == '-' || r == '.' || r == '/':
			b.WriteRune('_')
		case unicode.IsUpper(r):
			if i > 0 && rs[i-1] != '-' && rs[i-1] != '_' && (unicode.IsLower(rs[i-1]) || unicode.IsDigit(rs[i-1]) ||
				(i+1 < len(rs) && unicode.IsLower(rs[i+1]) && unicode.IsUpper(rs[i-1]))) {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	name := b.String()
	if name == "" || unicode.IsDigit([]rune(name)[0]) {
		name = "v_" + name
	}
	if elixirReserved[name] {
		name += "_"
	}
	return name
}

// fnName converts a Shen function symbol into an Elixir function name:
// member-of? -> member_of?, string->n -> string_to_n.
func fnName(s string) string {
	suffix := ""
	if strings.HasSuffix(s, "?") || strings.HasSuffix(s, "!") {
		suffix = s[len(s)-1:]
		s = s[:len(s)-1]
	}
	s = strings.ReplaceAll(s, "->", "_to_")
	var b strings.Builder
	rs := []rune(s)
	for i, r := range rs {
		switch {
		case unicode.IsUpper(r):
			if i > 0 && (unicode.IsLower(rs[i-1]) || unicode.IsDigit(rs[i-1])) {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	if name == "" || unicode.IsDigit([]rune(name)[0]) {
		name = "f_" + name
	}
	if elixirReserved[name] && suffix == "" {
		name += "_"
	}
	return name + suffix
}

// moduleFor returns the fully qualified Elixir module for a Shen type.
func (st *SymbolTable) moduleFor(shenType string) string {
	return st.Namespace + "." + toPascalCase(shenType)
}

// elixirLiteralString renders s as an Elixir double-quoted string literal.
func elixirLiteralString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '#':
			b.WriteString(`\#`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// elixirAtom renders a Shen symbol as an Elixir atom literal.
func elixirAtom(sym string) string {
	simple := sym != ""
	for i, r := range sym {
		if !(unicode.IsLower(r) || (i > 0 && (unicode.IsDigit(r) || r == '_' || unicode.IsUpper(r)))) {
			simple = false
			break
		}
	}
	if simple && !elixirReserved[sym] {
		return ":" + sym
	}
	return ":" + elixirLiteralString(sym)
}
