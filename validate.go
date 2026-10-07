package main

import (
	"fmt"
	"strconv"
)

// InstanceError is one reason an instance document was rejected.
type InstanceError struct {
	InstancePath string // JSON Pointer into the instance
	SchemaPath   string // JSON Pointer into the schema
	Msg          string
}

func (e InstanceError) String() string {
	return fmt.Sprintf("instance %s: %s (schema %s)", e.InstancePath, e.Msg, e.SchemaPath)
}

// Validate checks one decoded instance against the compiled schema.
func (s *Schema) Validate(instance interface{}) []InstanceError {
	var errs []InstanceError
	s.validateNode(s.root, instance, "", "#", &errs)
	return errs
}

func (s *Schema) validateNode(node interface{}, inst interface{}, instPath, schemaPath string, errs *[]InstanceError) {
	switch n := node.(type) {
	case bool:
		if !n {
			s.fail(errs, instPath, schemaPath, "not allowed by boolean schema false")
		}
		return
	case map[string]interface{}:
		s.validateObject(n, inst, instPath, schemaPath, errs)
	}
}

func (s *Schema) fail(errs *[]InstanceError, instPath, schemaPath, msg string) {
	if instPath == "" {
		instPath = "/"
	}
	*errs = append(*errs, InstanceError{InstancePath: instPath, SchemaPath: schemaPath, Msg: msg})
}

func (s *Schema) validateObject(m map[string]interface{}, inst interface{}, instPath, schemaPath string, errs *[]InstanceError) {
	if ref, ok := m["$ref"].(string); ok {
		target, err := s.resolveRef(ref)
		if err == nil {
			s.validateNode(target, inst, instPath, ref, errs)
		}
		// sibling keywords still apply; fall through
	}
	if t, ok := m["type"].(string); ok && !typeMatches(t, inst) {
		s.fail(errs, instPath, schemaPath+"/type", "expected type "+t)
	}
	if list, ok := m["enum"].([]interface{}); ok {
		found := false
		for _, v := range list {
			if jsonEqual(v, inst) {
				found = true
				break
			}
		}
		if !found {
			s.fail(errs, instPath, schemaPath+"/enum", "value is not in enum")
		}
	}
	s.validateNumber(m, inst, instPath, schemaPath, errs)
	s.validateString(m, inst, instPath, schemaPath, errs)
	s.validateObjectProps(m, inst, instPath, schemaPath, errs)
	s.validateArray(m, inst, instPath, schemaPath, errs)
	s.validateCombinations(m, inst, instPath, schemaPath, errs)
}

func typeMatches(t string, inst interface{}) bool {
	switch t {
	case "null":
		return inst == nil
	case "boolean":
		_, ok := inst.(bool)
		return ok
	case "string":
		_, ok := inst.(string)
		return ok
	case "object":
		_, ok := inst.(map[string]interface{})
		return ok
	case "array":
		_, ok := inst.([]interface{})
		return ok
	case "number":
		_, ok := inst.(jsonNumber)
		return ok
	case "integer":
		n, ok := inst.(jsonNumber)
		return ok && isIntegerNumber(n)
	}
	return false
}

func (s *Schema) validateNumber(m map[string]interface{}, inst interface{}, instPath, schemaPath string, errs *[]InstanceError) {
	n, ok := inst.(jsonNumber)
	if !ok {
		return
	}
	nv, _ := numberRat(n)
	if bound, ok := m["minimum"].(jsonNumber); ok {
		if bv, _ := numberRat(bound); nv.Cmp(bv) < 0 {
			s.fail(errs, instPath, schemaPath+"/minimum", "number is below minimum "+bound.String())
		}
	}
	if bound, ok := m["maximum"].(jsonNumber); ok {
		if bv, _ := numberRat(bound); nv.Cmp(bv) > 0 {
			s.fail(errs, instPath, schemaPath+"/maximum", "number is above maximum "+bound.String())
		}
	}
}

func (s *Schema) validateString(m map[string]interface{}, inst interface{}, instPath, schemaPath string, errs *[]InstanceError) {
	str, ok := inst.(string)
	if !ok {
		return
	}
	length := runeLen(str)
	if v, ok := m["minLength"].(jsonNumber); ok {
		if limit, _ := numberRat(v); int64(length) < limit.Num().Int64() {
			s.fail(errs, instPath, schemaPath+"/minLength", fmt.Sprintf("string length %d is below minLength %s", length, v.String()))
		}
	}
	if v, ok := m["maxLength"].(jsonNumber); ok {
		if limit, _ := numberRat(v); int64(length) > limit.Num().Int64() {
			s.fail(errs, instPath, schemaPath+"/maxLength", fmt.Sprintf("string length %d exceeds maxLength %s", length, v.String()))
		}
	}
}

