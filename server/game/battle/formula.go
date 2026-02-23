package battle

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// CharacterStats holds the stat values usable in damage formulas.
type CharacterStats struct {
	HP, MP                     int
	Atk, Def, Mat, Mdf, Agi, Luk int
	Level                      int
}

// EvalFormula evaluates an RMMV-style damage formula string.
// Variables: a.atk, a.def, a.mat, a.mdf, a.agi, a.luk, a.hp, a.mp, a.level
//            b.*  (same for defender)
// Operators: + - * /  with parentheses.
// Functions: Math.floor, Math.ceil, Math.round, Math.max, Math.min, Math.abs
func EvalFormula(formula string, a, b *CharacterStats) (float64, error) {
	// Reject complex JS (fallback handled by caller).
	lower := strings.ToLower(formula)
	for _, kw := range []string{"if", "function", "var", "let", "const", ";", "{", "}"} {
		if strings.Contains(lower, kw) {
			return 0, fmt.Errorf("formula requires JS sandbox: %q", formula)
		}
	}
	p := &parser{input: formula, a: a, b: b}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	if p.pos < len(p.input) {
		return 0, fmt.Errorf("unexpected chars at pos %d: %q", p.pos, p.input[p.pos:])
	}
	return v, nil
}

// ---- Recursive-descent parser ----

type parser struct {
	input string
	pos   int
	a, b  *CharacterStats
}

func (p *parser) skipWS() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}

func (p *parser) peek() byte {
	p.skipWS()
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *parser) consume() byte {
	p.skipWS()
	if p.pos >= len(p.input) {
		return 0
	}
	ch := p.input[p.pos]
	p.pos++
	return ch
}

// parseExpr = parseTerm (('+' | '-') parseTerm)*
func (p *parser) parseExpr() (float64, error) {
	v, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		ch := p.peek()
		if ch != '+' && ch != '-' {
			break
		}
		p.consume()
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if ch == '+' {
			v += right
		} else {
			v -= right
		}
	}
	return v, nil
}

// parseTerm = parseFactor (('*' | '/') parseFactor)*
func (p *parser) parseTerm() (float64, error) {
	v, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		ch := p.peek()
		if ch != '*' && ch != '/' {
			break
		}
		p.consume()
		right, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		if ch == '*' {
			v *= right
		} else {
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			v /= right
		}
	}
	return v, nil
}

// parseFactor = '(' parseExpr ')' | number | variable | Math.func(args)
func (p *parser) parseFactor() (float64, error) {
	ch := p.peek()
	switch {
	case ch == '(':
		p.consume()
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipWS()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return 0, fmt.Errorf("expected ')'")
		}
		p.pos++
		return v, nil

	case ch == '-':
		p.consume()
		v, err := p.parseFactor()
		return -v, err

	case unicode.IsDigit(rune(ch)) || ch == '.':
		return p.parseNumber()

	case ch == 'a' || ch == 'b':
		return p.parseVariable()

	case ch == 'M':
		return p.parseMathFunc()

	default:
		return 0, fmt.Errorf("unexpected character %q at pos %d", ch, p.pos)
	}
}

func (p *parser) parseNumber() (float64, error) {
	p.skipWS()
	start := p.pos
	hasDot := false
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == '.' && !hasDot {
			hasDot = true
			p.pos++
		} else if c >= '0' && c <= '9' {
			p.pos++
		} else {
			break
		}
	}
	return strconv.ParseFloat(p.input[start:p.pos], 64)
}

func (p *parser) parseVariable() (float64, error) {
	p.skipWS()
	who := p.input[p.pos]
	p.pos++
	if p.pos >= len(p.input) || p.input[p.pos] != '.' {
		return 0, fmt.Errorf("expected '.' after '%c'", who)
	}
	p.pos++
	// read field name
	start := p.pos
	for p.pos < len(p.input) && (unicode.IsLetter(rune(p.input[p.pos])) || p.input[p.pos] == '_') {
		p.pos++
	}
	field := p.input[start:p.pos]
	var stats *CharacterStats
	if who == 'a' {
		stats = p.a
	} else {
		stats = p.b
	}
	return statField(stats, field)
}

func statField(s *CharacterStats, field string) (float64, error) {
	switch field {
	case "hp":
		return float64(s.HP), nil
	case "mp":
		return float64(s.MP), nil
	case "atk":
		return float64(s.Atk), nil
	case "def":
		return float64(s.Def), nil
	case "mat":
		return float64(s.Mat), nil
	case "mdf":
		return float64(s.Mdf), nil
	case "agi":
		return float64(s.Agi), nil
	case "luk":
		return float64(s.Luk), nil
	case "level":
		return float64(s.Level), nil
	}
	return 0, fmt.Errorf("unknown stat field %q", field)
}

func (p *parser) parseMathFunc() (float64, error) {
	p.skipWS()
	// expect "Math."
	prefix := "Math."
	if !strings.HasPrefix(p.input[p.pos:], prefix) {
		return 0, fmt.Errorf("expected Math.xxx at pos %d", p.pos)
	}
	p.pos += len(prefix)
	// read function name
	start := p.pos
	for p.pos < len(p.input) && unicode.IsLetter(rune(p.input[p.pos])) {
		p.pos++
	}
	fname := p.input[start:p.pos]
	p.skipWS()
	if p.pos >= len(p.input) || p.input[p.pos] != '(' {
		return 0, fmt.Errorf("expected '(' after Math.%s", fname)
	}
	p.pos++
	// parse arguments
	var args []float64
	for {
		p.skipWS()
		if p.pos < len(p.input) && p.input[p.pos] == ')' {
			p.pos++
			break
		}
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		args = append(args, v)
		p.skipWS()
		if p.pos < len(p.input) && p.input[p.pos] == ',' {
			p.pos++
		}
	}
	return applyMathFunc(fname, args)
}

func applyMathFunc(name string, args []float64) (float64, error) {
	switch name {
	case "floor":
		if len(args) != 1 {
			return 0, fmt.Errorf("Math.floor expects 1 argument")
		}
		return math.Floor(args[0]), nil
	case "ceil":
		if len(args) != 1 {
			return 0, fmt.Errorf("Math.ceil expects 1 argument")
		}
		return math.Ceil(args[0]), nil
	case "round":
		if len(args) != 1 {
			return 0, fmt.Errorf("Math.round expects 1 argument")
		}
		return math.Round(args[0]), nil
	case "abs":
		if len(args) != 1 {
			return 0, fmt.Errorf("Math.abs expects 1 argument")
		}
		return math.Abs(args[0]), nil
	case "max":
		if len(args) == 0 {
			return 0, fmt.Errorf("Math.max expects >=1 argument")
		}
		v := args[0]
		for _, a := range args[1:] {
			if a > v {
				v = a
			}
		}
		return v, nil
	case "min":
		if len(args) == 0 {
			return 0, fmt.Errorf("Math.min expects >=1 argument")
		}
		v := args[0]
		for _, a := range args[1:] {
			if a < v {
				v = a
			}
		}
		return v, nil
	}
	return 0, fmt.Errorf("unknown Math.%s", name)
}
