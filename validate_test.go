package main

import (
	"strings"
	"testing"
)

func mustCompile(t *testing.T, schema string) *Schema {
	t.Helper()
	s, errs := CompileSchema([]byte(schema))
	if len(errs) > 0 {
		t.Fatalf("unexpected schema errors: %v", errs)
	}
	return s
}

func mustFailCompile(t *testing.T, schema string) []*SchemaError {
	t.Helper()
	_, errs := CompileSchema([]byte(schema))
	if len(errs) == 0 {
		t.Fatalf("expected schema errors, got none")
	}
	return errs
}

func decodeInst(t *testing.T, s string) any {
	t.Helper()
	inst, err := decodeJSON(s)
	if err != nil {
		t.Fatalf("bad test instance: %v", err)
	}
	return inst
}

func assertValid(t *testing.T, schema, inst string) {
	t.Helper()
	s := mustCompile(t, schema)
	if errs := Validate(s, decodeInst(t, inst)); len(errs) > 0 {
		t.Fatalf("expected valid, got %v", errs)
	}
}

func assertInvalid(t *testing.T, schema, inst string) []*InstanceError {
	t.Helper()
	s := mustCompile(t, schema)
	errs := Validate(s, decodeInst(t, inst))
	if len(errs) == 0 {
		t.Fatalf("expected invalid")
	}
	return errs
}

func TestTypes(t *testing.T) {
	assertValid(t, `{"type":"integer"}`, `3`)
	assertValid(t, `{"type":"integer"}`, `3.0`) // 整数按数值判断
	assertInvalid(t, `{"type":"integer"}`, `3.5`)
	assertInvalid(t, `{"type":"integer"}`, `true`) // 布尔不是数字
	assertInvalid(t, `{"type":"number"}`, `false`)
	assertValid(t, `{"type":"number"}`, `1e3`)
	assertValid(t, `{"type":"null"}`, `null`)
	assertValid(t, `{"type":"boolean"}`, `true`)
	assertValid(t, `{"type":"string"}`, `"x"`)
	assertValid(t, `{"type":"array"}`, `[]`)
	assertValid(t, `{"type":"object"}`, `{}`)
}

func TestBooleanSchema(t *testing.T) {
	assertValid(t, `true`, `{"anything":1}`)
	assertInvalid(t, `false`, `null`)
	assertValid(t, `{"properties":{"a":false}}`, `{}`)
	assertInvalid(t, `{"properties":{"a":false}}`, `{"a":1}`)
}

func TestNumericBounds(t *testing.T) {
	assertValid(t, `{"minimum":2,"maximum":5}`, `3`)
	assertValid(t, `{"minimum":2}`, `2`)
	assertInvalid(t, `{"minimum":2}`, `1.999`)
	assertInvalid(t, `{"maximum":5}`, `6`)
	assertInvalid(t, `{"exclusiveMinimum":2}`, `2`)
	assertValid(t, `{"exclusiveMaximum":5}`, `4.999`)
	assertInvalid(t, `{"exclusiveMaximum":5}`, `5`)
	// 写法不同不影响边界比较
	assertValid(t, `{"minimum":2.0}`, `2`)
}

func TestStringLengthRunes(t *testing.T) {
	schema := `{"minLength":2,"maxLength":3}`
	assertValid(t, schema, `"汉字"`) // 2 个码点
	assertValid(t, schema, `"éé"`) // 组合字符外的普通多字节
	assertValid(t, schema, `"abc"`)
	assertInvalid(t, schema, `"汉"`)
	assertInvalid(t, schema, `"汉字串长"`)
}

func TestEnumStructural(t *testing.T) {
	schema := `{"enum":[1,"a",[1,2],{"x":1},null,true]}`
	assertValid(t, schema, `1.0`) // 数值相等不因写法失败
	assertValid(t, schema, `[1,2]`)
	assertValid(t, schema, `{"x":1.0}`)
	assertValid(t, schema, `null`)
	assertInvalid(t, schema, `2`)
	assertInvalid(t, schema, `[2,1]`)
	assertInvalid(t, schema, `{"x":1,"y":2}`)
	assertInvalid(t, schema, `"1"`) // 字符串 "1" 不等于数字 1
}

func TestConst(t *testing.T) {
	assertValid(t, `{"const":{"a":[1,2]}}`, `{"a":[1.0,2]}`)
	assertInvalid(t, `{"const":1}`, `2`)
}

func TestObjectRules(t *testing.T) {
	schema := `{
		"type":"object",
		"properties":{"name":{"type":"string"},"age":{"type":"integer","minimum":0}},
		"required":["name"],
		"additionalProperties":false
	}`
	assertValid(t, schema, `{"name":"n","age":3}`)
	errs := assertInvalid(t, schema, `{"age":3}`)
	if !strings.Contains(errs[0].Message, "missing required") {
		t.Fatalf("expected missing required, got %v", errs[0])
	}
	// 显式 null 不等于缺失：required 通过，但 type 约束仍会拒绝。
	errs = assertInvalid(t, schema, `{"name":null}`)
	for _, e := range errs {
		if strings.Contains(e.Message, "missing required") {
			t.Fatalf("null must not be reported as missing: %v", e)
		}
	}
	assertInvalid(t, schema, `{"name":"n","extra":1}`)
	assertInvalid(t, schema, `{"name":"n","age":-1}`)
	// 错误定位
	errs = assertInvalid(t, schema, `{"name":"n","age":"x"}`)
	if errs[0].InstancePath != "/age" {
		t.Fatalf("expected instance path /age, got %q", errs[0].InstancePath)
	}
}

func TestAdditionalPropertiesSchema(t *testing.T) {
	schema := `{"properties":{"a":{}},"additionalProperties":{"type":"integer"}}`
	assertValid(t, schema, `{"a":"x","b":2}`)
	assertInvalid(t, schema, `{"a":"x","b":"y"}`)
}

func TestArrayRules(t *testing.T) {
	schema := `{"items":{"type":"integer"},"minItems":1,"maxItems":3}`
	assertValid(t, schema, `[1,2]`)
	assertInvalid(t, schema, `[]`)
	assertInvalid(t, schema, `[1,2,3,4]`)
	errs := assertInvalid(t, schema, `[1,"x"]`)
	if errs[0].InstancePath != "/1" {
		t.Fatalf("expected /1, got %q", errs[0].InstancePath)
	}
}
