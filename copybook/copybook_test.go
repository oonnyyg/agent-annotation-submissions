package copybook

import (
	"strings"
	"testing"
)

const testCopybook = `
01 ORDER-RECORD.
   05 ORDER-ID        PIC X(8).
   05 ORDER-DATE      PIC 9(8).
   05 TOTAL-AMOUNT    PIC S9(7)V99 COMP-3.
   05 ORDER-LINES     OCCURS 3 TIMES.
      10 LINE-NO      PIC 9(2).
      10 LINE-ITEM.
         15 SKU       PIC X(6).
         15 QTY       PIC 9(3).
      10 UNIT-PRICE   PIC S9(5)V99 COMP-3.
`

func compileTest(t *testing.T) *Layout {
	t.Helper()
	root, err := Parse(testCopybook)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	l, err := Compile(root)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return l
}

func TestLayoutOffsets(t *testing.T) {
	l := compileTest(t)
	// 8 + 8 + 5 + 3*(2+6+3+4) = 66
	if l.RecordLength != 66 {
		t.Fatalf("record length = %d, want 66", l.RecordLength)
	}
	want := map[string]Field{
		"ORDER-RECORD.ORDER-ID":                  {Offset: 0, Length: 8, Occurs: 1, ElementStride: 8},
		"ORDER-RECORD.ORDER-DATE":                {Offset: 8, Length: 8, Occurs: 1, ElementStride: 8},
		"ORDER-RECORD.TOTAL-AMOUNT":              {Offset: 16, Length: 5, Occurs: 1, Scale: 2, ElementStride: 5},
		"ORDER-RECORD.ORDER-LINES.LINE-NO":       {Offset: 21, Length: 2, Occurs: 3, ElementStride: 15},
		"ORDER-RECORD.ORDER-LINES.LINE-ITEM.SKU": {Offset: 23, Length: 6, Occurs: 3, ElementStride: 15},
		"ORDER-RECORD.ORDER-LINES.UNIT-PRICE":    {Offset: 32, Length: 4, Occurs: 3, Scale: 2, ElementStride: 15},
	}
	got := map[string]Field{}
	for _, f := range l.Fields() {
		got[f.Path] = f
	}
	for path, w := range want {
		g, ok := got[path]
		if !ok {
			t.Errorf("missing field %s", path)
			continue
		}
		if g.Offset != w.Offset || g.Length != w.Length || g.Occurs != w.Occurs || g.ElementStride != w.ElementStride || g.Scale != w.Scale {
			t.Errorf("%s = %+v, want %+v", path, g, w)
		}
	}
}

func TestRoundTripNestedOccurs(t *testing.T) {
	l := compileTest(t)
	in := map[string]any{
		"ORDER-ID":     "ABC-123 ",
		"ORDER-DATE":   "20261007",
		"TOTAL-AMOUNT": "-1234.56",
		"ORDER-LINES": []any{
			map[string]any{"LINE-NO": "01", "LINE-ITEM": map[string]any{"SKU": "SKU001", "QTY": "5"}, "UNIT-PRICE": "10.50"},
			map[string]any{"LINE-NO": "02", "LINE-ITEM": map[string]any{"SKU": "SKU002", "QTY": "3"}, "UNIT-PRICE": "-0.99"},
			map[string]any{"LINE-NO": "03", "LINE-ITEM": map[string]any{"SKU": "SKU003", "QTY": "1"}, "UNIT-PRICE": "99999.99"},
		},
	}
	rec, err := l.EncodeRecord(in)
	if err != nil {
		t.Fatalf("EncodeRecord: %v", err)
	}
	if len(rec) != l.RecordLength {
		t.Fatalf("encoded length %d, want %d", len(rec), l.RecordLength)
	}
	// Text padding: short SKU right-padded with spaces.
	if got := string(rec[23:29]); got != "SKU001" {
		t.Fatalf("SKU bytes = %q", got)
	}
	// COMP-3 negative: -1234.56 -> digits 000123456, sign D.
	total := rec[16:21]
	if total[4]&0x0F != 0x0D {
		t.Fatalf("TOTAL-AMOUNT sign nibble = 0x%X, want D", total[4]&0x0F)
	}
	out, err := l.DecodeRecord(rec)
	if err != nil {
		t.Fatalf("DecodeRecord: %v", err)
	}
	obj := out.(map[string]any)
	if obj["TOTAL-AMOUNT"] != "-1234.56" {
		t.Errorf("TOTAL-AMOUNT = %v", obj["TOTAL-AMOUNT"])
	}
	lines := obj["ORDER-LINES"].([]any)
	if len(lines) != 3 {
		t.Fatalf("ORDER-LINES len = %d", len(lines))
	}
	line2 := lines[1].(map[string]any)
	if line2["UNIT-PRICE"] != "-0.99" {
		t.Errorf("line2 UNIT-PRICE = %v", line2["UNIT-PRICE"])
	}
	item := line2["LINE-ITEM"].(map[string]any)
	if item["SKU"] != "SKU002" || item["QTY"] != "3" {
		t.Errorf("line2 item = %v", item)
	}
}

