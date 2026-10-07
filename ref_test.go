package main

import (
	"strings"
	"testing"
)

func TestLocalRef(t *testing.T) {
	schema := `{
		"$defs":{"pos":{"type":"integer","minimum":1}},
		"properties":{"count":{"$ref":"#/$defs/pos"}}
	}`
	assertValid(t, schema, `{"count":5}`)
	errs := assertInvalid(t, schema, `{"count":0}`)
	if !strings.Contains(errs[0].SchemaPath, "/$defs/pos") {
		t.Fatalf("expected schema path into $defs, got %q", errs[0].SchemaPath)
	}
}

func TestRefPointerEscapes(t *testing.T) {
	// 键名包含 ~ 和 /，验证 RFC 6901 转义。
	schema := `{
		"$defs":{"a/b":{"type":"string"},"x~y":{"type":"integer"}},
		"properties":{
			"p":{"$ref":"#/$defs/a~1b"},
			"q":{"$ref":"#/$defs/x~0y"}
		}
	}`
	assertValid(t, schema, `{"p":"s","q":1}`)
	assertInvalid(t, schema, `{"p":1}`)
	assertInvalid(t, schema, `{"q":"s"}`)
}

func TestRefMissing(t *testing.T) {
	errs := mustFailCompile(t, `{"$ref":"#/$defs/nope","$defs":{}}`)
	if !strings.Contains(errs[0].Reason, "unresolvable") {
		t.Fatalf("expected unresolvable ref, got %v", errs[0])
	}
}

func TestRefRemoteRejected(t *testing.T) {
	errs := mustFailCompile(t, `{"$ref":"https://example.com/s.json"}`)
	if !strings.Contains(errs[0].Reason, "remote") {
		t.Fatalf("expected remote rejection, got %v", errs[0])
	}
}

func TestRefCircular(t *testing.T) {
	cases := []string{
		`{"$ref":"#"}`,
		`{"$defs":{"a":{"$ref":"#/$defs/a"}},"$ref":"#/$defs/a"}`,
		`{"$defs":{"a":{"$ref":"#/$defs/b"},"b":{"$ref":"#/$defs/a"}},"$ref":"#/$defs/a"}`,
	}
	for i, c := range cases {
		errs := mustFailCompile(t, c)
		found := false
		for _, e := range errs {
			if strings.Contains(e.Reason, "circular") {
				found = true
			}
		}
		if !found {
			t.Fatalf("case %d: expected circular ref error, got %v", i, errs)
		}
	}
}

func TestRefSiblingsApply(t *testing.T) {
	// $ref 旁边的约束同时生效。
	schema := `{
		"$defs":{"str":{"type":"string"}},
		"properties":{"s":{"$ref":"#/$defs/str","minLength":3}}
	}`
	assertValid(t, schema, `{"s":"abc"}`)
	assertInvalid(t, schema, `{"s":1}`)    // 违反 ref 目标
	assertInvalid(t, schema, `{"s":"ab"}`) // 违反兄弟约束
}

func TestRefSharedNoFalseCycle(t *testing.T) {
	// 两个属性引用同一定义：合法，不是环。
	schema := `{
		"$defs":{"n":{"type":"integer"}},
		"properties":{"a":{"$ref":"#/$defs/n"},"b":{"$ref":"#/$defs/n"}}
	}`
	assertValid(t, schema, `{"a":1,"b":2}`)
}

func TestCombinators(t *testing.T) {
	// allOf
	all := `{"allOf":[{"type":"integer"},{"minimum":2}]}`
	assertValid(t, all, `3`)
	assertInvalid(t, all, `1`)
	assertInvalid(t, all, `"x"`)

	// anyOf
	any := `{"anyOf":[{"type":"string"},{"type":"integer"}]}`
	assertValid(t, any, `"s"`)
	assertValid(t, any, `1`)
	assertInvalid(t, any, `true`)

	// oneOf
	one := `{"oneOf":[{"type":"integer"},{"minimum":2}]}`
	assertInvalid(t, one, `3`) // 两个分支都满足
	assertValid(t, one, `1`)   // 仅第一个满足
	assertValid(t, one, `2.5`) // 仅第二个满足（数值约束不适用于非数字）
	assertInvalid(t, `{"oneOf":[{"type":"integer"},{"type":"string"}]}`, `true`)

	// not
	not := `{"not":{"type":"string"}}`
	assertValid(t, not, `1`)
	assertInvalid(t, not, `"s"`)
}

