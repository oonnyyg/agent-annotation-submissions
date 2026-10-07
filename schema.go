package main

import (
	"fmt"
	"strconv"
	"strings"
)

// supportedKeywords is the explicit subset this tool implements.
var supportedKeywords = map[string]bool{
	"$ref": true, "$defs": true, "definitions": true,
	"type": true, "enum": true,
	"minimum": true, "maximum": true,
	"minLength": true, "maxLength": true,
	"properties": true, "required": true, "additionalProperties": true,
	"items": true, "minItems": true, "maxItems": true,
	"allOf": true, "anyOf": true, "oneOf": true, "not": true,
}

var validTypes = map[string]bool{
	"null": true, "boolean": true, "object": true,
	"array": true, "number": true, "integer": true, "string": true,
}

// SchemaError describes a problem in the schema itself.
type SchemaError struct {
	Path string // JSON Pointer to the offending schema node
	Msg  string
}

func (e *SchemaError) Error() string {
	return fmt.Sprintf("schema error at %s: %s", e.Path, e.Msg)
}

// Schema holds a decoded, structure-checked schema document.
type Schema struct {
	root interface{}
}

// CompileSchema decodes and validates the structure of a schema document.
func CompileSchema(root interface{}) (*Schema, error) {
	s := &Schema{root: root}
	if err := s.checkNode(root, "#", nil); err != nil {
		return nil, err
	}
	return s, nil
}

// checkNode validates the structure of one schema node, following $ref
// chains to reject missing references and cycles before any data is seen.
func (s *Schema) checkNode(node interface{}, path string, refStack []string) error {
	switch n := node.(type) {
	case bool:
		return nil // boolean schema: always accept / always reject
	case map[string]interface{}:
	default:
		_ = n
		return &SchemaError{path, "schema must be an object or boolean"}
	}
	m := node.(map[string]interface{})
	for k := range m {
		if !supportedKeywords[k] {
			return &SchemaError{path + "/" + escapePointer(k), "unsupported keyword " + strconv.Quote(k)}
		}
	}
	if ref, ok := m["$ref"]; ok {
		refStr, ok := ref.(string)
		if !ok {
			return &SchemaError{path + "/$ref", "$ref must be a string"}
		}
		if !strings.HasPrefix(refStr, "#") {
			return &SchemaError{path + "/$ref", "only local file references (#...) are supported"}
		}
		for _, seen := range refStack {
			if seen == refStr {
				return &SchemaError{path + "/$ref", "circular reference " + refStr}
			}
		}
		target, err := s.resolveRef(refStr)
		if err != nil {
			return &SchemaError{path + "/$ref", err.Error()}
		}
		if err := s.checkNode(target, refStr, append(refStack, refStr)); err != nil {
			return err
		}
	}
	return s.checkKeywords(m, path, refStack)
}

// checkKeywords validates per-keyword shapes and recurses into subschemas.
func (s *Schema) checkKeywords(m map[string]interface{}, path string, refStack []string) error {
	sub := func(keyword string) error {
		v, ok := m[keyword]
		if !ok {
			return nil
		}
		return s.checkNode(v, path+"/"+keyword, refStack)
	}
	subList := func(keyword string) error {
		v, ok := m[keyword]
		if !ok {
			return nil
		}
		list, ok := v.([]interface{})
		if !ok || len(list) == 0 {
			return &SchemaError{path + "/" + keyword, keyword + " must be a non-empty array of schemas"}
		}
		for i, item := range list {
			if err := s.checkNode(item, path+"/"+keyword+"/"+strconv.Itoa(i), refStack); err != nil {
				return err
			}
		}
		return nil
	}
	if v, ok := m["type"]; ok {
		t, ok := v.(string)
		if !ok || !validTypes[t] {
			return &SchemaError{path + "/type", "type must be one of null, boolean, object, array, number, integer, string"}
		}
	}
	if v, ok := m["enum"]; ok {
		list, ok := v.([]interface{})
		if !ok || len(list) == 0 {
			return &SchemaError{path + "/enum", "enum must be a non-empty array"}
		}
	}
	for _, k := range []string{"minimum", "maximum"} {
		if v, ok := m[k]; ok {
			if _, ok := v.(jsonNumber); !ok {
				return &SchemaError{path + "/" + k, k + " must be a number"}
			}
		}
	}
	for _, k := range []string{"minLength", "maxLength", "minItems", "maxItems"} {
		if v, ok := m[k]; ok {
			n, ok := v.(jsonNumber)
			if !ok || !isIntegerNumber(n) || numberSign(n) < 0 {
				return &SchemaError{path + "/" + k, k + " must be a non-negative integer"}
			}
		}
	}
	if v, ok := m["properties"]; ok {
		props, ok := v.(map[string]interface{})
		if !ok {
			return &SchemaError{path + "/properties", "properties must be an object"}
		}
		for name, subschema := range props {
			if err := s.checkNode(subschema, path+"/properties/"+escapePointer(name), refStack); err != nil {
				return err
			}
		}
	}
	if v, ok := m["required"]; ok {
		list, ok := v.([]interface{})
		if !ok {
			return &SchemaError{path + "/required", "required must be an array of strings"}
		}
		seen := map[string]bool{}
		for i, item := range list {
			name, ok := item.(string)
			if !ok {
				return &SchemaError{path + "/required/" + strconv.Itoa(i), "required entries must be strings"}
			}
			if seen[name] {
				return &SchemaError{path + "/required", "duplicate required entry " + strconv.Quote(name)}
			}
			seen[name] = true
		}
	}
	if v, ok := m["additionalProperties"]; ok {
		if _, ok := v.(bool); !ok {
			return &SchemaError{path + "/additionalProperties", "additionalProperties must be a boolean"}
		}
	}
	for _, k := range []string{"items", "not"} {
		if err := sub(k); err != nil {
			return err
		}
	}
	for _, k := range []string{"allOf", "anyOf", "oneOf"} {
		if err := subList(k); err != nil {
			return err
		}
	}
	for _, k := range []string{"$defs", "definitions"} {
		if v, ok := m[k]; ok {
			defs, ok := v.(map[string]interface{})
			if !ok {
				return &SchemaError{path + "/" + k, k + " must be an object"}
			}
			for name, subschema := range defs {
				if err := s.checkNode(subschema, path+"/"+k+"/"+escapePointer(name), refStack); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// resolveRef resolves a local JSON Pointer reference against the root.
func (s *Schema) resolveRef(ref string) (interface{}, error) {
	if ref == "#" {
		return s.root, nil
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("unsupported reference %s", ref)
	}
	current := s.root
	for _, raw := range strings.Split(ref[2:], "/") {
		token := unescapePointer(raw)
		obj, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("reference %s does not resolve", ref)
		}
		next, ok := obj[token]
		if !ok {
			return nil, fmt.Errorf("reference %s does not resolve", ref)
		}
		current = next
	}
	return current, nil
}

func escapePointer(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}

func unescapePointer(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
}
