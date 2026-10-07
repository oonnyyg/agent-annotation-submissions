package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// escapePtrToken 按 RFC 6901 转义一个指针段。
func escapePtrToken(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	return strings.ReplaceAll(s, "/", "~1")
}

// unescapePtrToken 还原一个指针段；非法转义返回错误。
func unescapePtrToken(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '~' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			return "", fmt.Errorf("dangling '~' escape")
		}
		switch s[i+1] {
		case '0':
			b.WriteByte('~')
		case '1':
			b.WriteByte('/')
		default:
			return "", fmt.Errorf("invalid escape '~%c'", s[i+1])
		}
		i++
	}
	return b.String(), nil
}

// joinPtr 拼接 JSON Pointer 片段（自动转义）。
func joinPtr(base string, tokens ...string) string {
	for _, t := range tokens {
		base += "/" + escapePtrToken(t)
	}
	return base
}

// resolveRef 解析文件内 JSON Pointer 引用，并检测循环。
func (c *compiler) resolveRef(ref, fromPath string) (*Schema, []*SchemaError) {
	if !strings.HasPrefix(ref, "#") {
		return nil, []*SchemaError{{
			Path:   fromPath,
			Reason: fmt.Sprintf("only local refs (#/...) are supported, got %q; remote loading is disabled", ref),
		}}
	}
	ptr := strings.TrimPrefix(ref, "#")
	if ptr == "" {
		ptr = ""
	} else if !strings.HasPrefix(ptr, "/") {
		return nil, []*SchemaError{{
			Path:   fromPath,
			Reason: fmt.Sprintf("unsupported ref fragment %q (only JSON Pointer fragments are supported)", ref),
		}}
	}

	// 在原始文档上按指针定位，正确处理转义。
	node := c.raw
	if ptr != "" {
		for _, rawTok := range strings.Split(ptr[1:], "/") {
			tok, err := unescapePtrToken(rawTok)
			if err != nil {
				return nil, []*SchemaError{{Path: fromPath, Reason: fmt.Sprintf("bad escape in ref %q: %v", ref, err)}}
			}
			switch cur := node.(type) {
			case map[string]any:
				next, ok := cur[tok]
				if !ok {
					return nil, []*SchemaError{{Path: fromPath, Reason: fmt.Sprintf("unresolvable ref %q: missing key %q", ref, tok)}}
				}
				node = next
			case []any:
				idx, err := strconv.Atoi(tok)
				if err != nil || idx < 0 || idx >= len(cur) {
					return nil, []*SchemaError{{Path: fromPath, Reason: fmt.Sprintf("unresolvable ref %q: bad array index %q", ref, tok)}}
				}
				node = cur[idx]
			default:
				return nil, []*SchemaError{{Path: fromPath, Reason: fmt.Sprintf("unresolvable ref %q: cannot descend into scalar", ref)}}
			}
		}
	}

	// 循环检测：目标指针正在编译链上即构成环。
	if c.inProgress[ptr] {
		return nil, []*SchemaError{{Path: fromPath, Reason: fmt.Sprintf("circular ref %q detected", ref)}}
	}
	s, errs := c.compileNode(node, ptr)
	return s, errs
}

// toRat 将 JSON 数值精确转换为有理数（写法不同不影响相等性）。
func toRat(v any) (*big.Rat, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(n.String())
	return r, ok
}

// toNonNegInt 解析非负整数（用于长度类关键词）。
func toNonNegInt(v any) (int64, bool) {
	r, ok := toRat(v)
	if !ok || !r.IsInt() {
		return 0, false
	}
	n := r.Num()
	if !n.IsInt64() {
		return 0, false
	}
	i := n.Int64()
	if i < 0 {
		return 0, false
	}
	return i, true
}
