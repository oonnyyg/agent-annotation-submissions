package catalog

import (
	"testing"

	"example.com/marc-isbn-catalog/internal/marc"
)

func rec(fields ...marc.Field) *marc.Record {
	return &marc.Record{Fields: fields}
}

func dataField(tag string, subs ...marc.Subfield) marc.Field {
	return marc.Field{Tag: tag, Subfields: subs}
}

func sf(code byte, val string) marc.Subfield {
	return marc.Subfield{Code: code, Value: val}
}

func TestExtractTitleAndISBNs(t *testing.T) {
	r := rec(
		marc.Field{Tag: "001", Value: "ocm1"},
		dataField("245", sf('a', "主标题"), sf('b', "副标题")),
		dataField("245", sf('a', "第二部分")),
		dataField("020", sf('a', "0-306-40615-2"), sf('a', "bad-isbn")),
		dataField("020", sf('a', "9780306406157")), // same canonical as the 10-digit above
	)
	e := Extract(r)
	if e.ControlNumber != "ocm1" {
		t.Fatalf("control number %q", e.ControlNumber)
	}
	if e.Title != "主标题 副标题 第二部分" {
		t.Fatalf("title %q", e.Title)
	}
	if len(e.ValidISBNs) != 1 || e.ValidISBNs[0] != "9780306406157" {
		t.Fatalf("valid isbns %v", e.ValidISBNs)
	}
	if len(e.InvalidISBNs) != 1 || e.InvalidISBNs[0] != "bad-isbn" {
		t.Fatalf("invalid isbns %v", e.InvalidISBNs)
	}
}

func entry(title string, isbns ...string) Entry {
	e := Entry{Title: title}
	for _, s := range isbns {
		e.ValidISBNs = append(e.ValidISBNs, s)
	}
	return e
}

func TestTransitiveGrouping(t *testing.T) {
	entries := []Entry{
		entry("甲", "9780000000002"),
		entry("甲", "9780000000002", "9791090636071"),
		entry("甲", "9791090636071"),
		entry("乙", "9780306406157"),
	}
	groups := Groups(entries)
	if len(groups) != 2 {
		t.Fatalf("groups=%d %+v", len(groups), groups)
	}
	if len(groups[0].Members) != 3 || groups[0].Conflict {
		t.Fatalf("transitive group wrong: %+v", groups[0])
	}
	if len(groups[0].ISBNs) != 2 {
		t.Fatalf("group isbns %v", groups[0].ISBNs)
	}
}

func TestTitleConflictAndWhitespaceCollapse(t *testing.T) {
	entries := []Entry{
		entry("Some  Title", "9780000000002"),
		entry(" Some Title ", "9780000000002"), // same after collapse
		entry("Other Title", "9780000000002"),  // conflict
	}
	groups := Groups(entries)
	if len(groups) != 1 {
		t.Fatalf("groups=%d", len(groups))
	}
	g := groups[0]
	if !g.Conflict || len(g.Titles) != 2 {
		t.Fatalf("expected conflict with 2 titles: %+v", g)
	}
	if g.Titles[0] != "Some Title" || g.Titles[1] != "Other Title" {
		t.Fatalf("titles %v", g.Titles)
	}
}

func TestInvalidISBNNeverLinks(t *testing.T) {
	entries := []Entry{
		{Title: "A", InvalidISBNs: []string{"junk"}},
		{Title: "B", InvalidISBNs: []string{"junk"}},
	}
	groups := Groups(entries)
	if len(groups) != 2 {
		t.Fatalf("invalid ISBNs must not link records: %+v", groups)
	}
}
