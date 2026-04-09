package core

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// --- Lexer ---

type TokenType int

const (
	TokenEOF TokenType = iota
	TokenInt
	TokenString
	TokenIdent
	// Operators
	TokenPlus    // +
	TokenMinus   // -
	TokenStar    // *
	TokenSlash   // /
	TokenPercent // %
	TokenEqEq    // ==
	TokenNeq     // /=
	TokenLt      // <
	TokenLe      // <=
	TokenGt      // >
	TokenGe      // >=
	TokenAmpAmp  // &&
	TokenPipePipe // ||
	TokenDot     // .
	TokenColon   // :
	// Delimiters
	TokenLParen    // (
	TokenRParen    // )
	TokenLBrack    // [
	TokenRBrack    // ]
	TokenBackslash // \.
	TokenArrow     // ->
	TokenEq        // =
	TokenComma     // ,
	// Keywords
	TokenLet
	TokenIn
	TokenIf
	TokenThen
	TokenElse
	TokenTrue
	TokenFalse
)

type Token struct {
	Type   TokenType
	Text   string
	IntVal int64
	StrVal string
	Pos    Position
}

func (t Token) String() string {
	if t.Type == TokenEOF {
		return "EOF"
	}
	return t.Text
}

type Lexer struct {
	input []rune
	pos   int
	line  int
	col   int
}

func NewLexer(input string) *Lexer {
	return &Lexer{input: []rune(input), pos: 0, line: 1, col: 1}
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

func (l *Lexer) peekN(n int) rune {
	if l.pos+n >= len(l.input) {
		return 0
	}
	return l.input[l.pos+n]
}

func (l *Lexer) advance() rune {
	ch := l.input[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) curPos() Position {
	return Position{Line: l.line, Col: l.col}
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == '-' && l.pos+1 < len(l.input) && l.input[l.pos+1] == '-' {
			// Line comment
			for l.pos < len(l.input) && l.input[l.pos] != '\n' {
				l.advance()
			}
			continue
		}
		if unicode.IsSpace(ch) {
			l.advance()
			continue
		}
		break
	}
}

func (l *Lexer) Tokenize() ([]Token, error) {
	var tokens []Token
	for {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			tokens = append(tokens, Token{Type: TokenEOF, Pos: l.curPos()})
			return tokens, nil
		}

		pos := l.curPos()
		ch := l.peek()

		// Numbers
		if unicode.IsDigit(ch) {
			tok, err := l.lexNumber(pos)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			continue
		}

		// Strings
		if ch == '"' {
			tok, err := l.lexString(pos)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			continue
		}

		// Identifiers and keywords
		if unicode.IsLetter(ch) || ch == '_' {
			tok := l.lexIdent(pos)
			tokens = append(tokens, tok)
			continue
		}

		// Two-char operators first
		if l.pos+1 < len(l.input) {
			two := string(l.input[l.pos : l.pos+2])
			switch two {
			case "==":
				l.advance()
				l.advance()
				tokens = append(tokens, Token{Type: TokenEqEq, Text: "==", Pos: pos})
				continue
			case "/=":
				l.advance()
				l.advance()
				tokens = append(tokens, Token{Type: TokenNeq, Text: "/=", Pos: pos})
				continue
			case "<=":
				l.advance()
				l.advance()
				tokens = append(tokens, Token{Type: TokenLe, Text: "<=", Pos: pos})
				continue
			case ">=":
				l.advance()
				l.advance()
				tokens = append(tokens, Token{Type: TokenGe, Text: ">=", Pos: pos})
				continue
			case "&&":
				l.advance()
				l.advance()
				tokens = append(tokens, Token{Type: TokenAmpAmp, Text: "&&", Pos: pos})
				continue
			case "||":
				l.advance()
				l.advance()
				tokens = append(tokens, Token{Type: TokenPipePipe, Text: "||", Pos: pos})
				continue
			case "->":
				l.advance()
				l.advance()
				tokens = append(tokens, Token{Type: TokenArrow, Text: "->", Pos: pos})
				continue
			}
		}

		// Single-char operators and delimiters
		l.advance()
		switch ch {
		case '+':
			tokens = append(tokens, Token{Type: TokenPlus, Text: "+", Pos: pos})
		case '-':
			tokens = append(tokens, Token{Type: TokenMinus, Text: "-", Pos: pos})
		case '*':
			tokens = append(tokens, Token{Type: TokenStar, Text: "*", Pos: pos})
		case '/':
			tokens = append(tokens, Token{Type: TokenSlash, Text: "/", Pos: pos})
		case '%':
			tokens = append(tokens, Token{Type: TokenPercent, Text: "%", Pos: pos})
		case '<':
			tokens = append(tokens, Token{Type: TokenLt, Text: "<", Pos: pos})
		case '>':
			tokens = append(tokens, Token{Type: TokenGt, Text: ">", Pos: pos})
		case '.':
			tokens = append(tokens, Token{Type: TokenDot, Text: ".", Pos: pos})
		case ':':
			tokens = append(tokens, Token{Type: TokenColon, Text: ":", Pos: pos})
		case '(':
			tokens = append(tokens, Token{Type: TokenLParen, Text: "(", Pos: pos})
		case ')':
			tokens = append(tokens, Token{Type: TokenRParen, Text: ")", Pos: pos})
		case '[':
			tokens = append(tokens, Token{Type: TokenLBrack, Text: "[", Pos: pos})
		case ']':
			tokens = append(tokens, Token{Type: TokenRBrack, Text: "]", Pos: pos})
		case '\\':
			tokens = append(tokens, Token{Type: TokenBackslash, Text: "\\", Pos: pos})
		case '=':
			tokens = append(tokens, Token{Type: TokenEq, Text: "=", Pos: pos})
		case ',':
			tokens = append(tokens, Token{Type: TokenComma, Text: ",", Pos: pos})
		default:
			return nil, fmt.Errorf("%s: unexpected character %q", pos, ch)
		}
	}
}

