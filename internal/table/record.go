package table

import (
	"fmt"

	"example.com/external-stable-table-sort/internal/config"
	"example.com/external-stable-table-sort/internal/decimal"
)

// Record is one CSV data row with its original-order coordinates.
// LogicalRecord is 1-based within the source file and excludes the header.
type Record struct {
	Fields        []string
	SourceFile    int
	LogicalRecord int
}

// Location identifies a record for error messages.
func (r Record) Location(fileNames []string) string {
	if r.SourceFile >= 0 && r.SourceFile < len(fileNames) {
		return fmt.Sprintf("file %q record %d", fileNames[r.SourceFile], r.LogicalRecord)
	}
	return fmt.Sprintf("file index %d record %d", r.SourceFile, r.LogicalRecord)
}

// Comparator compares records by the configured keys and never by raw record
// payload: callers achieve stability via OriginalLess on full-key ties.
type Comparator struct {
	keys      []config.ResolvedKey
	fileNames []string
}

// NewComparator validates that every key value is usable in the records it
// will compare; decimal conversion errors surface lazily during Compare.
func NewComparator(keys []config.ResolvedKey, fileNames []string) *Comparator {
	return &Comparator{keys: keys, fileNames: fileNames}
}

// Less reports whether a sorts before b under the key rules.
func (c *Comparator) Less(a, b table.Record) (bool, error) {
	for _, k := range c.keys {
		cmp, err := c.compareKey(k, a, b)
		if err != nil {
			return false, err
		}
		if cmp != 0 {
			if k.Rule.Direction == config.Desc {
				cmp = -cmp
			}
			return cmp < 0, nil
		}
	}
	return false, nil
}

// KeyEqual reports whether all sort keys match (used for dedup).
func (c *Comparator) KeyEqual(a, b table.Record) (bool, error) {
	for _, k := range c.keys {
		cmp, err := c.compareKey(k, a, b)
		if err != nil {
			return false, err
		}
		if cmp != 0 {
			return false, nil
		}
	}
	return true, nil
}

func (c *Comparator) compareKey(k config.ResolvedKey, a, b table.Record) (int, error) {
	va := a.Fields[k.Index]
	vb := b.Fields[k.Index]
	aNull := va == ""
	bNull := vb == ""
	switch {
	case aNull && bNull:
		return 0, nil
	case aNull:
		if k.Rule.NullOrder == config.NullFirst {
			return -1, nil
		}
		return 1, nil
	case bNull:
		if k.Rule.NullOrder == config.NullFirst {
			return 1, nil
		}
		return -1, nil
	}
	switch k.Rule.Type {
	case config.KeyText:
		switch {
		case va < vb:
			return -1, nil
		case va > vb:
			return 1, nil
		default:
			return 0, nil
		}
	case config.KeyDecimal:
		da, err := decimal.Parse(va)
		if err != nil {
			return 0, fmt.Errorf("%s: key column %q: %w", a.Location(c.fileNames), k.Rule.Column, err)
		}
		db, err := decimal.Parse(vb)
		if err != nil {
			return 0, fmt.Errorf("%s: key column %q: %w", b.Location(c.fileNames), k.Rule.Column, err)
		}
		return decimal.Compare(da, db), nil
	default:
		return 0, fmt.Errorf("internal error: unknown key type %q", k.Rule.Type)
	}
}

// OriginalLess defines global original order: file order, then in-file order.
func OriginalLess(a, b Record) bool {
	if a.SourceFile != b.SourceFile {
		return a.SourceFile < b.SourceFile
	}
	return a.LogicalRecord < b.LogicalRecord
}
