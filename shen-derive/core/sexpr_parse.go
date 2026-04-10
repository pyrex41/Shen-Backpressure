package core

import (
	"fmt"
	"strings"
	"unicode"
)

// ParseSexpr parses a single s-expression from input.
func ParseSexpr(input string) (Sexpr, error) {
	p := &sexprParser{input: input}
	p.skipWhitespace()
	expr, err := p.parse()
	if err != nil {
		return nil, err
	}
	p.skipWhitespace()
	if p.pos < len(p.input) {
		return nil, fmt.Errorf("unexpected input after expression at position %d", p.pos)
	}
	return expr, nil
}

// ParseAllSexprs parses all top-level s-expressions from input.
func ParseAllSexprs(input string) ([]Sexpr, error) {
	p := &sexprParser{input: input}
	var result []Sexpr
	for {
		p.skipWhitespace()
		if p.pos >= len(p.input) {
			break
		}
		expr, err := p.parse()
		if err != nil {
			return nil, err
		}
		result = append(result, expr)
	}
	return result, nil
}

type sexprParser struct {
	input string
	pos   int
}

func (p *sexprParser) parse() (Sexpr, error) {
	p.skipWhitespace()
	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("unexpected end of input")
	}

	ch := p.input[p.pos]

	switch {
	case ch == '(':
		return p.parseList()
	case ch == '[':
		return p.parseSquareList()
	case ch == '"':
		return p.parseString()
	case ch == ')' || ch == ']':
		return nil, fmt.Errorf("unexpected '%c' at position %d", ch, p.pos)
	default:
		return p.parseAtom()
	}
}

// parseList parses (elem1 elem2 ...)
func (p *sexprParser) parseList() (Sexpr, error) {
	p.pos++ // consume '('
	var elems []Sexpr

	for {
		p.skipWhitespace()
		if p.pos >= len(p.input) {
			return nil, fmt.Errorf("unclosed '(' — expected ')'")
		}
		if p.input[p.pos] == ')' {
			p.pos++ // consume ')'
			return &List{Elems: elems}, nil
		}
		elem, err := p.parse()
		if err != nil {
			return nil, err
		}
		elems = append(elems, elem)
	}
}

// parseSquareList parses [e1 e2 ... | tail] or [e1 e2 ...].
// [a b c] desugars to (cons a (cons b (cons c nil)))
// [a b | c] desugars to (cons a (cons b c))
func (p *sexprParser) parseSquareList() (Sexpr, error) {
	p.pos++ // consume '['
	var elems []Sexpr
	var tail Sexpr

	for {
		p.skipWhitespace()
		if p.pos >= len(p.input) {
			return nil, fmt.Errorf("unclosed '[' — expected ']'")
		}
		if p.input[p.pos] == ']' {
			p.pos++ // consume ']'
			break
		}
		// Check for | (cons tail)
		if p.input[p.pos] == '|' {
			p.pos++ // consume '|'
			p.skipWhitespace()
			var err error
			tail, err = p.parse()
			if err != nil {
				return nil, err
			}
			p.skipWhitespace()
			if p.pos >= len(p.input) || p.input[p.pos] != ']' {
				return nil, fmt.Errorf("expected ']' after tail in cons notation")
			}
			p.pos++ // consume ']'
			break
		}

		elem, err := p.parse()
		if err != nil {
			return nil, err
		}
		elems = append(elems, elem)
	}

	// Build cons chain from right
	if tail == nil {
		tail = Sym("nil")
	}
	result := tail
	for i := len(elems) - 1; i >= 0; i-- {
		result = SList(Sym("cons"), elems[i], result)
	}
	return result, nil
}

// parseString parses "..." with escape handling.
func (p *sexprParser) parseString() (Sexpr, error) {
	p.pos++ // consume opening '"'
	var b strings.Builder

	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == '"' {
			p.pos++ // consume closing '"'
			return Str(b.String()), nil
		}
		if ch == '\\' && p.pos+1 < len(p.input) {
			p.pos++
			switch p.input[p.pos] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			default:
				b.WriteByte('\\')
				b.WriteByte(p.input[p.pos])
			}
			p.pos++
			continue
		}
		b.WriteByte(ch)
		p.pos++
	}
	return nil, fmt.Errorf("unclosed string literal")
}

// parseAtom parses a symbol, number, or boolean.
func (p *sexprParser) parseAtom() (Sexpr, error) {
	start := p.pos

	// Read until whitespace or delimiter
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == '(' || ch == ')' || ch == '[' || ch == ']' || ch == '"' || isWhitespace(ch) {
			break
		}
		p.pos++
	}

	if p.pos == start {
		return nil, fmt.Errorf("expected atom at position %d", p.pos)
	}

	tok := p.input[start:p.pos]

	// Boolean
	if tok == "true" {
		return Bool(true), nil
	}
	if tok == "false" {
		return Bool(false), nil
	}

	// Try integer
	if isIntLiteral(tok) {
		return &Atom{Val: tok, Kind: AtomInt}, nil
	}

	// Try float
	if isFloatLiteral(tok) {
		return &Atom{Val: tok, Kind: AtomFloat}, nil
	}

	// Symbol (including operators like +, -, *, >=, etc.)
	return Sym(tok), nil
}

func (p *sexprParser) skipWhitespace() {
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if isWhitespace(ch) {
			p.pos++
			continue
		}
		// Block comment: \* ... *\ (Shen style)
		if ch == '\\' && p.pos+1 < len(p.input) && p.input[p.pos+1] == '*' {
			p.pos += 2
			for p.pos+1 < len(p.input) {
				if p.input[p.pos] == '*' && p.input[p.pos+1] == '\\' {
					p.pos += 2
					break
				}
				p.pos++
			}
			continue
		}
		// Line comment: \\ ... (Shen style)
		if ch == '\\' && p.pos+1 < len(p.input) && p.input[p.pos+1] == '\\' {
			p.pos += 2
			for p.pos < len(p.input) && p.input[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		// Also handle -- comments (from shen-derive v1 syntax)
		if ch == '-' && p.pos+1 < len(p.input) && p.input[p.pos+1] == '-' {
			p.pos += 2
			for p.pos < len(p.input) && p.input[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

func isWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isIntLiteral(s string) bool {
	if len(s) == 0 {
		return false
	}
	start := 0
	if s[0] == '-' || s[0] == '+' {
		if len(s) == 1 {
			return false // bare + or - is a symbol
		}
		start = 1
	}
	for i := start; i < len(s); i++ {
		if !unicode.IsDigit(rune(s[i])) {
			return false
		}
	}
	return true
}

func isFloatLiteral(s string) bool {
	if len(s) == 0 {
		return false
	}
	hasDot := false
	start := 0
	if s[0] == '-' || s[0] == '+' {
		if len(s) == 1 {
			return false
		}
		start = 1
	}
	for i := start; i < len(s); i++ {
		if s[i] == '.' {
			if hasDot {
				return false
			}
			hasDot = true
			continue
		}
		if !unicode.IsDigit(rune(s[i])) {
			return false
		}
	}
	return hasDot // must have at least one dot to be float, not int
}
