package flow

import (
	"fmt"
	"strings"
)

// Premise is one clause of a (flow …) form.
type Premise interface {
	// ID is the stable premise identifier the discharge report uses.
	ID() string
	// Expression is the clause as written in the spec.
	Expression() string
}

// ConstructorOnly is `(constructor-only <ctor> <allowed-caller>…)`:
// every reference to <ctor> must lie inside a definition matching one
// of the allowed callers. It is the flow premise that replaces a grep
// gate over constructor names — and unlike the grep it sees through
// an aliased import, because the indexer resolved the alias.
type ConstructorOnly struct {
	Ctor    Pattern
	Allowed []Pattern
	raw     string
}

func (c ConstructorOnly) ID() string         { return "constructor-only:" + c.Ctor.String() }
func (c ConstructorOnly) Expression() string { return c.raw }

// MustPassThrough is `(must-pass-through <source> <proof> <sink>)`:
// on every call path from a definition matching <source> to a call of
// <sink>, some definition on the path must reference <proof>. With
// the guard constructor as the declassifier, this is the
// noninterference statement "no handler reaches the database without
// obtaining a proof first".
type MustPassThrough struct {
	Source Pattern
	Proof  Pattern
	Sink   Pattern
	raw    string
}

func (m MustPassThrough) ID() string {
	return "must-pass-through:" + m.Source.String() + "→" + m.Sink.String()
}
func (m MustPassThrough) Expression() string { return m.raw }

// Decl is a whole (flow <name> <clause>…) form.
type Decl struct {
	Name     string
	Premises []Premise
	// Raw is the form as it appears in the spec, for the report's
	// spec_excerpt.
	Raw string
	// SpecFile is the file the form was read from.
	SpecFile string
}

// ParseSpec extracts every (flow …) form from a .shen spec file's
// text.
//
// The forms are found textually, including inside Shen `\* … *\`
// comments. That is deliberate: `flow` is not a Shen function, so a
// bare top-level form would make `shen tc+` complain about an
// undefined symbol. Keeping the forms inside a comment block leaves
// gate 4 (shen-check) untouched while sb still reads them, and both
// spellings parse identically here. See docs/FLOW.md.
func ParseSpec(path, content string) ([]Decl, error) {
	var out []Decl
	for _, form := range extractFlowForms(content) {
		d, err := parseFlowForm(form)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		d.SpecFile = path
		out = append(out, d)
	}
	return out, nil
}

// extractFlowForms returns each balanced `(flow …)` form in content.
func extractFlowForms(content string) []string {
	var out []string
	for i := 0; i+6 <= len(content); i++ {
		if !strings.HasPrefix(content[i:], "(flow ") {
			continue
		}
		depth := 0
		inString := false
		for j := i; j < len(content); j++ {
			switch content[j] {
			case '"':
				inString = !inString
			case '(':
				if !inString {
					depth++
				}
			case ')':
				if inString {
					continue
				}
				depth--
				if depth == 0 {
					out = append(out, content[i:j+1])
					i = j
					j = len(content)
				}
			}
		}
	}
	return out
}

// parseFlowForm parses one form into a Decl.
func parseFlowForm(form string) (Decl, error) {
	inner := strings.TrimSpace(form)
	inner = strings.TrimPrefix(inner, "(flow")
	inner = strings.TrimSuffix(strings.TrimSpace(inner), ")")
	name, rest := firstWord(inner)
	if name == "" {
		return Decl{}, fmt.Errorf("flow: form has no name: %s", form)
	}
	d := Decl{Name: name, Raw: form}
	for _, clause := range splitForms(rest) {
		toks, err := tokenizeForm(clause)
		if err != nil {
			return Decl{}, err
		}
		switch toks[0] {
		case "constructor-only":
			if len(toks) < 3 {
				return Decl{}, fmt.Errorf("flow %s: (constructor-only …) needs a constructor and at least one allowed caller", name)
			}
			p := ConstructorOnly{Ctor: MustPattern(toks[1]), raw: clause}
			for _, a := range toks[2:] {
				p.Allowed = append(p.Allowed, MustPattern(a))
			}
			d.Premises = append(d.Premises, p)
		case "must-pass-through":
			if len(toks) != 4 {
				return Decl{}, fmt.Errorf("flow %s: (must-pass-through …) needs exactly a source, a proof and a sink", name)
			}
			d.Premises = append(d.Premises, MustPassThrough{
				Source: MustPattern(toks[1]),
				Proof:  MustPattern(toks[2]),
				Sink:   MustPattern(toks[3]),
				raw:    clause,
			})
		default:
			return Decl{}, fmt.Errorf("flow %s: unknown clause %q", name, toks[0])
		}
	}
	if len(d.Premises) == 0 {
		return Decl{}, fmt.Errorf("flow %s: no clauses", name)
	}
	return d, nil
}

func firstWord(s string) (word, rest string) {
	s = strings.TrimLeft(s, " \t\n\r")
	i := strings.IndexAny(s, " \t\n\r(")
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i:]
}
