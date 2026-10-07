package main

import (
	"strings"
	"testing"
)

func compile(t *testing.T, schemaJSON string) *Schema {
	t.Helper()
	raw, err := decodeJSON([]byte(schemaJSON))
	if err != nil {
		t.Fatalf("schema JSON decode: %v", err)
	}
	s, err := CompileSchema(raw)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return s
}

func validate(t *testing.T, schemaJSON, instJSON string) []InstanceError {
	t.Helper()
	s := compile(t, schemaJSON)
	inst, err := decodeJSON([]byte(instJSON))
	if err != nil {
		t.Fatalf("instance JSON decode: %v", err)
	}
	return s.Validate(inst)
}

func expectOK(t *testing.T, schemaJSON, instJSON string) {
	t.Helper()
	if errs := validate(t, schemaJSON, instJSON); len(errs) != 0 {
		t.Fatalf("expected OK, got %v", errs)
	}
}

func expectReject(t *testing.T, schemaJSON, instJSON, wantSub string) {
	t.Helper()
	errs := validate(t, schemaJSON, instJSON)
	if len(errs) == 0 {
		t.Fatalf("expected rejection containing %q, got OK", wantSub)
	}
	for _, e := range errs {
		if strings.Contains(e.String(), wantSub) {
			return
		}
	}
	t.Fatalf("expected error containing %q, got %v", wantSub, errs)
}

func TestTypes(t *testing.T) {
	expectOK(t, `{"type":"integer"}`, `3`)
	expectOK(t, `{"type":"integer"}`, `3.0`)
	expectReject(t, `{"type":"integer"}`, `3.5`, "expected type integer")
	expectReject(t, `{"type":"number"}`, `true`, "expected type number")
	expectOK(t, `{"type":"boolean"}`, `false`)
	expectReject(t, `{"type":"string"}`, `1`, "expected type string")
}

func TestEnumNumericEquality(t *testing.T) {
	expectOK(t, `{"enum":[1,"a",{"k":[2]}]}`, `1.0`)
	expectOK(t, `{"enum":[1,"a",{"k":[2]}]}`, `{"k":[2.0]}`)
	expectReject(t, `{"enum":[1]}`, `2`, "not in enum")
}

func TestNumberBounds(t *testing.T) {
	expectOK(t, `{"minimum":2,"maximum":5}`, `2`)
	expectOK(t, `{"minimum":2,"maximum":5}`, `5`)
	expectReject(t, `{"minimum":2}`, `1.5`, "below minimum")
	expectReject(t, `{"maximum":5}`, `6`, "above maximum")
}

func TestStringLengthRunes(t *testing.T) {
	expectOK(t, `{"minLength":2,"maxLength":3}`, `"你好"`)
	expectReject(t, `{"minLength":2}`, `"é"`, "below minLength")
	expectReject(t, `{"maxLength":1}`, `"ab"`, "exceeds maxLength")
}

func TestObjectProps(t *testing.T) {
	schema := `{"type":"object","properties":{"a":{"type":"integer"}},"required":["a"],"additionalProperties":false}`
	expectOK(t, schema, `{"a":1}`)
	expectReject(t, schema, `{"b":1}`, "missing required property")
	expectReject(t, schema, `{"a":1,"b":2}`, "additional property")
	// explicit null is not the same as missing
	expectOK(t, `{"required":["a"]}`, `{"a":null}`)
	expectReject(t, `{"required":["a"],"properties":{"a":{"type":"string"}}}`, `{"a":null}`, "expected type string")
}

func TestArray(t *testing.T) {
	schema := `{"items":{"type":"string"},"minItems":1,"maxItems":2}`
	expectOK(t, schema, `["x"]`)
	expectReject(t, schema, `[]`, "below minItems")
	expectReject(t, schema, `["a","b","c"]`, "exceeds maxItems")
	expectReject(t, schema, `["a",1]`, "expected type string")
}

func TestCombinations(t *testing.T) {
	expectOK(t, `{"allOf":[{"type":"integer"},{"minimum":1}]}`, `2`)
	expectReject(t, `{"allOf":[{"type":"integer"},{"minimum":1}]}`, `0`, "below minimum")
	expectOK(t, `{"anyOf":[{"type":"string"},{"type":"integer"}]}`, `1`)
	expectReject(t, `{"anyOf":[{"type":"string"},{"type":"integer"}]}`, `true`, "anyOf")
	expectOK(t, `{"oneOf":[{"type":"integer"},{"minimum":10}]}`, `5`)
	expectReject(t, `{"oneOf":[{"type":"integer"},{"minimum":10}]}`, `15`, "exactly 1")
	expectOK(t, `{"not":{"type":"string"}}`, `1`)
	expectReject(t, `{"not":{"type":"string"}}`, `"x"`, "not")
}

func TestSuccessfulCombinationHidesBranchFailures(t *testing.T) {
	// anyOf succeeds via second branch; first branch's failure must not leak.
	if errs := validate(t, `{"anyOf":[{"type":"string"},{"type":"integer"}]}`, `1`); len(errs) != 0 {
		t.Fatalf("branch failure leaked: %v", errs)
	}
	// oneOf: failing branches in a successful combination are not reported.
	if errs := validate(t, `{"oneOf":[{"type":"string"},{"type":"integer"},{"type":"boolean"}]}`, `1`); len(errs) != 0 {
		t.Fatalf("branch failure leaked: %v", errs)
	}
}

func TestBooleanSchemas(t *testing.T) {
	expectOK(t, `true`, `{"anything":1}`)
	expectReject(t, `false`, `1`, "boolean schema false")
	expectReject(t, `{"properties":{"a":false}}`, `{"a":1}`, "boolean schema false")
}
