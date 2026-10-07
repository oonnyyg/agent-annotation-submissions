package split

import (
	"strings"
	"testing"
)

func splitErr(input, path string, max int64) (int, error) {
	p, err := ParsePath(path)
	if err != nil {
		return 0, err
	}
	sp := &Splitter{Path: p, MaxRecord: max}
	return sp.Split(strings.NewReader(input))
}

func TestTruncatedInput(t *testing.T) {
	_, err := splitErr(`<r><item>1</item><item>2`, "r/item", 0)
	if err == nil {
		t.Fatal("expected error for truncated input")
	}
	se, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if se.Line < 1 || !strings.Contains(se.Msg, "unclosed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUndeclaredPrefix(t *testing.T) {
	_, err := splitErr(`<r><p:item/></r>`, "r/item", 0)
	se, ok := err.(*Error)
	if !ok || !strings.Contains(se.Msg, `undeclared namespace prefix "p"`) {
		t.Fatalf("err = %v", err)
	}
	if se.Offset <= 0 || se.Line != 1 {
		t.Fatalf("position missing: %v", se)
	}
}

func TestUndeclaredAttrPrefix(t *testing.T) {
	_, err := splitErr(`<r><item q:a="1"/></r>`, "r/item", 0)
	if err == nil || !strings.Contains(err.Error(), `undeclared namespace prefix "q"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestDTDRejected(t *testing.T) {
	input := `<!DOCTYPE r [<!ENTITY x "boom">]><r><item>&x;</item></r>`
	_, err := splitErr(input, "r/item", 0)
	if err == nil || !strings.Contains(err.Error(), "DTD") {
		t.Fatalf("err = %v", err)
	}
}

func TestExternalEntityRejected(t *testing.T) {
	input := `<!DOCTYPE r SYSTEM "http://evil/x.dtd"><r><item/></r>`
	_, err := splitErr(input, "r/item", 0)
	if err == nil || !strings.Contains(err.Error(), "DTD") {
		t.Fatalf("err = %v", err)
	}
}

func TestUndeclaredEntity(t *testing.T) {
	_, err := splitErr(`<r><item>&nosuch;</item></r>`, "r/item", 0)
	if err == nil {
		t.Fatal("expected error for undeclared entity")
	}
}

func TestMalformedXMLPosition(t *testing.T) {
	_, err := splitErr("<r>\n  <item>\n</r>", "r/item", 0)
	se, ok := err.(*Error)
	if !ok {
		t.Fatalf("err = %v", err)
	}
	if se.Line != 3 {
		t.Fatalf("line = %d, want 3 (%v)", se.Line, se)
	}
}

func TestRecordSizeLimit(t *testing.T) {
	input := `<r><item>` + strings.Repeat("x", 5000) + `</item></r>`
	_, err := splitErr(input, "r/item", 100)
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("err = %v", err)
	}
}

func TestNamespaceDistinctPaths(t *testing.T) {
	// Same local names, different namespaces: only exact URI matches.
	input := `<a:r xmlns:a="urn:a" xmlns:b="urn:b"><b:item>no</b:item></a:r>`
	recs := splitAll(t, input, "{urn:a}r/{urn:b}item", 0)
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	if _, err := splitErr(input, "{urn:x}r/{urn:b}item", 0); err != nil {
		t.Fatalf("err = %v", err)
	}
}
