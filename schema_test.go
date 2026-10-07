package main

import (
	"strings"
	"testing"
)

func expectSchemaError(t *testing.T, schemaJSON, wantSub string) {
	t.Helper()
	raw, err := decodeJSON([]byte(schemaJSON))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	_, err = CompileSchema(raw)
	if err == nil {
		t.Fatalf("expected schema error containing %q, got nil", wantSub)
	}
	if !strings.Contains(err.Error(), wantSub) {
		t.Fatalf("expected schema error containing %q, got %v", wantSub, err)
	}
}

func TestUnsupportedKeyword(t *testing.T) {
	expectSchemaError(t, `{"pattern":"^a"}`, "unsupported keyword")
	expectSchemaError(t, `{"properties":{"a":{"format":"email"}}}`, "unsupported keyword")
}

func TestBadKeywordShapes(t *testing.T) {
	expectSchemaError(t, `{"type":"text"}`, "type must be one of")
	expectSchemaError(t, `{"minimum":"1"}`, "minimum must be a number")
	expectSchemaError(t, `{"minLength":-1}`, "non-negative integer")
	expectSchemaError(t, `{"required":"a"}`, "array of strings")
	expectSchemaError(t, `{"allOf":[]}`, "non-empty array")
}

func TestRefResolutionAndEscapes(t *testing.T) {
	schema := `{
		"$defs": {
			"a/b": {"type":"integer"},
			"tilde~key": {"type":"string"}
		},
		"properties": {
			"x": {"$ref":"#/$defs/a~1b"},
			"y": {"$ref":"#/$defs/tilde~0key"}
		}
	}`
	expectOK(t, schema, `{"x":1,"y":"s"}`)
	expectReject(t, schema, `{"x":"s"}`, "expected type integer")
	expectReject(t, schema, `{"y":1}`, "expected type string")
}

func TestMissingRefRejected(t *testing.T) {
	expectSchemaError(t, `{"$ref":"#/$defs/nope"}`, "does not resolve")
	expectSchemaError(t, `{"$ref":"http://example.com/s.json"}`, "only local file references")
}

func TestCircularRefRejected(t *testing.T) {
	expectSchemaError(t, `{"$defs":{"a":{"$ref":"#/$defs/a"}},"$ref":"#/$defs/a"}`, "circular reference")
	expectSchemaError(t, `{"$defs":{"a":{"$ref":"#/$defs/b"},"b":{"$ref":"#/$defs/a"}},"$ref":"#/$defs/a"}`, "circular reference")
	// self-referencing root
	expectSchemaError(t, `{"$ref":"#"}`, "circular reference")
}

func TestRefSiblingsStillApply(t *testing.T) {
	schema := `{"$defs":{"n":{"type":"integer"}},"$ref":"#/$defs/n","minimum":10}`
	expectOK(t, schema, `12`)
	expectReject(t, schema, `5`, "below minimum")
	expectReject(t, schema, `"s"`, "expected type integer")
}

func TestRefInsideCombinations(t *testing.T) {
	schema := `{
		"$defs": {
			"pos": {"type":"integer","minimum":1},
			"neg": {"type":"integer","maximum":-1}
		},
		"oneOf": [{"$ref":"#/$defs/pos"},{"$ref":"#/$defs/neg"}]
	}`
	expectOK(t, schema, `3`)
	expectOK(t, schema, `-3`)
	expectReject(t, schema, `0`, "exactly 1")
	expectReject(t, schema, `"x"`, "exactly 1")
}

func TestCombinationInsideRef(t *testing.T) {
	schema := `{
		"$defs": {
			"name": {"allOf":[{"type":"string"},{"minLength":2}]}
		},
		"properties": {"n": {"$ref":"#/$defs/name"}}
	}`
	expectOK(t, schema, `{"n":"ab"}`)
	expectReject(t, schema, `{"n":"a"}`, "below minLength")
	expectReject(t, schema, `{"n":1}`, "expected type string")
}

func TestErrorPathsStable(t *testing.T) {
	schema := `{"properties":{"b":{"type":"integer"},"a":{"type":"integer"}},"additionalProperties":false}`
	errs1 := validate(t, schema, `{"b":"x","a":"y","z":1}`)
	errs2 := validate(t, schema, `{"b":"x","a":"y","z":1}`)
	if len(errs1) != 3 {
		t.Fatalf("expected 3 errors, got %v", errs1)
	}
	for i := range errs1 {
		if errs1[i] != errs2[i] {
			t.Fatalf("unstable error order: %v vs %v", errs1, errs2)
		}
	}
	if errs1[0].InstancePath != "/a" || errs1[1].InstancePath != "/b" || errs1[2].InstancePath != "/z" {
		t.Fatalf("unexpected error order: %v", errs1)
	}
	if !strings.Contains(errs1[0].SchemaPath, "#/properties/a/type") {
		t.Fatalf("unexpected schema path: %v", errs1[0])
	}
}