func TestCombinatorOrderIndependence(t *testing.T) {
	a := `{"oneOf":[{"type":"integer"},{"minimum":2}]}`
	b := `{"oneOf":[{"minimum":2},{"type":"integer"}]}`
	for _, inst := range []string{`1`, `2`, `3`, `"x"`} {
		sa, sb := mustCompile(t, a), mustCompile(t, b)
		ia := len(Validate(sa, decodeInst(t, inst))) == 0
		ib := len(Validate(sb, decodeInst(t, inst))) == 0
		if ia != ib {
			t.Fatalf("order changed result for %s: %v vs %v", inst, ia, ib)
		}
	}
}

func TestSuccessfulCombinationHidesBranchErrors(t *testing.T) {
	// anyOf 成功时，失败分支的错误不得出现在结果中。
	s := mustCompile(t, `{"anyOf":[{"type":"string"},{"type":"integer"}]}`)
	if errs := Validate(s, decodeInst(t, `1`)); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	// oneOf 恰好一个成功时同样干净。
	s = mustCompile(t, `{"oneOf":[{"type":"string"},{"type":"integer"}]}`)
	if errs := Validate(s, decodeInst(t, `1`)); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestRefInsideCombinator(t *testing.T) {
	// 引用与组合交叉：anyOf 分支是 $ref。
	schema := `{
		"$defs":{"name":{"type":"string","minLength":2}},
		"anyOf":[
			{"$ref":"#/$defs/name"},
			{"type":"integer","minimum":10}
		]
	}`
	assertValid(t, schema, `"ab"`)
	assertValid(t, schema, `15`)
	assertInvalid(t, schema, `"a"`)
	assertInvalid(t, schema, `5`)
}

func TestNotWithRef(t *testing.T) {
	schema := `{
		"$defs":{"bad":{"enum":["admin","root"]}},
		"not":{"$ref":"#/$defs/bad"}
	}`
	assertValid(t, schema, `"user"`)
	assertInvalid(t, schema, `"admin"`)
}

func TestUnsupportedKeyword(t *testing.T) {
	errs := mustFailCompile(t, `{"type":"string","pattern":"^a","format":"email"}`)
	if len(errs) != 2 {
		t.Fatalf("expected 2 unsupported keyword errors, got %v", errs)
	}
	for _, e := range errs {
		if !strings.Contains(e.Reason, "unsupported keyword") {
			t.Fatalf("unexpected error: %v", e)
		}
	}
}

func TestSchemaStructureErrors(t *testing.T) {
	mustFailCompile(t, `{"type":["string","null"]}`) // 类型数组不支持
	mustFailCompile(t, `{"type":"str"}`)             // 未知类型
	mustFailCompile(t, `{"minimum":"x"}`)            // 非数值边界
	mustFailCompile(t, `{"minimum":5,"maximum":1}`)  // 空区间
	mustFailCompile(t, `{"minLength":-1}`)           // 负长度
	mustFailCompile(t, `{"items":[{}]}`)             // 元组形式不支持
	mustFailCompile(t, `{"enum":[]}`)                // 空枚举
	mustFailCompile(t, `{"exclusiveMinimum":true}`)  // 旧式布尔形式不支持
	mustFailCompile(t, `42`)                         // 模式必须是对象或布尔
}

func TestEnumNumericRepresentation(t *testing.T) {
	// 数值相等不因写法不同而失败：1、1.0、1e0 视为同一值。
	s := mustCompile(t, `{"enum":[1]}`)
	for _, inst := range []string{`1`, `1.0`, `1e0`, `1.00`} {
		if errs := Validate(s, decodeInst(t, inst)); len(errs) != 0 {
			t.Fatalf("instance %s should match enum [1]", inst)
		}
	}
}
