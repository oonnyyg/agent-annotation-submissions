package bencode

import (
	"bytes"
	"testing"
)

func mustDecode(t *testing.T, s string) *Value {
	t.Helper()
	v, err := Decode([]byte(s))
	if err != nil {
		t.Fatalf("Decode(%q): %v", s, err)
	}
	return v
}

func TestDecodeValid(t *testing.T) {
	v := mustDecode(t, "d3:bari1e4:spamli42e3:abcee")
	if v.Kind != Dict {
		t.Fatal("expected dict")
	}
	if got := v.Get("bar").Int; got != 1 {
		t.Fatalf("bar = %d", got)
	}
	if got := v.Get("spam").List[0].Int; got != 42 {
		t.Fatalf("spam[0] = %d", got)
	}
	if !bytes.Equal(v.Get("spam").List[1].Str, []byte("abc")) {
		t.Fatal("spam[1] mismatch")
	}
}

func TestRawSpanRetained(t *testing.T) {
	data := []byte("d4:infod1:ai1eee")
	v := mustDecode(t, string(data))
	info := v.Get("info")
	if got := string(data[info.Start:info.End]); got != "d1:ai1ee" {
		t.Fatalf("raw info span = %q", got)
	}
}

func TestRejections(t *testing.T) {
	cases := map[string]string{
		"truncated string: 5:abc": "5:abc",
		"unterminated int":        "i42",
		"unterminated list":       "li1e",
		"unterminated dict":       "d1:ai1e",
		"trailing bytes":          "i1ei2e",
		"empty int":               "ie",
		"lone minus":              "i-e",
		"negative zero":           "i-0e",
		"leading zero int":        "i03e",
		"leading zero strlen":     "03:abc",
		"non-digit int":           "i1xe",
		"bad type byte":           "x1:a",
		"duplicate key":           "d1:ai1e1:ai2ee",
		"string length overruns":  "999999999999:a",
		"nested duplicate key":    "d1:ad1:bi1e1:bi2eee",
	}
	for name, in := range cases {
		if _, err := Decode([]byte(in)); err == nil {
			t.Errorf("%s: Decode(%q) succeeded, want error", name, in)
		}
	}
}

func TestUnsortedKeysWarning(t *testing.T) {
	v := mustDecode(t, "d1:bi1e1:ai2ee")
	if !v.UnsortedKeys() {
		t.Fatal("expected unsorted keys to be detected")
	}
	v = mustDecode(t, "d1:ai2e1:bi1ee")
	if v.UnsortedKeys() {
		t.Fatal("sorted keys flagged as unsorted")
	}
}