func (l *Lexer) lexNumber(pos Position) (Token, error) {
	start := l.pos
	for l.pos < len(l.input) && unicode.IsDigit(l.input[l.pos]) {
		l.advance()
	}
	text := string(l.input[start:l.pos])
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return Token{}, fmt.Errorf("%s: invalid integer %q", pos, text)
	}
	return Token{Type: TokenInt, Text: text, IntVal: n, Pos: pos}, nil
}

func (l *Lexer) lexString(pos Position) (Token, error) {
	l.advance() // opening "
	var buf strings.Builder
	for l.pos < len(l.input) {
		ch := l.advance()
		if ch == '"' {
			return Token{Type: TokenString, Text: buf.String(), StrVal: buf.String(), Pos: pos}, nil
		}
		if ch == '\\' {
			if l.pos >= len(l.input) {
				return Token{}, fmt.Errorf("%s: unterminated string escape", pos)
			}
			esc := l.advance()
			switch esc {
			case 'n':
				buf.WriteRune('\n')
			case 't':
				buf.WriteRune('\t')
			case '\\':
				buf.WriteRune('\\')
			case '"':
				buf.WriteRune('"')
			default:
				buf.WriteRune('\\')
				buf.WriteRune(esc)
			}
		} else {
			buf.WriteRune(ch)
		}
	}
	return Token{}, fmt.Errorf("%s: unterminated string", pos)
}

