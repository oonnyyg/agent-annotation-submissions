package sheet

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// 仅支持题面定义的公式子集：数字、+ - * /、括号、单元格引用、SUM(区域)。

type tokenKind int

const (
	tkNum tokenKind = iota
	tkRef
	tkRange
	tkIdent
	tkOp
	tkLParen
	tkRParen
	tkComma
)

type token struct {
	kind tokenKind
	text string
	num  float64
	addr Addr
	rng  Range
}

func lexFormula(s string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '+' || c == '-' || c == '*' || c == '/':
			toks = append(toks, token{kind: tkOp, text: string(c)})
			i++
		case c == '(':
			toks = append(toks, token{kind: tkLParen, text: "("})
			i++
		case c == ')':
			toks = append(toks, token{kind: tkRParen, text: ")"})
			i++
		case c == ',':
			toks = append(toks, token{kind: tkComma, text: ","})
			i++
		case c >= '0' && c <= '9' || c == '.':
			j := i
			dots := 0
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
				if s[j] == '.' {
					dots++
				}
				j++
			}
			text := s[i:j]
			if dots > 1 {
				return nil, fmt.Errorf("非法数字 %q", text)
			}
			v, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("非法数字 %q", text)
			}
			toks = append(toks, token{kind: tkNum, text: text, num: v})
			i = j
		case c >= 'A' && c <= 'Z':
			j := i + 1
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			ref := s[i:j]
			if ref == "SUM" {
				toks = append(toks, token{kind: tkIdent, text: "SUM"})
				i = j
				continue
			}
			addr, err := ParseAddr(ref)
			if err != nil {
				return nil, err
			}
			// 区域引用 A1:B2
			if j < len(s) && s[j] == ':' {
				k := j + 1
				if k >= len(s) || s[k] < 'A' || s[k] > 'Z' {
					return nil, fmt.Errorf("非法区域起始于 %q", ref)
				}
				m := k + 1
				for m < len(s) && s[m] >= '0' && s[m] <= '9' {
					m++
				}
				rng, err := ParseRange(s[i:m])
				if err != nil {
					return nil, err
				}
				toks = append(toks, token{kind: tkRange, text: s[i:m], rng: rng})
				i = m
				continue
			}
			toks = append(toks, token{kind: tkRef, text: ref, addr: addr})
			i = j
		default:
			return nil, fmt.Errorf("非法字符 %q", string(c))
		}
	}
	return toks, nil
}

// Node 为语法树节点。
type Node struct {
	kind  int // 0 数字 1 引用 2 二元运算 3 SUM
	num   float64
	addr  Addr
	op    byte
	left  *Node
	right *Node
	rng   Range
}

const (
	ndNum = iota
	ndRef
	ndBin
	ndSum
)

type parser struct {
	toks []token
	pos  int
}

// ParseFormula 解析 "=" 之后的公式文本，返回 AST。
func ParseFormula(expr string) (*Node, error) {
	toks, err := lexFormula(expr)
	if err != nil {
		return nil, err
	}
	if len(toks) == 0 {
		return nil, fmt.Errorf("空公式")
	}
	p := &parser{toks: toks}
	n, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.pos != len(toks) {
		return nil, fmt.Errorf("公式 %q 存在多余内容", expr)
	}
	return n, nil
}

func (p *parser) peek() *token {
	if p.pos >= len(p.toks) {
		return nil
	}
	return &p.toks[p.pos]
}

func (p *parser) parseExpr() (*Node, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
}
	for t := p.peek(); t != nil && t.kind == tkOp && (t.text == "+" || t.text == "-"); t = p.peek() {
		op := t.text[0]
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = &Node{kind: ndBin, op: op, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseTerm() (*Node, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for t := p.peek(); t != nil && t.kind == tkOp && (t.text == "*" || t.text == "/"); t = p.peek() {
		op := t.text[0]
		p.pos++
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		left = &Node{kind: ndBin, op: op, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseFactor() (*Node, error) {
	t := p.peek()
	if t == nil {
		return nil, fmt.Errorf("公式不完整")
	}
	switch t.kind {
	case tkOp:
		if t.text != "-" && t.text != "+" {
			return nil, fmt.Errorf("意外的运算符 %s", t.text)
		}
		p.pos++
		operand, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		if t.text == "-" {
			return &Node{kind: ndBin, op: '-', left: &Node{kind: ndNum, num: 0}, right: operand}, nil
		}
		return operand, nil
	case tkNum:
		p.pos++
		return &Node{kind: ndNum, num: t.num}, nil
	case tkRef:
		p.pos++
		return &Node{kind: ndRef, addr: t.addr}, nil
	case tkLParen:
		p.pos++
		n, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		cl := p.peek()
		if cl == nil || cl.kind != tkRParen {
			return nil, fmt.Errorf("缺少右括号")
		}
		p.pos++
		return n, nil
	case tkIdent:
		if t.text != "SUM" {
			return nil, fmt.Errorf("不支持的函数 %s", t.text)
		}
		p.pos++
		lp := p.peek()
		if lp == nil || lp.kind != tkLParen {
			return nil, fmt.Errorf("SUM 后缺少左括号")
		}
		p.pos++
		arg := p.peek()
		if arg == nil || arg.kind != tkRange {
			return nil, fmt.Errorf("SUM 仅接受矩形区域参数，如 SUM(A1:B2)")
		}
		p.pos++
		rp := p.peek()
		if rp == nil || rp.kind != tkRParen {
			return nil, fmt.Errorf("SUM 后缺少右括号")
		}
		p.pos++
		return &Node{kind: ndSum, rng: arg.rng}, nil
	default:
		return nil, fmt.Errorf("公式中出现意外内容 %q", t.text)
	}
}

// Refs 返回 AST 依赖的全部单元格引用（含 SUM 区域展开），可能重复。
func (n *Node) Refs() []Addr {
	switch n.kind {
	case ndNum:
		return nil
	case ndRef:
		return []Addr{n.addr}
	case ndBin:
		return append(n.left.Refs(), n.right.Refs()...)
	case ndSum:
		return n.rng.Cells()
	}
	return nil
}

// ValidateContent 校验单元格原始输入：空串、有限数值或 = 公式。
func ValidateContent(raw string) (*Node, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if raw[0] == '=' {
		return ParseFormula(raw[1:])
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, fmt.Errorf("内容 %q 既不是数值也不是公式", raw)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, fmt.Errorf("数值必须有限")
	}
	return nil, nil
}
