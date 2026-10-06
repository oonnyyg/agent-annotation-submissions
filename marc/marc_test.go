package marc

import (
	"bytes"
	"fmt"
	"testing"
)

type testField struct {
	tag  string
	data string // includes indicators for data fields, excludes 0x1E
}

// buildRecord assembles a MARC record. Directory order follows fields;
// physical order can be permuted via physicalOrder (indexes into fields).
func buildRecord(t *testing.T, fields []testField, physicalOrder []int) []byte {
	t.Helper()
	if physicalOrder == nil {
		physicalOrder = make([]int, len(fields))
		for i := range fields {
			physicalOrder[i] = i
		}
	}
	dirLen := len(fields) * DirectoryEntrySize
	baseAddr := LeaderSize + dirLen + 1
	// Compute physical offsets.
	physOffset := map[int]int{}
	area := []byte{}
	pos := 0
	for _, fi := range physicalOrder {
		body := []byte(fields[fi].data)
		physOffset[fi] = pos
		area = append(area, body...)
		area = append(area, FieldTerminator)
		pos += len(body) + 1
	}
	recLen := baseAddr + len(area) + 1
	leader := fmt.Sprintf("%05dnam a22%05d a 4500", recLen, baseAddr)
	if len(leader) != LeaderSize {
		t.Fatalf("bad leader %q", leader)
	}
	var buf bytes.Buffer
	buf.WriteString(leader)
	for i, f := range fields {
		buf.WriteString(f.tag)
		buf.WriteString(fmt.Sprintf("%04d", len(f.data)+1))
		buf.WriteString(fmt.Sprintf("%05d", physOffset[i]))
	}
	buf.WriteByte(FieldTerminator)
	buf.Write(area)
	buf.WriteByte(RecordTerminator)
	return buf.Bytes()
}

func TestParseChineseByteOffsets(t *testing.T) {
	fields := []testField{
		{"001", "ctrl-1"},
		{"245", "10\x1fa中文题名\x1fb副题名"},
		{"020", "  \x1fa978-7-02-000220-7 "},
	}
	// Physical order differs from directory order.
	data := buildRecord(t, fields, []int{2, 0, 1})
	rec, err := ParseRecord(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	e := ExtractEntry(rec, 0)
	if e.ControlNumber != "ctrl-1" {
		t.Errorf("control number = %q", e.ControlNumber)
	}
	if e.Title != "中文题名 副题名" {
		t.Errorf("title = %q", e.Title)
	}
	if len(e.ISBNs) != 1 || e.ISBNs[0] != "9787020002207" {
		t.Errorf("isbns = %v", e.ISBNs)
	}
}

func TestRepeatedFieldsAndSubfieldsOrder(t *testing.T) {
	fields := []testField{
		{"245", "10\x1fa第一部分"},
		{"020", "  \x1fa0-306-40615-2"},
		{"245", "10\x1fa第二部分\x1fb续"},
		{"020", "  \x1fa9787020002207\x1fa坏-号"},
	}
	rec, err := ParseRecord(buildRecord(t, fields, nil), 0)
	if err != nil {
		t.Fatal(err)
	}
	e := ExtractEntry(rec, 0)
	want := "第一部分 第二部分 续"
	if e.Title != want {
		t.Errorf("title = %q, want %q", e.Title, want)
	}
	if len(e.ISBNs) != 2 || e.ISBNs[0] != "9780306406157" || e.ISBNs[1] != "9787020002207" {
		t.Errorf("isbns = %v", e.ISBNs)
	}
	if len(e.InvalidISBNs) != 1 || e.InvalidISBNs[0] != "坏-号" {
		t.Errorf("invalid = %v", e.InvalidISBNs)
	}
}

func TestControlFieldNotSplit(t *testing.T) {
	fields := []testField{{"008", "960101s1996////ch|||||||||||||chi|d"}}
	rec, err := ParseRecord(buildRecord(t, fields, nil), 0)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Fields[0].Value != fields[0].data || len(rec.Fields[0].Subfields) != 0 {
		t.Errorf("control field parsed as %+v", rec.Fields[0])
	}
}
