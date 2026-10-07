package copybook

import (
	"strconv"
	"strings"
)

// Field is one parsed copybook entry (a group or an elementary item).
type Field struct {
	Pos      Pos
	Level    int
	Name     string
	HasPic   bool
	Pic      Pic
	Occurs   int // 0 means no OCCURS clause
	Children []*Field
}

// Pic describes a PICTURE clause for the supported subset.
type Pic struct {
	Kind   PicKind
	Length int // text: byte length; numeric: integer digits
	Scale  int // numeric: implied decimal places
	Signed bool
	Comp3  bool
}

type PicKind int

const (
	PicText PicKind = iota
	PicNumeric
)

// Parse parses a free-format copybook containing exactly one top-level
// (level-01) record definition.
func Parse(src string) ([]*Field, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	var fields []*Field
	for p.peek().kind != tokEOF {
		f, err := p.parseField()
		if err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
	return fields, nil
}

type parser struct {
	toks []token
	i    int
}

func (p *parser) peek() token { return p.toks[p.i] }
func (p *parser) next() token { t := p.toks[p.i]; p.i++; return t }

var rejectedWords = map[string]string{
	"REDEFINES":  "REDEFINES is not supported",
	"RENAMES":    "RENAMES is not supported",
	"VALUE":      "VALUE clauses are not supported",
	"USAGE":      "USAGE clauses other than COMP-3 are not supported",
	"COMP":       "COMP/BINARY fields are not supported",
	"COMP-1":     "COMP-1 is not supported",
	"COMP-2":     "COMP-2 is not supported",
	"COMP-4":     "COMP-4 is not supported",
	"COMP-5":     "COMP-5 is not supported",
	"BINARY":     "BINARY fields are not supported",
	"FLOAT":      "floating point is not supported",
	"POINTER":    "POINTER is not supported",
	"INDEX":      "INDEX is not supported",
	"JUSTIFIED":  "JUSTIFIED is not supported",
	"SIGN":       "SIGN clauses are not supported",
	"BLANK":      "BLANK is not supported",
	"FILLER":     "FILLER is not supported; every field must be named",
	"DEPENDING":  "OCCURS DEPENDING ON (variable-length arrays) is not supported",
	"ASCENDING":  "table keys are not supported",
	"DESCENDING": "table keys are not supported",
	"INDEXED":    "INDEXED BY is not supported",
}

func (p *parser) parseField() (*Field, error) {
	lvTok := p.next()
	if lvTok.kind != tokNumber {
		return nil, errAt(lvTok.pos, "expected level number, got %q", lvTok.text)
	}
	level, err := strconv.Atoi(lvTok.text)
	if err != nil || level < 1 || level > 49 {
		return nil, errAt(lvTok.pos, "invalid level number %q: only 01-49 are supported", lvTok.text)
	}
	nameTok := p.next()
	if nameTok.kind != tokWord {
		return nil, errAt(nameTok.pos, "expected field name after level %d", level)
	}
	if msg, bad := rejectedWords[nameTok.text]; bad {
		return nil, errAt(nameTok.pos, "%s", msg)
	}
	f := &Field{Pos: lvTok.pos, Level: level, Name: nameTok.text}

	for {
		t := p.peek()
		if t.kind == tokPeriod {
			p.next()
			return f, nil
		}
		if t.kind != tokWord {
			return nil, errAt(t.pos, "unexpected token %q in field %s", t.text, f.Name)
		}
		if msg, bad := rejectedWords[t.text]; bad {
			return nil, errAt(t.pos, "%s", msg)
		}
		switch t.text {
		case "PIC", "PICTURE":
			p.next()
			if err := p.parsePic(f); err != nil {
				return nil, err
			}
		case "OCCURS":
			p.next()
			nTok := p.next()
			if nTok.kind == tokWord && nTok.text == "DEPENDING" {
				return nil, errAt(nTok.pos, "OCCURS DEPENDING ON (variable-length arrays) is not supported")
			}
			if nTok.kind != tokNumber {
				return nil, errAt(nTok.pos, "OCCURS requires a fixed integer count")
			}
			n, err := strconv.Atoi(nTok.text)
			if err != nil || n < 1 {
				return nil, errAt(nTok.pos, "invalid OCCURS count %q", nTok.text)
			}
			if p.peek().kind == tokWord && p.peek().text == "TIMES" {
				p.next()
			}
			if p.peek().kind == tokWord && p.peek().text == "DEPENDING" {
				return nil, errAt(p.peek().pos, "OCCURS DEPENDING ON (variable-length arrays) is not supported")
			}
			f.Occurs = n
		case "COMP-3", "PACKED-DECIMAL":
			p.next()
			f.Pic.Comp3 = true
		case "USAGE":
			p.next()
			t2 := p.next()
			if t2.kind == tokWord && (t2.text == "COMP-3" || t2.text == "PACKED-DECIMAL") {
				f.Pic.Comp3 = true
			} else if t2.kind == tokWord && t2.text == "DISPLAY" {
				// default usage, accepted
			} else {
				return nil, errAt(t2.pos, "USAGE %s is not supported", t2.text)
			}
		case "IS":
			p.next() // optional noise word
		default:
			return nil, errAt(t.pos, "unsupported clause %q in field %s", t.text, f.Name)
		}
	}
}

// parsePic consumes the PIC character string, which the lexer split into
// word/number/paren tokens, e.g. S 9 ( 3 ) V 9 ( 2 ).
func (p *parser) parsePic(f *Field) error {
	start := p.peek()
	var b strings.Builder
	depth := 0
	for {
		t := p.peek()
		switch t.kind {
		case tokWord:
			if depth == 0 && !isPicWord(t.text) {
				goto done
			}
			b.WriteString(t.text)
			p.next()
		case tokNumber:
			b.WriteString(t.text)
			p.next()
		case tokLParen:
			depth++
			b.WriteByte('(')
			p.next()
		case tokRParen:
			depth--
			b.WriteByte(')')
			p.next()
		default:
			goto done
		}
	}
done:
	pic, err := parsePicString(start.pos, b.String())
	if err != nil {
		return err
	}
	f.HasPic = true
	f.Pic = pic
	return nil
}

func isPicWord(w string) bool {
	for _, c := range w {
		switch c {
		case 'X', '9', 'S', 'V', 'A':
		default:
			return false
		}
	}
	return true
}

func parsePicString(pos Pos, s string) (Pic, error) {
	if s == "" {
		return Pic{}, errAt(pos, "empty PICTURE string")
	}
	// Expand repeat counts: X(10) -> XXXXXXXXXX.
	var exp strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		exp.WriteByte(c)
		if i+1 < len(s) && s[i+1] == '(' {
			j := i + 2
			num := ""
			for j < len(s) && s[j] != ')' {
				num += string(s[j])
				j++
			}
			if j >= len(s) {
				return Pic{}, errAt(pos, "unbalanced parenthesis in PIC %q", s)
			}
			n, err := strconv.Atoi(num)
			if err != nil || n < 1 {
				return Pic{}, errAt(pos, "invalid repeat count in PIC %q", s)
			}
			for k := 1; k < n; k++ {
				exp.WriteByte(c)
			}
			i = j
		}
	}
	runes := exp.String()
	pic := Pic{}
	i := 0
	if runes[0] == 'S' {
		pic.Signed = true
		i++
	}
	counts := func(c byte) int {
		n := 0
		for i < len(runes) && runes[i] == c {
			n++
			i++
		}
		return n
	}
	switch {
	case i < len(runes) && (runes[i] == 'X' || runes[i] == 'A'):
		if pic.Signed {
			return Pic{}, errAt(pos, "text PIC %q cannot be signed", s)
		}
		n := 0
		for i < len(runes) && (runes[i] == 'X' || runes[i] == 'A') {
			n++
			i++
		}
		if i != len(runes) {
			return Pic{}, errAt(pos, "unsupported PIC %q", s)
		}
		pic.Kind = PicText
		pic.Length = n
	case i < len(runes) && runes[i] == '9':
		intDigits := counts('9')
		scale := 0
		if i < len(runes) && runes[i] == 'V' {
			i++
			scale = counts('9')
		}
		if i != len(runes) {
			return Pic{}, errAt(pos, "unsupported PIC %q: only 9, S, V are allowed in numeric pictures", s)
		}
		if intDigits+scale < 1 || intDigits+scale > 31 {
			return Pic{}, errAt(pos, "PIC %q: total digits must be between 1 and 31", s)
		}
		pic.Kind = PicNumeric
		pic.Length = intDigits
		pic.Scale = scale
	default:
		return Pic{}, errAt(pos, "unsupported PIC %q", s)
	}
	return pic, nil
}