func (s *Schema) validateObjectProps(m map[string]interface{}, inst interface{}, instPath, schemaPath string, errs *[]InstanceError) {
	obj, ok := inst.(map[string]interface{})
	if !ok {
		return
	}
	if list, ok := m["required"].([]interface{}); ok {
		for _, item := range list {
			name := item.(string)
			if _, present := obj[name]; !present {
				s.fail(errs, instPath, schemaPath+"/required", "missing required property "+strconv.Quote(name))
			}
		}
	}
	props, _ := m["properties"].(map[string]interface{})
	additional, allowAdditional := m["additionalProperties"].(bool)
	for _, name := range sortedKeys(obj) {
		value := obj[name]
		if subschema, known := props[name]; known {
			s.validateNode(subschema, value, instPath+"/"+escapePointer(name), schemaPath+"/properties/"+escapePointer(name), errs)
			continue
		}
		if allowAdditional && !additional {
			s.fail(errs, instPath+"/"+escapePointer(name), schemaPath+"/additionalProperties", "additional property "+strconv.Quote(name)+" is not allowed")
		}
	}
}

func (s *Schema) validateArray(m map[string]interface{}, inst interface{}, instPath, schemaPath string, errs *[]InstanceError) {
	arr, ok := inst.([]interface{})
	if !ok {
		return
	}
	if v, ok := m["minItems"].(jsonNumber); ok {
		if limit, _ := numberRat(v); int64(len(arr)) < limit.Num().Int64() {
			s.fail(errs, instPath, schemaPath+"/minItems", fmt.Sprintf("array length %d is below minItems %s", len(arr), v.String()))
		}
	}
	if v, ok := m["maxItems"].(jsonNumber); ok {
		if limit, _ := numberRat(v); int64(len(arr)) > limit.Num().Int64() {
			s.fail(errs, instPath, schemaPath+"/maxItems", fmt.Sprintf("array length %d exceeds maxItems %s", len(arr), v.String()))
		}
	}
	if items, ok := m["items"]; ok {
		for i, item := range arr {
			s.validateNode(items, item, instPath+"/"+strconv.Itoa(i), schemaPath+"/items", errs)
		}
	}
}

// validateCombinations handles allOf/anyOf/oneOf/not. Branch failures are
// collected into scratch buffers and only surface when the combination as a
// whole fails, so results never depend on branch order.
func (s *Schema) validateCombinations(m map[string]interface{}, inst interface{}, instPath, schemaPath string, errs *[]InstanceError) {
	branchOK := func(subschema interface{}, subPath string) bool {
		var scratch []InstanceError
		s.validateNode(subschema, inst, instPath, subPath, &scratch)
		return len(scratch) == 0
	}
	if list, ok := m["allOf"].([]interface{}); ok {
		for i, subschema := range list {
			subPath := schemaPath + "/allOf/" + strconv.Itoa(i)
			var scratch []InstanceError
			s.validateNode(subschema, inst, instPath, subPath, &scratch)
			*errs = append(*errs, scratch...)
		}
	}
	if list, ok := m["anyOf"].([]interface{}); ok {
		matched := 0
		for i, subschema := range list {
			if branchOK(subschema, schemaPath+"/anyOf/"+strconv.Itoa(i)) {
				matched++
			}
		}
		if matched == 0 {
			s.fail(errs, instPath, schemaPath+"/anyOf", "does not match any anyOf branch")
		}
	}
	if list, ok := m["oneOf"].([]interface{}); ok {
		matched := 0
		for i, subschema := range list {
			if branchOK(subschema, schemaPath+"/oneOf/"+strconv.Itoa(i)) {
				matched++
			}
		}
		if matched != 1 {
			s.fail(errs, instPath, schemaPath+"/oneOf", fmt.Sprintf("matches %d oneOf branches, expected exactly 1", matched))
		}
	}
	if subschema, ok := m["not"]; ok {
		if branchOK(subschema, schemaPath+"/not") {
			s.fail(errs, instPath, schemaPath+"/not", "matches schema in not")
		}
	}
}