func (l *Lexer) lexIdent(pos Position) Token {
	start := l.pos
	for l.pos < len(l.input) && (unicode.IsLetter(l.input[l.pos]) || unicode.IsDigit(l.input[l.pos]) || l.input[l.pos] == '_' || l.input[l.pos] == '\'') {
		l.advance()
	}
	text := string(l.input[start:l.pos])

	switch text {
	case "let":
		return Token{Type: TokenLet, Text: text, Pos: pos}
	case "in":
		return Token{Type: TokenIn, Text: text, Pos: pos}
	case "if":
		return Token{Type: TokenIf, Text: text, Pos: pos}
	case "then":
		return Token{Type: TokenThen, Text: text, Pos: pos}
	case "else":
		return Token{Type: TokenElse, Text: text, Pos: pos}
	case "True":
		return Token{Type: TokenTrue, Text: text, Pos: pos}
	case "False":
		return Token{Type: TokenFalse, Text: text, Pos: pos}
	default:
		return Token{Type: TokenIdent, Text: text, Pos: pos}
	}
}

// --- Parser ---

// Parser is a recursive-descent parser for the derivation core surface syntax.
type Parser struct {
	tokens []Token
	pos    int
}

func NewParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens, pos: 0}
}

// Parse parses a complete expression from a string.
func Parse(input string) (Term, error) {
	lex := NewLexer(input)
	tokens, err := lex.Tokenize()
	if err != nil {
		return nil, err
	}
	p := NewParser(tokens)
	t, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.peek().Type != TokenEOF {
		return nil, fmt.Errorf("%s: unexpected token %q after expression", p.peek().Pos, p.peek().Text)
	}
	return t, nil
}

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *Parser) expect(tt TokenType) (Token, error) {
	t := p.peek()
	if t.Type != tt {
		return Token{}, fmt.Errorf("%s: expected %s, got %q", t.Pos, tokenName(tt), t.Text)
	}
	return p.advance(), nil
}

func tokenName(tt TokenType) string {
	switch tt {
	case TokenEOF:
		return "end of input"
	case TokenArrow:
		return "'->' "
	case TokenEq:
		return "'='"
	case TokenIn:
		return "'in'"
	case TokenThen:
		return "'then'"
	case TokenElse:
		return "'else'"
	case TokenRParen:
		return "')'"
	case TokenRBrack:
		return "']'"
	case TokenComma:
		return "','"
	case TokenColon:
		return "':'"
	default:
		return fmt.Sprintf("token type %d", tt)
	}
}

// parseExpr = letExpr | lamExpr | ifExpr | orExpr
func (p *Parser) parseExpr() (Term, error) {
	switch p.peek().Type {
	case TokenLet:
		return p.parseLet()
	case TokenBackslash:
		return p.parseLam()
	case TokenIf:
		return p.parseIf()
	default:
		return p.parseOr()
	}
}

// parseLet: "let" IDENT "=" expr "in" expr
func (p *Parser) parseLet() (Term, error) {
	pos := p.advance().Pos // consume "let"
	name, err := p.expect(TokenIdent)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenEq); err != nil {
		return nil, err
	}
	bound, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenIn); err != nil {
		return nil, err
	}
	body, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &Let{Name: name.Text, Bound: bound, Body: body, P: pos}, nil
}

// parseLam: "\" param+ "->" expr
// param = IDENT | "(" IDENT ":" type ")"
func (p *Parser) parseLam() (Term, error) {
	pos := p.advance().Pos // consume "\"

	type lamParam struct {
		name string
		ty   Type
	}
	var params []lamParam

	for p.peek().Type != TokenArrow {
		if p.peek().Type == TokenEOF {
			return nil, fmt.Errorf("%s: expected '->' in lambda", p.peek().Pos)
		}
		if p.peek().Type == TokenLParen {
			p.advance() // consume "("
			nm, err := p.expect(TokenIdent)
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(TokenColon); err != nil {
				return nil, err
			}
			ty, err := p.parseType()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(TokenRParen); err != nil {
				return nil, err
			}
			params = append(params, lamParam{name: nm.Text, ty: ty})
		} else if p.peek().Type == TokenIdent {
			nm := p.advance()
			params = append(params, lamParam{name: nm.Text, ty: nil})
		} else {
			return nil, fmt.Errorf("%s: expected parameter name or '(' in lambda, got %q", p.peek().Pos, p.peek().Text)
		}
	}

	p.advance() // consume "->"
	if len(params) == 0 {
		return nil, fmt.Errorf("%s: lambda must have at least one parameter", pos)
	}

	body, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	// Build nested lambdas (right to left)
	result := body
	for i := len(params) - 1; i >= 0; i-- {
		result = &Lam{Param: params[i].name, ParamType: params[i].ty, Body: result, P: pos}
	}
	return result, nil
}

