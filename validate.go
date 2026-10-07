package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"unicode/utf8"
)

// InstanceError 表示一条数据不符合规则的错误（与模式错误分开报告）。
type InstanceError struct {
	InstancePath string // 数据内 JSON Pointer
	SchemaPath   string // 模式内 JSON Pointer
	Message      string
}

func (e *InstanceError) Error() string {
	return fmt.Sprintf("instance %s violates %s: %s",
		orRoot(e.InstancePath), orRoot(e.SchemaPath), e.Message)
}

// Validate 校验单条实例，返回按稳定顺序排列的错误。
func Validate(s *Schema, inst any) []*InstanceError {
	var errs []*InstanceError
	validateNode(s, inst, "", &errs)
	return errs
}

func validateNode(s *Schema, inst any, ipath string, errs *[]*InstanceError) {
	if s == nil {
		return
	}
	if s.isBool {
		if !s.boolV {
			addErr(errs, ipath, s.path, "boolean schema false rejects every instance")
		}
		return
	}

	// $ref 与其旁边的其他约束同时生效。
	if s.refTo != nil {
		validateNode(s.refTo, inst, ipath, errs)
	}

	if s.hasType && !typeMatches(s.typ, inst) {
		addErr(errs, ipath, joinPtr(s.path, "type"),
			fmt.Sprintf("expected type %s, got %s", s.typ, typeName(inst)))
		return // 类型不符时跳过后续同类约束，避免噪声
	}

	if s.hasEnum && !inEnum(s.enum, inst) {
		addErr(errs, ipath, joinPtr(s.path, "enum"), "value is not one of the allowed enum values")
	}

	if num, ok := inst.(json.Number); ok {
		if r, ok := toRat(num); ok {
			if s.minimum != nil {
				cmp := r.Cmp(s.minimum)
				if cmp < 0 || (cmp == 0 && s.minExcl) {
					addErr(errs, ipath, joinPtr(s.path, "minimum"),
						fmt.Sprintf("number %s is below minimum %s", num, s.minimum.RatString()))
				}
			}
			if s.maximum != nil {
				cmp := r.Cmp(s.maximum)
				if cmp > 0 || (cmp == 0 && s.maxExcl) {
					addErr(errs, ipath, joinPtr(s.path, "maximum"),
						fmt.Sprintf("number %s is above maximum %s", num, s.maximum.RatString()))
				}
			}
		}
	}

	if str, ok := inst.(string); ok {
		n := int64(utf8.RuneCountInString(str)) // 按 Unicode 码点计数
		if s.minLength != nil && n < *s.minLength {
			addErr(errs, ipath, joinPtr(s.path, "minLength"),
				fmt.Sprintf("string length %d is less than minLength %d", n, *s.minLength))
		}
		if s.maxLength != nil && n > *s.maxLength {
			addErr(errs, ipath, joinPtr(s.path, "maxLength"),
				fmt.Sprintf("string length %d exceeds maxLength %d", n, *s.maxLength))
		}
	}

	if obj, ok := inst.(map[string]any); ok {
		// 必填检查：缺失与显式 null 不混淆（null 视为已提供）。
		req := make([]string, 0, len(s.required))
		for name := range s.required {
			req = append(req, name)
		}
		sort.Strings(req)
		for _, name := range req {
			if _, present := obj[name]; !present {
				addErr(errs, ipath, joinPtr(s.path, "required"),
					fmt.Sprintf("missing required property %q", name))
			}
		}
		// 属性校验：按属性名排序保证错误顺序稳定。
		names := make([]string, 0, len(obj))
		for name := range obj {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if sub, ok := s.properties[name]; ok {
				validateNode(sub, obj[name], joinPtr(ipath, name), errs)
			} else if s.additionalProperties != nil {
				if !s.additionalAllowed {
					addErr(errs, joinPtr(ipath, name), joinPtr(s.path, "additionalProperties"),
						fmt.Sprintf("additional property %q is not allowed", name))
				} else {
					validateNode(s.additionalProperties, obj[name], joinPtr(ipath, name), errs)
				}
			}
		}
	}

	if arr, ok := inst.([]any); ok {
		n := int64(len(arr))
		if s.minItems != nil && n < *s.minItems {
			addErr(errs, ipath, joinPtr(s.path, "minItems"),
				fmt.Sprintf("array length %d is less than minItems %d", n, *s.minItems))
		}
		if s.maxItems != nil && n > *s.maxItems {
			addErr(errs, ipath, joinPtr(s.path, "maxItems"),
				fmt.Sprintf("array length %d exceeds maxItems %d", n, *s.maxItems))
		}
		if s.items != nil {
			for i, item := range arr {
				validateNode(s.items, item, joinPtr(ipath, fmt.Sprint(i)), errs)
			}
		}
	}

	// 组合：结果不依赖分支顺序；成功组合中的失败分支不计入错误。
	for _, sub := range s.allOf {
		validateNode(sub, inst, ipath, errs)
	}
	if len(s.anyOf) > 0 {
		matched := false
		for _, sub := range s.anyOf {
			if len(Validate(sub, inst)) == 0 {
				matched = true
				break
			}
		}
		if !matched {
			addErr(errs, ipath, joinPtr(s.path, "anyOf"), "instance matches none of the anyOf branches")
		}
	}
	if len(s.oneOf) > 0 {
		matched := 0
		for _, sub := range s.oneOf {
			if len(Validate(sub, inst)) == 0 {
				matched++
			}
		}
		if matched != 1 {
			addErr(errs, ipath, joinPtr(s.path, "oneOf"),
				fmt.Sprintf("instance matches %d oneOf branches, expected exactly 1", matched))
		}
	}
	if s.not != nil && len(Validate(s.not, inst)) == 0 {
		addErr(errs, ipath, joinPtr(s.path, "not"), "instance matches the forbidden not schema")
	}
}

