package copybook

import (
	"bytes"
	"strings"
	"testing"
)

const testCopybook = `
01 ORDER.
   05 CUST-NAME PIC X(10).
   05 ORDER-ID PIC 9(5).
   05 LINES OCCURS 2 TIMES.
      10 ITEM PIC X(4).
      10 QTY PIC 9(3).
      10 PRICE PIC S9(3)V99 COMP-3.
   05 TOTAL PIC S9(5)V99 COMP-3.
`

func compileTest(t *testing.T) *Layout {
	t.Helper()
	fields, err := Parse(testCopybook)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	l, err := Compile(fields)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return l
}

func TestLayoutOffsets(t *testing.T) {
	l := compileTest(t)
	if l.Length != 39 {
		t.Fatalf("record length = %d, want 39", l.Length)
	}
	root := l.Root
	name, id, lines, total := root.Children[0], root.Children[1], root.Children[2], root.Children[3]
	if name.Offset != 0 || name.Length != 10 {
		t.Errorf("CUST-NAME off/len = %d/%d", name.Offset, name.Length)
	}
	if id.Offset != 10 || id.Length != 5 {
		t.Errorf("ORDER-ID off/len = %d/%d", id.Offset, id.Length)
	}
	if lines.Offset != 15 || lines.Length != 20 || lines.Count != 2 {
		t.Errorf("LINES off/len/count = %d/%d/%d", lines.Offset, lines.Length, lines.Count)
	}
	price := lines.Children[2]
	if price.Offset != 22 || price.Length != 3 || price.Scale != 2 {
		t.Errorf("PRICE off/len/scale = %d/%d/%d", price.Offset, price.Length, price.Scale)
	}
	if total.Offset != 35 || total.Length != 4 {
		t.Errorf("TOTAL off/len = %d/%d", total.Offset, total.Length)
	}
	j, err := l.LayoutJSON()
	if err != nil || !strings.Contains(string(j), `"recordLength": 39`) {
		t.Errorf("layout json: %v %s", err, j)
	}
}