// parseIf: "if" expr "then" expr "else" expr
func (p *Parser) parseIf() (Term, error) {
	pos := p.advance().Pos // consume "if"
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenThen); err != nil {
		return nil, err
	}
	then, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenElse); err != nil {
		return nil, err
	}
	els, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return &IfExpr{Cond: cond, Then: then, Else: els, P: pos}, nil
}

// parseOr: andExpr ("||" andExpr)*
func (p *Parser) parseOr() (Term, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == TokenPipePipe {
		pos := p.advance().Pos
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &App{Func: &App{Func: &Prim{Op: PrimOr, P: pos}, Arg: left, P: pos}, Arg: right, P: pos}
	}
	return left, nil
}

// parseAnd: cmpExpr ("&&" cmpExpr)*
func (p *Parser) parseAnd() (Term, error) {
	left, err := p.parseCmp()
	if err != nil {
		return nil, err
	}
	for p.peek().Type == TokenAmpAmp {
		pos := p.advance().Pos
		right, err := p.parseCmp()
		if err != nil {
			return nil, err
		}
		left = &App{Func: &App{Func: &Prim{Op: PrimAnd, P: pos}, Arg: left, P: pos}, Arg: right, P: pos}
	}
	return left, nil
}

// parseCmp: addExpr (("==" | "/=" | "<" | "<=" | ">" | ">=") addExpr)?
func (p *Parser) parseCmp() (Term, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	var op PrimOp
	switch p.peek().Type {
	case TokenEqEq:
		op = PrimEq
	case TokenNeq:
		op = PrimNeq
	case TokenLt:
		op = PrimLt
	case TokenLe:
		op = PrimLe
	case TokenGt:
		op = PrimGt
	case TokenGe:
		op = PrimGe
	default:
		return left, nil
	}
	pos := p.advance().Pos
	right, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	return &App{Func: &App{Func: &Prim{Op: op, P: pos}, Arg: left, P: pos}, Arg: right, P: pos}, nil
}

// parseAdd: mulExpr (("+" | "-") mulExpr)*
func (p *Parser) parseAdd() (Term, error) {
	left, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for {
		var op PrimOp
		switch p.peek().Type {
		case TokenPlus:
			op = PrimAdd
		case TokenMinus:
			op = PrimSub
		default:
			return left, nil
		}
		pos := p.advance().Pos
		right, err := p.parseMul()
		if err != nil {
			return nil, err
		}
		left = &App{Func: &App{Func: &Prim{Op: op, P: pos}, Arg: left, P: pos}, Arg: right, P: pos}
	}
}

// parseMul: unaryExpr (("*" | "/" | "%") unaryExpr)*
func (p *Parser) parseMul() (Term, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		var op PrimOp
		switch p.peek().Type {
		case TokenStar:
			op = PrimMul
		case TokenSlash:
			op = PrimDiv
		case TokenPercent:
			op = PrimMod
		default:
			return left, nil
		}
		pos := p.advance().Pos
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &App{Func: &App{Func: &Prim{Op: op, P: pos}, Arg: left, P: pos}, Arg: right, P: pos}
	}
}

