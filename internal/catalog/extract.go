// Package catalog extracts bibliographic data from MARC records and
// builds ISBN-connected groups with title-conflict diagnostics.
package catalog

import (
	"strings"

	"example.com/marc-isbn-catalog/internal/isbn"
	"example.com/marc-isbn-catalog/internal/marc"
)

// Entry is the extracted bibliographic view of one record.
type Entry struct {
	Index         int      `json:"index"`
	Offset        int      `json:"offset"`
	ControlNumber string   `json:"control_number"`
	Title         string   `json:"title"`
	RawISBNs      []string `json:"raw_isbns"`
	ValidISBNs    []string `json:"valid_isbns"`   // canonical ISBN-13, deduplicated, order kept
	InvalidISBNs  []string `json:"invalid_isbns"` // raw values failing validation
}

// Extract pulls control number (001), title (all 245 $a/$b in directory
// order, space-joined) and ISBNs (020 $a) from a record.
func Extract(rec *marc.Record) Entry {
	e := Entry{Index: rec.Index, Offset: rec.Offset}
	if f := rec.FieldsByTag("001"); len(f) > 0 {
		e.ControlNumber = f[0].Value
	}
	var titleParts []string
	for _, f := range rec.FieldsByTag("245") {
		for _, sf := range f.Subfields {
			if sf.Code == 'a' || sf.Code == 'b' {
				titleParts = append(titleParts, sf.Value)
			}
		}
	}
	e.Title = strings.Join(titleParts, " ")
	seen := map[string]bool{}
	for _, f := range rec.FieldsByTag("020") {
		for _, sf := range f.Subfields {
			if sf.Code != 'a' {
				continue
			}
			raw := sf.Value
			e.RawISBNs = append(e.RawISBNs, raw)
			if canon, ok := isbn.Normalize(raw); ok {
				if !seen[canon] {
					seen[canon] = true
					e.ValidISBNs = append(e.ValidISBNs, canon)
				}
			} else {
				e.InvalidISBNs = append(e.InvalidISBNs, raw)
			}
		}
	}
	return e
}

// ExtractAll maps records to entries.
func ExtractAll(records []marc.Record) []Entry {
	entries := make([]Entry, len(records))
	for i := range records {
		entries[i] = Extract(&records[i])
	}
	return entries
}
