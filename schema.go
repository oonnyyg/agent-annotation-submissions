package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

// Schema 是已编译的 JSON Schema 子集节点。
type Schema struct {
	path string // 该节点在模式文档中的 JSON Pointer（不带 "#" 前缀）

	isBool bool
	boolV  bool

	ref     string
	refTo   *Schema
	typ     string
	hasType bool
	enum    []any
	hasEnum bool

	minimum *big.Rat
	maximum *big.Rat
	minExcl bool
	maxExcl bool

	minLength *int64
	maxLength *int64

	properties           map[string]*Schema
	required             map[string]bool
	additionalProperties *Schema // nil 表示未声明（允许）
	additionalAllowed    bool    // additionalProperties 为布尔时的取值

	items    *Schema
	minItems *int64
	maxItems *int64

	allOf []*Schema
	anyOf []*Schema
	oneOf []*Schema
	not   *Schema
}

// SchemaError 表示模式本身的结构错误（在校验数据前报告）。
type SchemaError struct {
	Path   string // 模式内 JSON Pointer
	Reason string
}

func (e *SchemaError) Error() string {
	return fmt.Sprintf("schema error at %s: %s", orRoot(e.Path), e.Reason)
}

func orRoot(p string) string {
	if p == "" {
		return "#"
	}
	return "#" + p
}

// supportedKeywords 是本工具明确支持的关键词白名单。
var supportedKeywords = map[string]bool{
	"$schema": true, "$id": true, "title": true, "description": true,
	"$defs": true, "definitions": true, "$ref": true,
	"type": true, "enum": true, "const": true,
	"minimum": true, "maximum": true,
	"exclusiveMinimum": true, "exclusiveMaximum": true,
	"minLength": true, "maxLength": true,
	"properties": true, "required": true, "additionalProperties": true,
	"items": true, "minItems": true, "maxItems": true,
	"allOf": true, "anyOf": true, "oneOf": true, "not": true,
}

var validTypes = map[string]bool{
	"null": true, "boolean": true, "number": true, "integer": true,
	"string": true, "array": true, "object": true,
}

// compiler 在编译期解析引用并检测循环。
type compiler struct {
	raw        any // 原始模式文档（json.Number 解码）
	byPointer  map[string]*Schema
	inProgress map[string]bool
}

// CompileSchema 解析并校验模式文档，返回根模式。
// 所有结构错误（不支持的类型、未知关键词、缺失/循环引用）在此阶段报告。
func CompileSchema(data []byte) (*Schema, []*SchemaError) {
	dec := json.NewDecoder(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff")))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, []*SchemaError{{Path: "", Reason: "invalid JSON: " + err.Error()}}
	}
	c := &compiler{raw: raw, byPointer: map[string]*Schema{}, inProgress: map[string]bool{}}
	root, errs := c.compileNode(raw, "")
	if len(errs) > 0 {
		return nil, errs
	}
	return root, nil
}

func (c *compiler) compileNode(raw any, path string) (*Schema, []*SchemaError) {
	if s, ok := c.byPointer[path]; ok {
		return s, nil
	}
	s := &Schema{path: path}
	c.byPointer[path] = s
	c.inProgress[path] = true
	defer delete(c.inProgress, path)

	switch v := raw.(type) {
	case bool:
		s.isBool = true
		s.boolV = v
		return s, nil
	case map[string]any:
		return s, c.compileObject(s, v, path)
	default:
		return nil, []*SchemaError{{Path: path, Reason: "schema must be an object or boolean"}}
	}
}

