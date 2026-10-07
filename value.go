package main

import (
	"encoding/json"
	"math/big"
	"sort"
	"unicode/utf8"
)

// jsonEqual compares two decoded JSON values structurally.
// Numbers compare by numeric value, so 1, 1.0 and 1e0 are equal.
func jsonEqual(a, b interface{}) bool {
	if an, ok := a.(json.Number); ok {
		bn, ok := b.(json.Number)
		if !ok {
			return false
		}
		return numberEqual(an, bn)
	}
	switch av := a.(type) {
	case nil:
		return b == nil
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case []interface{}:
		bv, ok := b.([]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, va := range av {
			vb, ok := bv[k]
			if !ok || !jsonEqual(va, vb) {
				return false
			}
		}
		return true
	}
	return false
}

func numberRat(n json.Number) (*big.Rat, bool) {
	r, ok := new(big.Rat).SetString(n.String())
	return r, ok
}

func numberEqual(a, b json.Number) bool {
	ra, ok1 := numberRat(a)
	rb, ok2 := numberRat(b)
	if !ok1 || !ok2 {
		return a.String() == b.String()
	}
	return ra.Cmp(rb) == 0
}

// isIntegerNumber reports whether n has no fractional part.
func isIntegerNumber(n json.Number) bool {
	r, ok := numberRat(n)
	return ok && r.IsInt()
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

type jsonNumber = json.Number

func numberSign(n json.Number) int {
	r, ok := numberRat(n)
	if !ok {
		return 0
	}
	return r.Sign()
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
