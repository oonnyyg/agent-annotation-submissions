package dnswire

import (
	"encoding/json"
	"testing"
)

func TestJSONRoundTrip(t *testing.T) {
	docJSON := `{
	  "header": {"id": 42, "qr": true, "rd": true},
	  "questions": [{"name": ["www", "Example", "com"], "qtype": 1, "qclass": 1}],
	  "answers": [
	    {"name": ["www", "Example", "com"], "type": 1, "class": 1, "ttl": 300, "data": "192.0.2.1"},
	    {"name": ["Example", "com"], "type": 15, "class": 1, "ttl": 60,
	     "data": {"preference": 10, "exchange": ["mail", "Example", "com"]}},
	    {"name": [], "type": 16, "class": 1, "ttl": 0, "data": ["a", "", "b"]}
	  ]
	}`
	var doc Document
	if err := json.Unmarshal([]byte(docJSON), &doc); err != nil {
		t.Fatal(err)
	}
	m, err := doc.ToMessage()
	if err != nil {
		t.Fatal(err)
	}
	wire, err := EncodeMessage(m)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeMessage(wire)
	if err != nil {
		t.Fatal(err)
	}
	// Decode output JSON must be acceptable as encode input.
	out, err := json.Marshal(back)
	if err != nil {
		t.Fatal(err)
	}
	var doc2 Document
	if err := json.Unmarshal(out, &doc2); err != nil {
		t.Fatal(err)
	}
	m2, err := doc2.ToMessage()
	if err != nil {
		t.Fatalf("decoded JSON not re-encodable: %v", err)
	}
	wire2, err := EncodeMessage(m2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeMessage(wire2); err != nil {
		t.Fatal(err)
	}
	if got := back.Questions[0].Name.Labels[1]; got != "Example" {
		t.Fatalf("case lost: %q", got)
	}
	txts := back.Answers[2].Data.([]string)
	if len(txts) != 3 || txts[1] != "" {
		t.Fatalf("TXT strings lost: %#v", txts)
	}
}

func TestJSONRejectsUnknownRaw(t *testing.T) {
	docJSON := `{"answers": [{"name": ["x"], "type": 99, "class": 1, "ttl": 0, "raw": "deadbeef"}]}`
	var doc Document
	if err := json.Unmarshal([]byte(docJSON), &doc); err != nil {
		t.Fatal(err)
	}
	m, err := doc.ToMessage()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EncodeMessage(m); err == nil {
		t.Fatal("expected rejection of raw record")
	}
}
