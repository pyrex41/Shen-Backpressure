package specfile

import (
	"strings"
	"unicode"
)

// TypeCategory classifies how a Shen datatype maps to Go.
// Mirrors shengen/main.go classification at lines 161-177.
type TypeCategory string

const (
	CatWrapper     TypeCategory = "wrapper"     // single-field, no verification: struct{v T} + NewX(x) X
	CatConstrained TypeCategory = "constrained" // single-field, verified premises: NewX(x) (X, error)
	CatComposite   TypeCategory = "composite"   // multi-field, no verification
	CatGuarded     TypeCategory = "guarded"     // multi-field, verified
	CatAlias       TypeCategory = "alias"       // type X = Y
	CatSumType     TypeCategory = "sumtype"     // union produced by multiple datatypes
)

// TypeEntry describes the Go shape of one Shen type.
type TypeEntry struct {
	ShenName    string
	GoName      string // "Amount" (unqualified)
	GoQualified string // "shenguard.Amount" (with import alias)
	Category    TypeCategory

	// For wrapper/constrained:
	ShenPrim   string // "number", "string", "boolean", "symbol"
	GoPrimType string // "float64", "string", "bool"
	VarName    string // the premise variable name (e.g. "X"), used when evaluating Verified

	// For constrained and guarded: raw verified s-expressions, e.g. "(>= X 0)".
	Verified []string

	// For composite/guarded:
	Fields []Field
}

// Field is one field of a composite Shen datatype.
type Field struct {
	Index    int
	ShenName string // "Amount" (as written in the spec)
	ShenType string // "amount"
	GoMethod string // "Amount" (the Go accessor method name)
}

// TypeTable maps Shen type names to their Go equivalents.
type TypeTable struct {
	Entries     map[string]*TypeEntry
	ImportPath  string // e.g., "example.com/project/internal/shenguard"
	ImportAlias string // e.g., "shenguard"
}

// BuildTypeTable classifies every datatype and builds a type table.
// importPath is the Go import path of the shengen-generated package;
// importAlias is the short name used to qualify types (e.g., "shenguard").
func BuildTypeTable(datatypes []Datatype, importPath, importAlias string) *TypeTable {
	tt := &TypeTable{
		Entries:     make(map[string]*TypeEntry),
		ImportPath:  importPath,
		ImportAlias: importAlias,
	}

	// Count how many datatype blocks produce each conclusion type.
	// Multiple → sum type variants.
	concCount := make(map[string]int)
	for _, dt := range datatypes {
		for _, r := range dt.Rules {
			concCount[r.Conclusion.TypeName]++
		}
	}

	for _, dt := range datatypes {
		for _, r := range dt.Rules {
			typeName := r.Conclusion.TypeName
			// Sum variants use the block name; the union type itself is registered below.
			isSumVariant := dt.Name != r.Conclusion.TypeName && concCount[r.Conclusion.TypeName] > 1
			if isSumVariant {
				typeName = dt.Name
			}

			entry := &TypeEntry{
				ShenName: typeName,
				GoName:   toPascalCase(typeName),
			}
			entry.GoQualified = qualify(importAlias, entry.GoName)

			switch {
			case r.Conclusion.IsWrapped && len(r.Verified) == 0 && len(r.Premises) == 1 && isPrimitive(r.Premises[0].TypeName):
				entry.Category = CatWrapper
				entry.ShenPrim = r.Premises[0].TypeName
				entry.GoPrimType = shenPrimToGo(entry.ShenPrim)
				entry.VarName = r.Premises[0].VarName

			case r.Conclusion.IsWrapped && len(r.Verified) > 0 && len(r.Premises) >= 1 && isPrimitive(r.Premises[0].TypeName):
				entry.Category = CatConstrained
				entry.ShenPrim = r.Premises[0].TypeName
				entry.GoPrimType = shenPrimToGo(entry.ShenPrim)
				entry.VarName = r.Premises[0].VarName

			case r.Conclusion.IsWrapped && len(r.Premises) == 1 && !isPrimitive(r.Premises[0].TypeName) && !isSumVariant:
				entry.Category = CatAlias

			case !r.Conclusion.IsWrapped && len(r.Verified) > 0:
				entry.Category = CatGuarded

			default:
				entry.Category = CatComposite
			}

			// Record verified predicates on entries that have constraints.
			if entry.Category == CatConstrained || entry.Category == CatGuarded {
				for _, v := range r.Verified {
					entry.Verified = append(entry.Verified, v.Raw)
				}
			}

			// Fields for composites (and sum variants with wrapped conclusions,
			// for symmetry with shengen).
			if !r.Conclusion.IsWrapped || isSumVariant {
				premMap := map[string]string{}
				for _, p := range r.Premises {
					premMap[p.VarName] = p.TypeName
				}
				for i, fieldName := range r.Conclusion.Fields {
					shenType := premMap[fieldName]
					if shenType == "" {
						shenType = "unknown"
					}
					entry.Fields = append(entry.Fields, Field{
						Index:    i,
						ShenName: fieldName,
						ShenType: shenType,
						GoMethod: toPascalCase(fieldName),
					})
				}
			}

			tt.Entries[typeName] = entry
		}
	}

	// Register synthetic sum type entries for union conclusions.
	for _, dt := range datatypes {
		for _, r := range dt.Rules {
			conc := r.Conclusion.TypeName
			if concCount[conc] > 1 {
				if _, ok := tt.Entries[conc]; !ok {
					tt.Entries[conc] = &TypeEntry{
						ShenName:    conc,
						GoName:      toPascalCase(conc),
						GoQualified: qualify(importAlias, toPascalCase(conc)),
						Category:    CatSumType,
					}
				}
			}
		}
	}

	return tt
}

// GoType returns the Go type expression for a Shen type.
// Handles "(list T)" by recursing on the element type.
// Primitive types map to Go built-ins; declared types are qualified with the import alias.
func (tt *TypeTable) GoType(shenType string) string {
	shenType = strings.TrimSpace(shenType)
	if strings.HasPrefix(shenType, "(list ") && strings.HasSuffix(shenType, ")") {
		inner := strings.TrimSpace(shenType[len("(list ") : len(shenType)-1])
		return "[]" + tt.GoType(inner)
	}
	if isPrimitive(shenType) {
		return shenPrimToGo(shenType)
	}
	if e, ok := tt.Entries[shenType]; ok {
		return e.GoQualified
	}
	// Unknown — fall back to Pascal case, no qualification.
	return toPascalCase(shenType)
}

// ElemType extracts the element type from "(list T)" or returns "".
func ElemType(shenType string) string {
	shenType = strings.TrimSpace(shenType)
	if strings.HasPrefix(shenType, "(list ") && strings.HasSuffix(shenType, ")") {
		return strings.TrimSpace(shenType[len("(list ") : len(shenType)-1])
	}
	return ""
}

// --- Helpers ---

func isPrimitive(t string) bool {
	return t == "string" || t == "number" || t == "boolean" || t == "symbol"
}

func shenPrimToGo(t string) string {
	switch t {
	case "number":
		return "float64"
	case "string", "symbol":
		return "string"
	case "boolean":
		return "bool"
	default:
		return "any"
	}
}

func toPascalCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' })
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

func qualify(alias, name string) string {
	if alias == "" {
		return name
	}
	return alias + "." + name
}