func TestComp3ZeroEncodesPositive(t *testing.T) {
	l := compileTest(t)
	in := map[string]any{
		"ORDER-ID": "X", "ORDER-DATE": "0", "TOTAL-AMOUNT": "-0.00",
		"ORDER-LINES": []any{
			map[string]any{"LINE-NO": "0", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "0"}, "UNIT-PRICE": "0"},
			map[string]any{"LINE-NO": "0", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "0"}, "UNIT-PRICE": "0"},
			map[string]any{"LINE-NO": "0", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "0"}, "UNIT-PRICE": "0"},
		},
	}
	rec, err := l.EncodeRecord(in)
	if err != nil {
		t.Fatalf("EncodeRecord: %v", err)
	}
	if rec[20] != 0x0C {
		t.Fatalf("zero sign nibble = 0x%X, want C", rec[20])
	}
}

func TestComp3RejectsBadData(t *testing.T) {
	l := compileTest(t)
	in := map[string]any{
		"ORDER-ID": "ID", "ORDER-DATE": "1", "TOTAL-AMOUNT": "123.45",
		"ORDER-LINES": []any{
			map[string]any{"LINE-NO": "1", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "1"}, "UNIT-PRICE": "0"},
			map[string]any{"LINE-NO": "1", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "1"}, "UNIT-PRICE": "0"},
			map[string]any{"LINE-NO": "1", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "1"}, "UNIT-PRICE": "0"},
		},
	}
	rec, encErr := l.EncodeRecord(in)
	if encErr != nil {
		t.Fatalf("EncodeRecord: %v", encErr)
	}
	if _, err := l.DecodeRecord(rec); err != nil {
		t.Fatalf("valid record failed: %v", err)
	}
	rec[20] = 0x6F // sign nibble F
	_, err := l.DecodeRecord(rec)
	if err == nil || !strings.Contains(err.Error(), "TOTAL-AMOUNT") {
		t.Fatalf("want sign nibble error with path, got %v", err)
	}
	// Invalid digit nibble A.
	rec[20] = 0x6C
	rec[18] = 0x3A
	if _, err := l.DecodeRecord(rec); err == nil {
		t.Fatal("want invalid digit error")
	}
	// Non-zero pad nibble: TOTAL-AMOUNT has 9 digits -> 5 bytes, 10 nibbles,
	// 9 digits + sign = 10, no pad. UNIT-PRICE has 7 digits -> 8 nibbles
	// including sign, no pad either. Use a direct field test instead.
}

func TestComp3PadNibble(t *testing.T) {
	root, err := Parse("01 R.\n  05 A PIC S9(4) COMP-3.\n")
	if err != nil {
		t.Fatal(err)
	}
	l, err := Compile(root)
	if err != nil {
		t.Fatal(err)
	}
	// 4 digits + sign = 5 nibbles -> 3 bytes with leading pad.
	if l.RecordLength != 3 {
		t.Fatalf("record length = %d, want 3", l.RecordLength)
	}
	rec, err := l.EncodeRecord(map[string]any{"A": "1234"})
	if err != nil {
		t.Fatal(err)
	}
	if rec[0] != 0x01 || rec[1] != 0x23 || rec[2] != 0x4C {
		t.Fatalf("encoded = % X", rec)
	}
	rec[0] = 0x11 // non-zero pad nibble
	if _, err := l.DecodeRecord(rec); err == nil || !strings.Contains(err.Error(), "pad") {
		t.Fatalf("want pad nibble error, got %v", err)
	}
}