// parseUnary: "-" unaryExpr | compExpr
func (p *Parser) parseUnary() (Term, error) {
	if p.peek().Type == TokenMinus {
		pos := p.advance().Pos
		arg, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		// Optimize: negate a literal directly
		if lit, ok := arg.(*Lit); ok && lit.Kind == LitInt {
			return &Lit{Kind: LitInt, IntVal: -lit.IntVal, P: pos}, nil
		}
		return &App{Func: &Prim{Op: PrimNeg, P: pos}, Arg: arg, P: pos}, nil
	}
	return p.parseComp()
}

// parseComp: appExpr ("." appExpr)* (right-associative)
func (p *Parser) parseComp() (Term, error) {
	first, err := p.parseApp()
	if err != nil {
		return nil, err
	}
	if p.peek().Type != TokenDot {
		return first, nil
	}
	// Collect all parts: f . g . h
	parts := []Term{first}
	for p.peek().Type == TokenDot {
		p.advance()
		next, err := p.parseApp()
		if err != nil {
			return nil, err
		}
		parts = append(parts, next)
	}
	// Right-associate: f . g . h = compose f (compose g h)
	result := parts[len(parts)-1]
	for i := len(parts) - 2; i >= 0; i-- {
		pos := parts[i].Pos()
		result = &App{
			Func: &App{Func: &Prim{Op: PrimCompose, P: pos}, Arg: parts[i], P: pos},
			Arg:  result,
			P:    pos,
		}
	}
	return result, nil
}

// parseApp: atom+ (left-associative application)
func (p *Parser) parseApp() (Term, error) {
	first, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	for p.isAtomStart() {
		arg, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		first = &App{Func: first, Arg: arg, P: first.Pos()}
	}
	return first, nil
}

// isAtomStart returns true if the current token can start an atom.
func (p *Parser) isAtomStart() bool {
	switch p.peek().Type {
	case TokenInt, TokenString, TokenIdent, TokenTrue, TokenFalse,
		TokenLParen, TokenLBrack:
		return true
	}
	return false
}

// parseAtom: IDENT | INT | BOOL | STRING | list | paren_or_tuple | section
func (p *Parser) parseAtom() (Term, error) {
	tok := p.peek()

	switch tok.Type {
	case TokenInt:
		p.advance()
		return &Lit{Kind: LitInt, IntVal: tok.IntVal, P: tok.Pos}, nil

	case TokenString:
		p.advance()
		return &Lit{Kind: LitString, StrVal: tok.StrVal, P: tok.Pos}, nil

	case TokenTrue:
		p.advance()
		return &Lit{Kind: LitBool, BoolVal: true, P: tok.Pos}, nil

	case TokenFalse:
		p.advance()
		return &Lit{Kind: LitBool, BoolVal: false, P: tok.Pos}, nil

	case TokenIdent:
		p.advance()
		// Resolve primitive names
		if op, ok := IsPrimName(tok.Text); ok {
			return &Prim{Op: op, P: tok.Pos}, nil
		}
		return &Var{Name: tok.Text, P: tok.Pos}, nil

	case TokenLBrack:
		return p.parseList()

	case TokenLParen:
		return p.parseParenOrTupleOrSection()

	default:
		return nil, fmt.Errorf("%s: expected expression, got %q", tok.Pos, tok.Text)
	}
}

// parseList: "[" [expr ("," expr)*] "]"
func (p *Parser) parseList() (Term, error) {
	pos := p.advance().Pos // consume "["
	if p.peek().Type == TokenRBrack {
		p.advance()
		return &ListLit{Elems: nil, P: pos}, nil
	}
	var elems []Term
	first, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	elems = append(elems, first)
	for p.peek().Type == TokenComma {
		p.advance()
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		elems = append(elems, e)
	}
	if _, err := p.expect(TokenRBrack); err != nil {
		return nil, err
	}
	return &ListLit{Elems: elems, P: pos}, nil
}

