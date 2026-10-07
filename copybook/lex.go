package copybook

import (
	"fmt"
	"strings"
)

// Pos identifies a location in the copybook source (1-based).
type Pos struct {
	Line int
	Col  int
}

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// Error is a copybook definition error with source position.
type Error struct {
	Pos Pos
	Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Pos, e.Msg) }

func errAt(p Pos, format string, args ...any) *Error {
	return &Error{Pos: p, Msg: fmt.Sprintf(format, args...)}
}

type tokenKind int

const (
	tokNumber tokenKind = iota
	tokWord
	tokLParen
	tokRParen
	tokPeriod
	tokEOF
)

type token struct {
	kind tokenKind
	text string
	pos  Pos
}

// lex splits the free-format copybook source into tokens. Words are
// uppercased; PIC strings like X(10) or S9(3)V99 arrive as separate tokens
// and are reassembled by the parser.
func lex(src string) ([]token, error) {
	var toks []token
	line, col := 1, 1
	i := 0
	for i < len(src) {
		c := src[i]
		if c == '\n' {
			line++
			col = 1
			i++
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' {
			col++
			i++
			continue
		}
		pos := Pos{line, col}
		switch {
		case c == '(':
			toks = append(toks, token{tokLParen, "(", pos})
			i++
			col++
		case c == ')':
			toks = append(toks, token{tokRParen, ")", pos})
			i++
			col++
		case c == '.':
			toks = append(toks, token{tokPeriod, ".", pos})
			i++
			col++
		case c >= '0' && c <= '9':
			j := i
			for j < len(src) && src[j] >= '0' && src[j] <= '9' {
				j++
			}
			toks = append(toks, token{tokNumber, src[i:j], pos})
			col += j - i
			i = j
		case isWordChar(c):
			j := i
			for j < len(src) && isWordChar(src[j]) {
				j++
			}
			toks = append(toks, token{tokWord, strings.ToUpper(src[i:j]), pos})
			col += j - i
			i = j
		default:
			return nil, errAt(pos, "unexpected character %q", string(c))
		}
	}
	toks = append(toks, token{tokEOF, "", Pos{line, col}})
	return toks, nil
}

func isWordChar(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-'
}
