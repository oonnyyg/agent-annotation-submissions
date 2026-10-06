package engine

import (
	"fmt"
	"math"
	"strings"
)

// 公式仅支持以下子集：
//   数字、单元格引用（A1..Z100）、四则运算 + - * /、括号、
//   一元负号、SUM(矩形区域[, 矩形区域...])。
// 不存在任何函数调用/代码执行路径：SUM 是唯一被识别的名字。

// Node 是解析后的公式节点。
type Node interface {
	deps() []string
}

type numberNode float64

func (numberNode) deps() []string { return nil }

type refNode struct{ addr string }

func (n refNode) deps() []string { return []string{n.addr} }

type unaryNode struct {
	op      byte // 目前仅支持 '-'
	operand Node
}

func (n unaryNode) deps() []string { return n.operand.deps() }

type binaryNode struct {
	op          byte
	left, right Node
}

func (n binaryNode) deps() []string { return append(n.left.deps(), n.right.deps()...) }

type sumNode struct {
	cells []string // 展开后的区域单元格（去重、按首次出现排序）
}

func (n sumNode) deps() []string { return n.cells }

type parser struct {
	src string
	pos int
}

// ParseFormula 解析等号之后的公式正文。
func ParseFormula(body string) (Node, error) {
	p := &parser{src: body}
	p.skipSpaces()
	node, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.skipSpaces()
	if p.pos != len(p.src) {
		return nil, fmt.Errorf("公式 %q 语法错误：%q 之后存在无法识别的内容", body, p.src[:p.pos])
	}
	return node, nil
}

func (p *parser) skipSpaces() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

func (p *parser) parseExpr() (Node, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpaces()
		if p.pos >= len(p.src) || (p.src[p.pos] != '+' && p.src[p.pos] != '-') {
			return left, nil
		}
		op := p.src[p.pos]
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: op, left: left, right: right}
	}
}

func (p *parser) parseTerm() (Node, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpaces()
		if p.pos >= len(p.src) || (p.src[p.pos] != '*' && p.src[p.pos] != '/') {
			return left, nil
		}
		op := p.src[p.pos]
		p.pos++
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: op, left: left, right: right}
	}
}

func (p *parser) parseFactor() (Node, error) {
	p.skipSpaces()
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("公式 %q 语法错误：表达式不完整", p.src)
	}
	switch p.src[p.pos] {
	case '+':
		p.pos++
		return p.parseFactor()
	case '-':
		p.pos++
		operand, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		return unaryNode{op: '-', operand: operand}, nil
	case '(':
		p.pos++
		node, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if p.pos >= len(p.src) || p.src[p.pos] != ')' {
			return nil, fmt.Errorf("公式 %q 语法错误：缺少右括号", p.src)
		}
		p.pos++
		return node, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Node, error) {
	p.skipSpaces()
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("公式 %q 语法错误：表达式不完整", p.src)
	}
	ch := p.src[p.pos]
	if ch >= '0' && ch <= '9' || ch == '.' {
		return p.parseNumber()
	}
	if isLetter(ch) {
		return p.parseName()
	}
	return nil, fmt.Errorf("公式 %q 语法错误：位置 %d 出现意外字符 %q", p.src, p.pos+1, string(ch))
}

func (p *parser) parseNumber() (Node, error) {
	start := p.pos
	dot := false
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if ch >= '0' && ch <= '9' {
			p.pos++
			continue
		}
		if ch == '.' && !dot {
			dot = true
			p.pos++
			continue
		}
		break
	}
	text := p.src[start:p.pos]
	var value float64
	if _, err := fmt.Sscanf(text, "%g", &value); err != nil || math.IsNaN(value) {
		return nil, fmt.Errorf("公式 %q 语法错误：数字 %q 非法", p.src, text)
	}
	if math.IsInf(value, 0) {
		return nil, fmt.Errorf("公式 %q 语法错误：数字 %q 非有限值", p.src, text)
	}
	return numberNode(value), nil
}

func isLetter(ch byte) bool {
	return ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

func (p *parser) parseName() (Node, error) {
	start := p.pos
	for p.pos < len(p.src) && isLetter(p.src[p.pos]) {
		p.pos++
	}
	name := strings.ToUpper(p.src[start:p.pos])
	p.skipSpaces()
	if name == "SUM" && p.pos < len(p.src) && p.src[p.pos] == '(' {
		return p.parseSumArgs()
	}
	p.pos = start
	return p.parseCellRef()
}

func (p *parser) parseCellRef() (Node, error) {
	start := p.pos
	if p.pos >= len(p.src) || !isLetter(p.src[p.pos]) {
		return nil, fmt.Errorf("公式 %q 语法错误：位置 %d 应为单元格引用", p.src, p.pos+1)
	}
	p.pos++
	digitStart := p.pos
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == digitStart {
		return nil, fmt.Errorf("公式 %q 语法错误：未知名称或非法引用 %q", p.src, p.src[start:p.pos])
	}
	addr, err := ParseAddr(p.src[start:p.pos])
	if err != nil {
		return nil, err
	}
	return refNode{addr: addr.String()}, nil
}

func (p *parser) parseSumArgs() (Node, error) {
	p.pos++ // 消费 '('
	var cells []string
	seen := map[string]bool{}
	first := true
	for {
		p.skipSpaces()
		if !first {
			if p.pos >= len(p.src) || p.src[p.pos] != ',' {
				return nil, fmt.Errorf("公式 %q 语法错误：SUM 参数间缺少逗号", p.src)
			}
			p.pos++
			p.skipSpaces()
		}
		rng, err := p.parseRangeArg()
		if err != nil {
			return nil, err
		}
		for _, cell := range rng.Cells() {
			if !seen[cell] {
				seen[cell] = true
				cells = append(cells, cell)
			}
		}
		first = false
		p.skipSpaces()
		if p.pos < len(p.src) && p.src[p.pos] == ')' {
			p.pos++
			return sumNode{cells: cells}, nil
		}
	}
}

func (p *parser) parseRangeArg() (CellRange, error) {
	fromNode, err := p.parseCellRef()
	if err != nil {
		return CellRange{}, err
	}
	from := fromNode.(refNode).addr
	p.skipSpaces()
	if p.pos >= len(p.src) || p.src[p.pos] != ':' {
		return CellRange{}, fmt.Errorf("公式 %q 语法错误：SUM 只接受 A1:B2 形式的区域", p.src)
	}
	p.pos++
	p.skipSpaces()
	toNode, err := p.parseCellRef()
	if err != nil {
		return CellRange{}, err
	}
	to := toNode.(refNode).addr
	rng, err := ParseRange(from + ":" + to)
	if err != nil {
		return CellRange{}, err
	}
	return rng, nil
}