func (c *compiler) compileObject(s *Schema, m map[string]any, path string) []*SchemaError {
	var errs []*SchemaError
	fail := func(p, format string, args ...any) {
		errs = append(errs, &SchemaError{Path: p, Reason: fmt.Sprintf(format, args...)})
	}

	// 未知关键词：明确报不支持。
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !supportedKeywords[k] {
			fail(joinPtr(path, k), "unsupported keyword %q", k)
		}
	}

	if v, ok := m["$ref"]; ok {
		str, ok := v.(string)
		if !ok {
			fail(joinPtr(path, "$ref"), "$ref must be a string")
		} else {
			s.ref = str
			target, rerrs := c.resolveRef(str, path)
			errs = append(errs, rerrs...)
			s.refTo = target
		}
	}

	if v, ok := m["type"]; ok {
		str, ok := v.(string)
		if !ok {
			fail(joinPtr(path, "type"), "type must be a single string (type arrays are not supported)")
		} else if !validTypes[str] {
			fail(joinPtr(path, "type"), "unknown type %q", str)
		} else {
			s.typ, s.hasType = str, true
		}
	}

	if v, ok := m["enum"]; ok {
		arr, ok := v.([]any)
		if !ok || len(arr) == 0 {
			fail(joinPtr(path, "enum"), "enum must be a non-empty array")
		} else {
			s.enum, s.hasEnum = arr, true
		}
	}
	if v, ok := m["const"]; ok {
		s.enum, s.hasEnum = []any{v}, true
	}

	for _, kw := range []struct {
		name string
		dst  **big.Rat
	}{
		{"minimum", &s.minimum},
		{"maximum", &s.maximum},
	} {
		if v, ok := m[kw.name]; ok {
			r, ok := toRat(v)
			if !ok {
				fail(joinPtr(path, kw.name), "%s must be a number", kw.name)
			} else {
				*kw.dst = r
			}
		}
	}
	// exclusiveMinimum/exclusiveMaximum 仅接受数值形式（draft-06+）。
	for _, kw := range []struct {
		name string
		min  bool
	}{
		{"exclusiveMinimum", true},
		{"exclusiveMaximum", false},
	} {
		if v, ok := m[kw.name]; ok {
			r, ok := toRat(v)
			if !ok {
				fail(joinPtr(path, kw.name), "%s must be a number (boolean form is not supported)", kw.name)
			} else if kw.min {
				s.minimum, s.minExcl = r, true
			} else {
				s.maximum, s.maxExcl = r, true
			}
		}
	}
	if s.minimum != nil && s.maximum != nil && (s.minimum.Cmp(s.maximum) > 0 ||
		(s.minimum.Cmp(s.maximum) == 0 && (s.minExcl || s.maxExcl))) {
		fail(path, "minimum is greater than maximum")
	}

	for _, kw := range []struct {
		name string
		dst  **int64
	}{
		{"minLength", &s.minLength},
		{"maxLength", &s.maxLength},
		{"minItems", &s.minItems},
		{"maxItems", &s.maxItems},
	} {
		if v, ok := m[kw.name]; ok {
			n, ok := toNonNegInt(v)
			if !ok {
				fail(joinPtr(path, kw.name), "%s must be a non-negative integer", kw.name)
			} else {
				*kw.dst = &n
			}
		}
	}
	if s.minLength != nil && s.maxLength != nil && *s.minLength > *s.maxLength {
		fail(path, "minLength is greater than maxLength")
	}
	if s.minItems != nil && s.maxItems != nil && *s.minItems > *s.maxItems {
		fail(path, "minItems is greater than maxItems")
	}

	if v, ok := m["properties"]; ok {
		pm, ok := v.(map[string]any)
		if !ok {
			fail(joinPtr(path, "properties"), "properties must be an object")
		} else {
			s.properties = map[string]*Schema{}
			for name, sub := range pm {
				child, cerrs := c.compileNode(sub, joinPtr(path, "properties", name))
				errs = append(errs, cerrs...)
				s.properties[name] = child
			}
		}
	}

	if v, ok := m["required"]; ok {
		arr, ok := v.([]any)
		if !ok {
			fail(joinPtr(path, "required"), "required must be an array of strings")
		} else {
			s.required = map[string]bool{}
			for i, item := range arr {
				str, ok := item.(string)
				if !ok {
					fail(joinPtr(path, "required", fmt.Sprint(i)), "required entries must be strings")
					continue
				}
				if s.required[str] {
					fail(joinPtr(path, "required"), "duplicate required entry %q", str)
				}
				s.required[str] = true
			}
		}
	}

	if v, ok := m["additionalProperties"]; ok {
		child, cerrs := c.compileNode(v, joinPtr(path, "additionalProperties"))
		errs = append(errs, cerrs...)
		s.additionalProperties = child
		if child != nil && child.isBool {
			s.additionalAllowed = child.boolV
		} else {
			s.additionalAllowed = true
		}
	}

	if v, ok := m["items"]; ok {
		if _, isArr := v.([]any); isArr {
			fail(joinPtr(path, "items"), "array form of items (tuple validation) is not supported")
		} else {
			child, cerrs := c.compileNode(v, joinPtr(path, "items"))
			errs = append(errs, cerrs...)
			s.items = child
		}
	}

	for _, kw := range []struct {
		name string
		dst  *[]*Schema
	}{
		{"allOf", &s.allOf},
		{"anyOf", &s.anyOf},
		{"oneOf", &s.oneOf},
	} {
		if v, ok := m[kw.name]; ok {
			arr, ok := v.([]any)
			if !ok || len(arr) == 0 {
				fail(joinPtr(path, kw.name), "%s must be a non-empty array of schemas", kw.name)
				continue
			}
			for i, sub := range arr {
				child, cerrs := c.compileNode(sub, joinPtr(path, kw.name, fmt.Sprint(i)))
				errs = append(errs, cerrs...)
				*kw.dst = append(*kw.dst, child)
			}
		}
	}

	if v, ok := m["not"]; ok {
		child, cerrs := c.compileNode(v, joinPtr(path, "not"))
		errs = append(errs, cerrs...)
		s.not = child
	}

	for _, defsKey := range []string{"$defs", "definitions"} {
		if v, ok := m[defsKey]; ok {
			dm, ok := v.(map[string]any)
			if !ok {
				fail(joinPtr(path, defsKey), "%s must be an object", defsKey)
				continue
			}
			for name, sub := range dm {
				_, cerrs := c.compileNode(sub, joinPtr(path, defsKey, name))
				errs = append(errs, cerrs...)
			}
		}
	}

	return errs
}
