package main

import (
	"encoding/json"
	"strings"
)

func decodeJSON(s string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	err := dec.Decode(&v)
	return v, err
}
