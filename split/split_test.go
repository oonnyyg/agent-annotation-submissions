package split

import (
	"encoding/xml"
	"strings"
	"testing"
)

func splitAll(t *testing.T, input, path string, max int64) []string {
	t.Helper()
	p, err := ParsePath(path)
	if err != nil {
		t.Fatalf("ParsePath: %v", err)
	}
	var out []string
	sp := &Splitter{
		Path:      p,
		MaxRecord: max,
		OnRecord:  func(b []byte) error { out = append(out, string(b)); return nil },
	}
	if _, err := sp.Split(strings.NewReader(input)); err != nil {
		t.Fatalf("Split: %v", err)
	}
	return out
}

// parseNames re-parses a fragment and returns the resolved element and
// attribute names in document order, proving the fragment is standalone.
func parseNames(t *testing.T, frag string) (elems []string, attrs []string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(frag))
	for {
		tok, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				return elems, attrs
			}
			t.Fatalf("fragment does not re-parse: %v\n%s", err, frag)
		}
		if se, ok := tok.(xml.StartElement); ok {
			elems = append(elems, "{"+se.Name.Space+"}"+se.Name.Local)
			for _, a := range se.Attr {
				if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
					continue
				}
				attrs = append(attrs, "{"+a.Name.Space+"}"+a.Name.Local+"="+a.Value)
			}
		}
	}
}

func TestCrossScopeDeclaration(t *testing.T) {
	// The record uses a prefix declared two levels above the record root.
	input := `<env xmlns="urn:env" xmlns:pay="urn:pay"><body><pay:order id="1"><pay:amount>5</pay:amount></pay:order></body></env>`
	recs := splitAll(t, input, "{urn:env}env/{urn:env}body/{urn:pay}order", 0)
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	elems, _ := parseNames(t, recs[0])
	want := []string{"{urn:pay}order", "{urn:pay}amount"}
	if strings.Join(elems, ",") != strings.Join(want, ",") {
		t.Fatalf("elements = %v, want %v\nfragment: %s", elems, want, recs[0])
	}
	if !strings.Contains(recs[0], `="urn:pay"`) {
		t.Fatalf("record misses the urn:pay declaration: %s", recs[0])
	}
}

func TestPrefixRebinding(t *testing.T) {
	// The same prefix "a" is rebound inside the record; re-parsed names
	// must still resolve to the original URIs.
	input := `<r xmlns:a="urn:one"><a:item><a:sub xmlns:a="urn:two"><a:deep/></a:sub></a:item></r>`
	recs := splitAll(t, input, "r/{urn:one}item", 0)
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	elems, _ := parseNames(t, recs[0])
	want := []string{"{urn:one}item", "{urn:two}sub", "{urn:two}deep"}
	if strings.Join(elems, ",") != strings.Join(want, ",") {
		t.Fatalf("elements = %v, want %v\nfragment: %s", elems, want, recs[0])
	}
}

func TestDefaultNamespaceAndUnprefixedAttr(t *testing.T) {
	input := `<r xmlns="urn:d"><item code="x" xmlns="urn:d2"><sub/></item></r>`
	recs := splitAll(t, input, "{urn:d}r/{urn:d2}item", 0)
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	elems, attrs := parseNames(t, recs[0])
	if elems[0] != "{urn:d2}item" || elems[1] != "{urn:d2}sub" {
		t.Fatalf("elements = %v\nfragment: %s", elems, recs[0])
	}
	// The unprefixed attribute must stay in no namespace.
	if len(attrs) != 1 || attrs[0] != "{}code=x" {
		t.Fatalf("attrs = %v, want unprefixed code=x\nfragment: %s", attrs, recs[0])
	}
}

func TestPrefixedAttrInDefaultNamespace(t *testing.T) {
	input := `<r xmlns="urn:d" xmlns:p="urn:d"><item p:code="y"/></r>`
	recs := splitAll(t, input, "{urn:d}r/{urn:d}item", 0)
	_, attrs := parseNames(t, recs[0])
	if len(attrs) != 1 || attrs[0] != "{urn:d}code=y" {
		t.Fatalf("attrs = %v, want {urn:d}code=y\nfragment: %s", attrs, recs[0])
	}
}

func TestSameLocalNameDifferentNamespaces(t *testing.T) {
	input := `<r xmlns:a="urn:a" xmlns:b="urn:b"><a:item>1</a:item><b:item>2</b:item><a:item>3</a:item></r>`
	recs := splitAll(t, input, "r/{urn:a}item", 0)
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	if !strings.Contains(recs[0], ">1<") || !strings.Contains(recs[1], ">3<") {
		t.Fatalf("order/content wrong: %q %q", recs[0], recs[1])
	}
}

func TestXMLLangAndSpaceInherited(t *testing.T) {
	input := `<r xml:lang="zh-CN" xml:space="preserve"><item><sub xml:lang="en">t</sub></item></r>`
	recs := splitAll(t, input, "r/item", 0)
	_, attrs := parseNames(t, recs[0])
	joined := strings.Join(attrs, ";")
	if !strings.Contains(joined, "{http://www.w3.org/XML/1998/namespace}lang=zh-CN") ||
		!strings.Contains(joined, "{http://www.w3.org/XML/1998/namespace}space=preserve") {
		t.Fatalf("record root lost inherited xml attrs: %v\n%s", attrs, recs[0])
	}
	// The inner override must be kept as-is.
	if !strings.Contains(joined, "{http://www.w3.org/XML/1998/namespace}lang=en") {
		t.Fatalf("inner xml:lang override lost: %v", attrs)
	}
}

func TestTextAndAttrEscaping(t *testing.T) {
	input := `<r><item a="1 &amp; &quot;2&quot;">x &lt; y &amp; z &#65;</item></r>`
	recs := splitAll(t, input, "r/item", 0)
	dec := xml.NewDecoder(strings.NewReader(recs[0]))
	var text string
	var attrVal string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch tt := tok.(type) {
		case xml.StartElement:
			if tt.Name.Local == "item" {
				attrVal = tt.Attr[0].Value
			}
		case xml.CharData:
			text += string(tt)
		}
	}
	if attrVal != `1 & "2"` {
		t.Fatalf("attr = %q", attrVal)
	}
	if strings.TrimSpace(text) != "x < y & z A" {
		t.Fatalf("text = %q", text)
	}
}