func TestRoundTripNestedOccursNegativeDecimal(t *testing.T) {
	l := compileTest(t)
	jsonl := `{"CUST-NAME":"ALICE","ORDER-ID":"42","LINES":[` +
		`{"ITEM":"W1","QTY":"3","PRICE":"-12.50"},` +
		`{"ITEM":"W2","QTY":"0","PRICE":"0"}],` +
		`"TOTAL":"-37.50"}` + "\n"
	var bin bytes.Buffer
	if err := JSONLToBin(l, strings.NewReader(jsonl), &bin); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if bin.Len() != 39 {
		t.Fatalf("bin length = %d", bin.Len())
	}
	// CUST-NAME right-padded with spaces.
	if got := bin.Bytes()[0:10]; string(got) != "ALICE     " {
		t.Errorf("CUST-NAME bytes = %q", got)
	}
	// First PRICE = -12.50 -> 1250 D -> 01 25 0D.
	if got := bin.Bytes()[22:25]; !bytes.Equal(got, []byte{0x01, 0x25, 0x0D}) {
		t.Errorf("PRICE bytes = % X", got)
	}
	// Zero encoded with positive sign C.
	if got := bin.Bytes()[32:35]; !bytes.Equal(got, []byte{0x00, 0x00, 0x0C}) {
		t.Errorf("zero PRICE bytes = % X", got)
	}
	var out bytes.Buffer
	if err := BinToJSONL(l, &bin, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := out.String()
	for _, want := range []string{`"-12.50"`, `"0.00"`, `"-37.50"`, `"ALICE     "`, `"42"`} {
		if !strings.Contains(got, want) {
			t.Errorf("decoded JSONL %s missing %s", got, want)
		}
	}
}

func TestTruncatedFileFails(t *testing.T) {
	l := compileTest(t)
	var out bytes.Buffer
	err := BinToJSONL(l, bytes.NewReader(make([]byte, 40)), &out)
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("expected truncation error, got %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("partial output written")
	}
}

func TestBadComp3SignAndPad(t *testing.T) {
	l := compileTest(t)
	rec := make([]byte, 39)
	for i := 0; i < 10; i++ {
		rec[i] = ' '
	}
	copy(rec[10:15], "00001")
	for i := 15; i < 35; i++ {
		rec[i] = '0'
	}
	// Valid comp3 zero with C sign for the two PRICE fields.
	rec[24], rec[34] = 0x0C, 0x0C
	// TOTAL: F sign (unsigned) must be rejected.
	rec[38] = 0x0F
	var out bytes.Buffer
	err := BinToJSONL(l, bytes.NewReader(rec), &out)
	re, ok := err.(*RecordError)
	if !ok || re.Record != 1 || re.Path != "ORDER.TOTAL" {
		t.Fatalf("expected RecordError at ORDER.TOTAL, got %v", err)
	}
	// Non-zero pad nibble on even-digit COMP-3 (TOTAL has 7 digits -> odd, use PRICE: 5 digits odd too).
	// Make a 4-digit comp3 field to test padding.
	fields, _ := Parse("01 R.\n 05 A PIC S9(4) COMP-3.\n")
	l2, err := Compile(fields)
	if err != nil {
		t.Fatal(err)
	}
	err = BinToJSONL(l2, bytes.NewReader([]byte{0x12, 0x34, 0x5C}), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "pad") {
		t.Fatalf("expected pad nibble error, got %v", err)
	}
}

func TestDefinitionErrors(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"duplicate", "01 R.\n 05 A PIC X(1).\n 05 A PIC X(1).\n", "3:2"},
		{"redefines", "01 R.\n 05 A PIC X(1).\n 05 B REDEFINES A.\n", "REDEFINES"},
		{"occurs depending", "01 R.\n 05 A PIC X(1) OCCURS DEPENDING ON B.\n", "DEPENDING"},
		{"filler", "01 R.\n 05 FILLER PIC X(1).\n", "FILLER"},
		{"bad level", "01 R.\n 77 A PIC X(1).\n", "level"},
		{"group as scalar", "01 R.\n 05 G.\n 05 B PIC X(1).\n", "no PIC clause"},
		{"group with pic", "01 R.\n 05 G PIC X(3).\n 10 A PIC X(1).\n", "both PIC and subordinate"},
		{"signed display", "01 R.\n 05 A PIC S9(3).\n", "signed display"},
		{"unsigned comp3", "01 R.\n 05 A PIC 9(3) COMP-3.\n", "must be signed"},
		{"two records", "01 A PIC X(1).\n01 B PIC X(1).\n", "multiple level-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields, err := Parse(tc.src)
			if err == nil {
				_, err = Compile(fields)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestValueValidation(t *testing.T) {
	l := compileTest(t)
	base := `{"CUST-NAME":"A","ORDER-ID":"1","LINES":[` +
		`{"ITEM":"W","QTY":"1","PRICE":"1"},` +
		`{"ITEM":"W","QTY":"1","PRICE":"1"}],"TOTAL":"1"}`
	cases := []struct {
		name, mutate, want string
	}{
		{"text too long", `"CUST-NAME":"ABCDEFGHIJK"`, "exceeds field length"},
		{"non-ascii", `"CUST-NAME":"caf\u00e9"`, "non-ASCII"},
		{"too many int digits", `"ORDER-ID":"123456"`, "exceeds 5 integer digits"},
		{"too precise", `"PRICE":"1.001"`, "fractional digits"},
		{"unsigned negative", `"QTY":"-1"`, "unsigned"},
		{"wrong array len", `"LINES":[{"ITEM":"W","QTY":"1","PRICE":"1"}]`, "OCCURS requires 2"},
		{"unknown field", `"NOPE":"1"`, "unknown field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(base, `"CUST-NAME":"A"`, `"CUST-NAME":"A"`, 1)
			var line string
			switch tc.name {
			case "text too long", "non-ascii":
				line = strings.Replace(base, `"CUST-NAME":"A"`, tc.mutate, 1)
			case "too many int digits":
				line = strings.Replace(base, `"ORDER-ID":"1"`, tc.mutate, 1)
			case "too precise":
				line = strings.Replace(base, `"PRICE":"1"}],"TOTAL"`, tc.mutate+`}],"TOTAL"`, 1)
			case "unsigned negative":
				line = strings.Replace(base, `"QTY":"1","PRICE":"1"}],"TOTAL"`, `"QTY":"-1","PRICE":"1"}],"TOTAL"`, 1)
			case "wrong array len":
				line = strings.Replace(base, `"LINES":[`+"{"+`"ITEM":"W","QTY":"1","PRICE":"1"},`+"{"+`"ITEM":"W","QTY":"1","PRICE":"1"}]`, tc.mutate, 1)
			case "unknown field":
				line = strings.Replace(base, `"TOTAL":"1"`, `"TOTAL":"1","NOPE":"1"`, 1)
			}
			_ = src
			var out bytes.Buffer
			err := JSONLToBin(l, strings.NewReader(line+"\n"), &out)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want substring %q", err, tc.want)
			}
			if out.Len() != 0 {
				t.Errorf("partial output written")
			}
		})
	}
}