// parseParenOrTupleOrSection handles:
//   (expr)       - grouping
//   (expr, expr) - tuple
//   (op)         - operator section
func (p *Parser) parseParenOrTupleOrSection() (Term, error) {
	pos := p.advance().Pos // consume "("

	// Check for operator section: (+), (-), (*), etc.
	if isOpToken(p.peek().Type) && p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Type == TokenRParen {
		opTok := p.advance()
		p.advance() // consume ")"
		op := tokenToOp(opTok.Type)
		return &Prim{Op: op, P: pos}, nil
	}

	// Check for (.) specifically
	if p.peek().Type == TokenDot && p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Type == TokenRParen {
		p.advance() // consume "."
		p.advance() // consume ")"
		return &Prim{Op: PrimCompose, P: pos}, nil
	}

	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	if p.peek().Type == TokenComma {
		// Tuple
		p.advance()
		second, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return &TupleLit{Fst: expr, Snd: second, P: pos}, nil
	}

	// Grouping
	if _, err := p.expect(TokenRParen); err != nil {
		return nil, err
	}
	return expr, nil
}

func isOpToken(tt TokenType) bool {
	switch tt {
	case TokenPlus, TokenMinus, TokenStar, TokenSlash, TokenPercent,
		TokenEqEq, TokenNeq, TokenLt, TokenLe, TokenGt, TokenGe,
		TokenAmpAmp, TokenPipePipe, TokenColon:
		return true
	}
	return false
}

func tokenToOp(tt TokenType) PrimOp {
	switch tt {
	case TokenPlus:
		return PrimAdd
	case TokenMinus:
		return PrimSub
	case TokenStar:
		return PrimMul
	case TokenSlash:
		return PrimDiv
	case TokenPercent:
		return PrimMod
	case TokenEqEq:
		return PrimEq
	case TokenNeq:
		return PrimNeq
	case TokenLt:
		return PrimLt
	case TokenLe:
		return PrimLe
	case TokenGt:
		return PrimGt
	case TokenGe:
		return PrimGe
	case TokenAmpAmp:
		return PrimAnd
	case TokenPipePipe:
		return PrimOr
	case TokenColon:
		return PrimCons
	default:
		return PrimOp("?")
	}
}

// --- Type parser ---

// parseType: funType
func (p *Parser) parseType() (Type, error) {
	return p.parseFunType()
}

// parseFunType: atomType ("->" funType)?  (right-associative)
func (p *Parser) parseFunType() (Type, error) {
	left, err := p.parseAtomType()
	if err != nil {
		return nil, err
	}
	if p.peek().Type == TokenArrow {
		p.advance()
		right, err := p.parseFunType()
		if err != nil {
			return nil, err
		}
		return &TFun{Param: left, Result: right}, nil
	}
	return left, nil
}

// parseAtomType: "Int" | "Bool" | "String" | "[" type "]" | "(" type "," type ")" | "(" type ")" | IDENT
func (p *Parser) parseAtomType() (Type, error) {
	tok := p.peek()

	switch tok.Type {
	case TokenIdent:
		p.advance()
		switch tok.Text {
		case "Int":
			return &TInt{}, nil
		case "Bool":
			return &TBool{}, nil
		case "String":
			return &TString{}, nil
		default:
			// Type variable
			return &TVar{Name: tok.Text}, nil
		}

	case TokenLBrack:
		p.advance()
		elem, err := p.parseType()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TokenRBrack); err != nil {
			return nil, err
		}
		return &TList{Elem: elem}, nil

	case TokenLParen:
		p.advance()
		t, err := p.parseType()
		if err != nil {
			return nil, err
		}
		if p.peek().Type == TokenComma {
			p.advance()
			t2, err := p.parseType()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(TokenRParen); err != nil {
				return nil, err
			}
			return &TTuple{Fst: t, Snd: t2}, nil
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return t, nil

	default:
		return nil, fmt.Errorf("%s: expected type, got %q", tok.Pos, tok.Text)
	}
}