func addErr(errs *[]*InstanceError, ipath, spath, msg string) {
	*errs = append(*errs, &InstanceError{InstancePath: ipath, SchemaPath: spath, Message: msg})
}

// typeMatches 判断实例是否符合声明类型。整数按数值判断，布尔不是数字。
func typeMatches(typ string, inst any) bool {
	switch typ {
	case "null":
		return inst == nil
	case "boolean":
		_, ok := inst.(bool)
		return ok
	case "string":
		_, ok := inst.(string)
		return ok
	case "array":
		_, ok := inst.([]any)
		return ok
	case "object":
		_, ok := inst.(map[string]any)
		return ok
	case "number":
		_, ok := inst.(json.Number)
		return ok
	case "integer":
		n, ok := inst.(json.Number)
		if !ok {
			return false
		}
		r, ok := toRat(n)
		return ok && r.IsInt()
	}
	return false
}

func typeName(inst any) string {
	switch inst.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return "unknown"
}

// inEnum 按 JSON 值结构比较；数值相等不受写法影响（1 与 1.0 相等）。
func inEnum(enum []any, inst any) bool {
	for _, e := range enum {
		if jsonEqual(e, inst) {
			return true
		}
	}
	return false
}

func jsonEqual(a, b any) bool {
	// 数值：按有理数比较。
	if na, ok := a.(json.Number); ok {
		nb, ok := b.(json.Number)
		if !ok {
			return false
		}
		ra, oka := toRat(na)
		rb, okb := toRat(nb)
		return oka && okb && ra.Cmp(rb) == 0
	}
	switch va := a.(type) {
	case nil:
		return b == nil
	case bool:
		vb, ok := b.(bool)
		return ok && va == vb
	case string:
		vb, ok := b.(string)
		return ok && va == vb
	case []any:
		vb, ok := b.([]any)
		if !ok || len(va) != len(vb) {
			return false
		}
		for i := range va {
			if !jsonEqual(va[i], vb[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		vb, ok := b.(map[string]any)
		if !ok || len(va) != len(vb) {
			return false
		}
		for k, av := range va {
			bv, ok := vb[k]
			if !ok || !jsonEqual(av, bv) {
				return false
			}
		}
		return true
	}
	return false
}
