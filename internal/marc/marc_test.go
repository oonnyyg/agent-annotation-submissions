package marc

import (
	"fmt"
	"strings"
	"testing"
)

// buildRecord assembles a MARC record from tag/body pairs. Bodies must
// already include indicators and terminators are added here. Physical
// order follows the slice order.
func buildRecord(fields ...[2]string) []byte {
	var dir, data strings.Builder
	offset := 0
	for _, f := range fields {
		body := f[1] + "\x1e"
		fmt.Fprintf(&dir, "%s%04d%05d", f[0], len(body), offset)
		data.WriteString(body)
		offset += len(body)
	}
	base := leaderLen + dir.Len() + 1
	total := base + data.Len() + 1
	leader := fmt.Sprintf("%05dnam  22%05d   4500", total, base)
	return []byte(leader + dir.String() + "\x1e" + data.String() + "\x1d")
}

func TestParseChineseByteOffsets(t *testing.T) {
	// 中文 title: offsets must count bytes, not characters.
	title := "中文  书名"
	f245 := "10\x1fa" + title + "\x1fb副题名"
	rec := buildRecord([2]string{"001", "ctl-1"}, [2]string{"245", f245})
	records, diags := Parse(rec)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records", len(records))
	}
	f := records[0].FieldsByTag("245")[0]
	if len(f.Subfields) != 2 || f.Subfields[0].Value != title || f.Subfields[1].Value != "副题名" {
		t.Fatalf("bad subfields: %+v", f.Subfields)
	}
	ctl := records[0].FieldsByTag("001")[0]
	if ctl.Value != "ctl-1" || ctl.Subfields != nil {
		t.Fatalf("control field must not be split: %+v", ctl)
	}
}

func TestPhysicalOrderDiffersFromDirectory(t *testing.T) {
	// Directory lists 001 then 245, but 245 bytes come first physically.
	f001 := "ctl-2"
	f245 := "10\x1fa题"
	// Hand-assemble: directory order 001,245; data order 245,001.
	var dir, data strings.Builder
	body245 := f245 + "\x1e"
	body001 := f001 + "\x1e"
	fmt.Fprintf(&dir, "001%04d%05d", len(body001), len(body245))
	fmt.Fprintf(&dir, "245%04d%05d", len(body245), 0)
	data.WriteString(body245)
	data.WriteString(body001)
	base := leaderLen + dir.Len() + 1
	total := base + data.Len() + 1
	raw := []byte(fmt.Sprintf("%05dnam  22%05d   4500", total, base) + dir.String() + "\x1e" + data.String() + "\x1d")

	records, diags := Parse(raw)
	if len(diags) != 0 || len(records) != 1 {
		t.Fatalf("diags=%v records=%d", diags, len(records))
	}
	if records[0].Fields[0].Tag != "001" || records[0].Fields[1].Tag != "245" {
		t.Fatalf("fields should follow directory order: %+v", records[0].Fields)
	}
	if records[0].FieldsByTag("245")[0].Subfields[0].Value != "题" {
		t.Fatal("245 value mismatch")
	}
}

func TestRepeatedFieldsKeepOrder(t *testing.T) {
	rec := buildRecord(
		[2]string{"020", "  \x1fa111"},
		[2]string{"020", "  \x1fa222\x1fa333"},
	)
	records, diags := Parse(rec)
	if len(diags) != 0 {
		t.Fatal(diags)
	}
	fs := records[0].FieldsByTag("020")
	if len(fs) != 2 || fs[0].Subfields[0].Value != "111" ||
		fs[1].Subfields[0].Value != "222" || fs[1].Subfields[1].Value != "333" {
		t.Fatalf("order not preserved: %+v", fs)
	}
}
func TestInvalidUTF8Rejected(t *testing.T) {
	rec := buildRecord([2]string{"245", "10\x1fa\xff\xfe"})
	records, diags := Parse(rec)
	if len(records) != 0 || len(diags) != 1 || diags[0].Fatal {
		t.Fatalf("records=%d diags=%v", len(records), diags)
	}
}

func TestBadRecordIsolatedAndParsingContinues(t *testing.T) {
	good1 := buildRecord([2]string{"001", "a"})
	// Bad: field range outside record data.
	bad := buildRecord([2]string{"001", "b"})
	binary := []byte(bad)
	binary[24+3] = '9' // inflate first directory entry length
	good2 := buildRecord([2]string{"001", "c"})
	stream := append(append(good1, binary...), good2...)

	records, diags := Parse(stream)
	if len(records) != 2 {
		t.Fatalf("expected 2 good records, got %d (diags=%v)", len(records), diags)
	}
	if len(diags) != 1 || diags[0].Fatal || diags[0].Offset != len(good1) {
		t.Fatalf("diags=%v", diags)
	}
	if records[1].Offset != len(good1)+len(bad) {
		t.Fatalf("second record offset %d", records[1].Offset)
	}
}

func TestFatalStopsParsing(t *testing.T) {
	good := buildRecord([2]string{"001", "a"})
	stream := append(good, []byte("garbage")...)
	records, diags := Parse(stream)
	if len(records) != 1 || len(diags) != 1 || !diags[0].Fatal {
		t.Fatalf("records=%d diags=%v", len(records), diags)
	}
	if diags[0].Offset != len(good) {
		t.Fatalf("fatal offset %d, want %d", diags[0].Offset, len(good))
	}
}

func TestMissingFieldTerminator(t *testing.T) {
	rec := buildRecord([2]string{"001", "x"})
	rec[len(rec)-2] = ' ' // clobber field terminator (last data byte before 0x1d)
	_, diags := Parse(rec)
	if len(diags) != 1 || diags[0].Fatal {
		t.Fatalf("diags=%v", diags)
	}
}
