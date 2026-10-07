package main

import (
	"encoding/xml"
	"strings"
	"testing"
)

func collect(t *testing.T, doc, path string, maxSize int64) []string {
	t.Helper()
	segs, err := ParsePath(path)
	if err != nil {
		t.Fatalf("ParsePath: %v", err)
	}
	var records []string
	_, err = Run(strings.NewReader(doc), segs, maxSize, func(data []byte) error {
		records = append(records, string(data))
		return nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return records
}

// reparse parses a fragment and returns the resolved start-element events.
func reparse(t *testing.T, frag string) []xml.StartElement {
	t.Helper()
	var out []xml.StartElement
	d := xml.NewDecoder(strings.NewReader(frag))
	for {
		tok, err := d.Token()
		if err != nil {
			if err.Error() == "EOF" {
				return out
			}
			t.Fatalf("fragment does not reparse: %v\n%s", err, frag)
		}
		if se, ok := tok.(xml.StartElement); ok {
			out = append(out, se)
		}
	}
}

func TestCrossLayerDeclaration(t *testing.T) {
	doc := `<env xmlns="urn:env" xmlns:p="urn:p"><p:record><p:item p:k="v">x</p:item></p:record></env>`
	records := collect(t, doc, "{urn:env}env/{urn:p}record", 0)
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	els := reparse(t, records[0])
	if els[0].Name.Space != "urn:p" || els[0].Name.Local != "record" {
		t.Fatalf("record root resolved to %+v", els[0].Name)
	}
	if els[1].Name.Space != "urn:p" {
		t.Fatalf("inner element lost ancestor prefix binding: %+v", els[1].Name)
	}
	var found bool
	for _, a := range els[1].Attr {
		if a.Name.Local == "k" {
			found = true
			if a.Name.Space != "urn:p" {
				t.Fatalf("prefixed attribute resolved to %q", a.Name.Space)
			}
		}
	}
	if !found {
		t.Fatal("attribute k missing after reparse")
	}
}

func TestDefaultNamespaceAndUnprefixedAttribute(t *testing.T) {
	doc := `<r xmlns="urn:d"><rec a="1"><kid/></rec></r>`
	records := collect(t, doc, "{urn:d}r/{urn:d}rec", 0)
	els := reparse(t, records[0])
	if els[0].Name.Space != "urn:d" {
		t.Fatalf("default namespace lost: %+v", els[0].Name)
	}
	if els[1].Name.Space != "urn:d" {
		t.Fatalf("child did not inherit default namespace: %+v", els[1].Name)
	}
	for _, a := range els[0].Attr {
		if a.Name.Local == "a" && a.Name.Space != "" {
			t.Fatalf("unprefixed attribute wrongly inherited default namespace %q", a.Name.Space)
		}
	}
}

func TestPrefixRebindingInsideRecord(t *testing.T) {
	doc := `<r xmlns:p="urn:a"><rec><p:x/><inner xmlns:p="urn:b"><p:y/></inner></rec></r>`
	records := collect(t, doc, "r/rec", 0)
	els := reparse(t, records[0])
	if els[1].Name.Space != "urn:a" {
		t.Fatalf("outer binding: got %q", els[1].Name.Space)
	}
	if els[3].Name.Space != "urn:b" {
		t.Fatalf("rebound inner prefix: got %q", els[3].Name.Space)
	}
}

func TestSameLocalNameDifferentNamespace(t *testing.T) {
	doc := `<r xmlns:a="urn:a" xmlns:b="urn:b"><a:rec>1</a:rec><b:rec>2</b:rec><a:rec>3</a:rec></r>`
	records := collect(t, doc, "r/{urn:a}rec", 0)
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if !strings.Contains(records[0], ">1<") || !strings.Contains(records[1], ">3<") {
		t.Fatalf("records out of order or wrong: %v", records)
	}
}

func TestLangAndSpaceInheritance(t *testing.T) {
	doc := `<r xml:lang="zh-CN" xml:space="preserve"><rec><kid xml:lang="en"/></rec><rec xml:space="default"/></r>`
	records := collect(t, doc, "r/rec", 0)
	if len(records) != 2 {
		t.Fatalf("got %d records", len(records))
	}
	els := reparse(t, records[0])
	attrs := map[string]string{}
	for _, a := range els[0].Attr {
		attrs[a.Name.Local] = a.Value
	}
	if attrs["lang"] != "zh-CN" || attrs["space"] != "preserve" {
		t.Fatalf("record root missing inherited xml attrs: %v", attrs)
	}
	// Inner override must win inside the record.
	if els[1].Attr[0].Value != "en" {
		t.Fatalf("inner xml:lang override lost: %+v", els[1].Attr)
	}
	// Root-level override must not be replaced by the ancestor value.
	els2 := reparse(t, records[1])
	for _, a := range els2[0].Attr {
		if a.Name.Local == "space" && a.Value != "default" {
			t.Fatalf("root xml:space override lost: %v", a.Value)
		}
	}
}
