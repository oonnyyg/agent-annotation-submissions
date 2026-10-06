package scanlogic

import (
	"strings"
	"testing"
)

func TestInvalidProgramsRejected(t *testing.T) {
	cases := map[string]string{
		"undeclared contact": `{
		  "inputs":["I1"],"coils":["Q"],
		  "statements":[{"type":"coil","target":"Q","condition":{"op":"contact","name":"I9"}}]}`,
		"undeclared coil target": `{
		  "inputs":["I1"],"coils":["Q"],
		  "statements":[{"type":"coil","target":"Q2","condition":{"op":"contact","name":"I1"}}]}`,
		"double write same coil": `{
		  "inputs":["I1"],"coils":["Q"],
		  "statements":[
		    {"type":"coil","target":"Q","condition":{"op":"contact","name":"I1"}},
		    {"type":"coil","target":"Q","condition":{"op":"contact","name":"I1"}}]}`,
		"double write same timer": `{
		  "inputs":["I1"],"timers":[{"name":"T1","preset_ms":10}],
		  "statements":[
		    {"type":"ton","timer":"T1","condition":{"op":"contact","name":"I1"}},
		    {"type":"ton","timer":"T1","condition":{"op":"contact","name":"I1"}}]}`,
		"non-positive timer preset": `{
		  "inputs":["I1"],"timers":[{"name":"T1","preset_ms":0}],
		  "statements":[{"type":"ton","timer":"T1","condition":{"op":"contact","name":"I1"}}]}`,
		"non-positive counter preset": `{
		  "inputs":["I1"],"counters":[{"name":"C1","preset":-1}],
		  "statements":[{"type":"ctu","counter":"C1",
		    "condition":{"op":"contact","name":"I1"},
		    "reset":{"op":"contact","name":"I1"}}]}`,
		"done on undeclared block": `{
		  "inputs":["I1"],"coils":["Q"],
		  "statements":[{"type":"coil","target":"Q","condition":{"op":"done","name":"T9"}}]}`,
		"ctu missing reset": `{
		  "inputs":["I1"],"counters":[{"name":"C1","preset":3}],
		  "statements":[{"type":"ctu","counter":"C1","condition":{"op":"contact","name":"I1"}}]}`,
		"unknown statement type": `{
		  "inputs":["I1"],"coils":["Q"],
		  "statements":[{"type":"move","target":"Q","condition":{"op":"contact","name":"I1"}}]}`,
		"empty and": `{
		  "inputs":["I1"],"coils":["Q"],
		  "statements":[{"type":"coil","target":"Q","condition":{"op":"and","args":[]}}]}`,
	}
	for name, src := range cases {
		if _, err := LoadProgram(strings.NewReader(src)); err == nil {
			t.Errorf("%s: expected rejection, got nil error", name)
		}
	}
}

func TestInvalidRecordsRejected(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["I1"],
	  "coils": ["Q"],
	  "statements": [
	    {"type":"coil","target":"Q","condition":{"op":"contact","name":"I1"}}
	  ]
	}`)
	cases := map[string]string{
		"zero duration":     `{"scan":1,"duration_ms":0,"inputs":{"I1":true}}`,
		"negative duration": `{"scan":1,"duration_ms":-5,"inputs":{"I1":true}}`,
		"missing input":     `{"scan":1,"duration_ms":10,"inputs":{}}`,
		"unknown input":     `{"scan":1,"duration_ms":10,"inputs":{"I1":true,"I2":false}}`,
	}
	for name, src := range cases {
		recs := records(t, src+"\n")
		if _, err := p.Run(recs); err == nil {
			t.Errorf("%s: expected rejection, got nil error", name)
		}
	}
}

// Every scan reports per-statement before/after changes.
func TestChangesReported(t *testing.T) {
	p := loadProgram(t, `{
	  "inputs": ["I1"],
	  "coils": ["Q"],
	  "statements": [
	    {"type":"coil","target":"Q","condition":{"op":"contact","name":"I1"}}
	  ]
	}`)
	recs := records(t, `{"scan":1,"duration_ms":10,"inputs":{"I1":true}}`+"\n")
	res, err := p.Run(recs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res[0].Changes) != 1 {
		t.Fatalf("changes = %+v", res[0].Changes)
	}
	ch := res[0].Changes[0]
	if ch.Statement != 0 || ch.Target != "coil:Q" || ch.Before != false || ch.After != true {
		t.Fatalf("change = %+v", ch)
	}
}