func TestTextRules(t *testing.T) {
	l := compileTest(t)
	base := map[string]any{
		"ORDER-ID": "ID", "ORDER-DATE": "20261007", "TOTAL-AMOUNT": "0",
		"ORDER-LINES": []any{
			map[string]any{"LINE-NO": "1", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "1"}, "UNIT-PRICE": "0"},
			map[string]any{"LINE-NO": "1", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "1"}, "UNIT-PRICE": "0"},
			map[string]any{"LINE-NO": "1", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "1"}, "UNIT-PRICE": "0"},
		},
	}
	rec, err := l.EncodeRecord(base)
	if err != nil {
		t.Fatalf("padding encode: %v", err)
	}
	if string(rec[0:8]) != "ID      " {
		t.Fatalf("padded text = %q", rec[0:8])
	}
	out, _ := l.DecodeRecord(rec)
	if out.(map[string]any)["ORDER-ID"] != "ID      " {
		t.Fatal("decode must preserve trailing spaces")
	}
	base["ORDER-ID"] = "WAY-TOO-LONG-ID"
	if _, err := l.EncodeRecord(base); err == nil {
		t.Fatal("want overlong text error")
	}
	base["ORDER-ID"] = "BAD\tTAB"
	if _, err := l.EncodeRecord(base); err == nil {
		t.Fatal("want control char error")
	}
	base["ORDER-ID"] = "é"
	if _, err := l.EncodeRecord(base); err == nil {
		t.Fatal("want non-ASCII error")
	}
}

func TestNumericPrecision(t *testing.T) {
	l := compileTest(t)
	mk := func(amount string) map[string]any {
		return map[string]any{
			"ORDER-ID": "ID", "ORDER-DATE": "1", "TOTAL-AMOUNT": amount,
			"ORDER-LINES": []any{
				map[string]any{"LINE-NO": "1", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "1"}, "UNIT-PRICE": "0"},
				map[string]any{"LINE-NO": "1", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "1"}, "UNIT-PRICE": "0"},
				map[string]any{"LINE-NO": "1", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "1"}, "UNIT-PRICE": "0"},
			},
		}
	}
	for _, bad := range []string{"12345678.00", "1.001", "abc", "1,000"} {
		if _, err := l.EncodeRecord(mk(bad)); err == nil {
			t.Errorf("value %q should be rejected", bad)
		}
	}
	// Unsigned display rejects negatives.
	bad := mk("0")
	bad["ORDER-DATE"] = "-1"
	if _, err := l.EncodeRecord(bad); err == nil {
		t.Error("negative unsigned display should be rejected")
	}
}

func TestParseRejections(t *testing.T) {
	cases := map[string]string{
		"redefines":   "01 R.\n  05 A PIC X(3).\n  05 B REDEFINES A PIC X(3).\n",
		"odo":         "01 R.\n  05 A OCCURS 1 TO 5 DEPENDING ON X PIC X(2).\n",
		"level66":     "01 R.\n  66 A RENAMES B.\n",
		"level88":     "01 R.\n  05 A PIC X.\n  88 B VALUE 'Y'.\n",
		"dup-name":    "01 R.\n  05 A PIC X(2).\n  05 A PIC 9(2).\n",
		"no-top":      "05 A PIC X(2).\n",
		"unsigned-c3": "01 R.\n  05 A PIC 9(4) COMP-3.\n",
		"signed-disp": "01 R.\n  05 A PIC S9(4).\n",
		"no-period":   "01 R.\n  05 A PIC X(2)\n",
	}
	for name, src := range cases {
		if _, err := Parse(src); err == nil {
			t.Errorf("%s: expected parse error", name)
		}
	}
}

func TestEncodeStructureMismatch(t *testing.T) {
	l := compileTest(t)
	mk := func() map[string]any {
		return map[string]any{
			"ORDER-ID": "ID", "ORDER-DATE": "1", "TOTAL-AMOUNT": "0",
			"ORDER-LINES": []any{
				map[string]any{"LINE-NO": "1", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "1"}, "UNIT-PRICE": "0"},
				map[string]any{"LINE-NO": "1", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "1"}, "UNIT-PRICE": "0"},
				map[string]any{"LINE-NO": "1", "LINE-ITEM": map[string]any{"SKU": "A", "QTY": "1"}, "UNIT-PRICE": "0"},
			},
		}
	}
	short := mk()
	short["ORDER-LINES"] = short["ORDER-LINES"].([]any)[:2]
	if _, err := l.EncodeRecord(short); err == nil {
		t.Error("short array should be rejected")
	}
	extra := mk()
	extra["BOGUS"] = "x"
	if _, err := l.EncodeRecord(extra); err == nil {
		t.Error("unknown key should be rejected")
	}
	missing := mk()
	delete(missing, "TOTAL-AMOUNT")
	if _, err := l.EncodeRecord(missing); err == nil {
		t.Error("missing key should be rejected")
	}
	num := mk()
	num["TOTAL-AMOUNT"] = 12.5
	if _, err := l.EncodeRecord(num); err == nil {
		t.Error("JSON number should be rejected; use decimal strings")
	}
}
