package label128

import (
	"reflect"
	"testing"
)

func mustEncode(t *testing.T, text string) ([]int, []int) {
	t.Helper()
	codes, widths, err := Encode(text)
	if err != nil {
		t.Fatalf("Encode(%q): %v", text, err)
	}
	return codes, widths
}

func TestEncodeKnownVector(t *testing.T) {
	// Wikipedia worked example: HI345678 with check value 68.
	codes, _ := mustEncode(t, "HI345678")
	want := []int{104, 40, 41, 99, 34, 56, 78, 68, 106}
	if !reflect.DeepEqual(codes, want) {
		t.Fatalf("codes = %v, want %v", codes, want)
	}
}

func TestEncodeOptimalChoice(t *testing.T) {
	cases := []struct {
		text string
		want []int // start + data/switch only
	}{
		{"1234", []int{105, 12, 34}},
		{"12", []int{105, 12}},
		// Odd digit run: tie between B-only and C-then-B; the
		// lexicographically smaller sequence (start B) wins.
		{"123", []int{104, 17, 18, 19}},
		{"12345", []int{104, 17, 99, 23, 45}},
		// Mid-string switch pays off only for longer digit runs.
		{"AB123456CD", []int{104, 33, 34, 99, 12, 34, 56, 100, 35, 36}},
		{"A", []int{104, 33}},
		{"~", []int{104, 94}},
		{" ", []int{104, 0}},
	}
	for _, tc := range cases {
		codes, _ := mustEncode(t, tc.text)
		got := codes[:len(codes)-2]
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Encode(%q) body = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestEncodeWidths(t *testing.T) {
	_, widths := mustEncode(t, "A")
	// 3 symbols of 6 widths plus the 7-width stop symbol.
	if len(widths) != 25 {
		t.Fatalf("len(widths) = %d, want 25", len(widths))
	}
	tail := widths[len(widths)-7:]
	if !reflect.DeepEqual(tail, []int{2, 3, 3, 1, 1, 1, 2}) {
		t.Fatalf("stop widths = %v", tail)
	}
}

func TestEncodeRejectsInvalid(t *testing.T) {
	for _, text := range []string{"", "é", "a\x7f", "tab\there"} {
		if _, _, err := Encode(text); err == nil {
			t.Errorf("Encode(%q) succeeded, want error", text)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	texts := []string{
		"A", "123456", "123", "ABC1234def99", "~~~~~~~~",
		"CODE 128 label audit 2026!", "0000", "9",
	}
	for _, text := range texts {
		_, widths := mustEncode(t, text)
		res, err := Decode(widths)
		if err != nil {
			t.Fatalf("Decode(Encode(%q)): %v", text, err)
		}
		if res.Text != text {
			t.Errorf("round trip %q gave %q", text, res.Text)
		}
		if res.Extra != 0 {
			t.Errorf("round trip %q reports extra=%d", text, res.Extra)
		}
	}
}

func TestDecodeChecksumEqualToSwitchCode(t *testing.T) {
	// Find a text whose checksum value is a switch code (99 or 100);
	// it must still decode as a checksum, not a code-set switch.
	found := false
	for i := 0; i < 200 && !found; i++ {
		text := "X" + string(rune('0'+i%10)) + string(rune('A'+i%26))
		codes, widths := mustEncode(t, text)
		if codes[len(codes)-2] == 99 || codes[len(codes)-2] == 100 {
			found = true
			res, err := Decode(widths)
			if err != nil {
				t.Fatalf("Decode with switch-valued checksum: %v", err)
			}
			if res.Text != text {
				t.Fatalf("got %q, want %q", res.Text, text)
			}
		}
	}
	if !found {
		t.Fatal("no test text with switch-valued checksum found")
	}
}

func widthsOf(codes []int) []int {
	var widths []int
	for _, c := range codes {
		widths = append(widths, patternOf(c)...)
	}
	return widths
}

func TestDecodeNonMinimalReportsExtra(t *testing.T) {
	// "1234" encoded entirely in set B: legal but 2 codewords long.
	body := []int{104, 17, 18, 19, 20}
	full := append(append([]int{}, body...), checksum(body), 106)
	res, err := Decode(widthsOf(full))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if res.Text != "1234" {
		t.Fatalf("text = %q", res.Text)
	}
	if res.Extra != 2 {
		t.Fatalf("extra = %d, want 2", res.Extra)
	}
}

func TestDecodeDamagedStop(t *testing.T) {
	_, widths := mustEncode(t, "ABC")
	widths[len(widths)-1] = 1 // corrupt the stop symbol's final bar
	_, err := Decode(widths)
	if err == nil {
		t.Fatal("expected stop mismatch error")
	}
	de, ok := err.(*DecodeError)
	if !ok || de.Module != len(widths)-1 {
		t.Fatalf("error = %v, want module %d", err, len(widths)-1)
	}
}

func TestDecodeChecksumMismatch(t *testing.T) {
	_, widths := mustEncode(t, "ABC")
	// Corrupt a width inside the checksum symbol (third symbol).
	widths[12] = 9
	if _, err := Decode(widths); err == nil {
		t.Fatal("expected checksum or pattern error")
	}
}

func TestDecodeStructuralErrors(t *testing.T) {
	if _, err := Decode([]int{2, 1, 1}); err == nil {
		t.Error("short sequence accepted")
	}
	// Valid length but not 13 + 11k.
	if _, err := Decode(make([]int, 47)); err == nil {
		t.Error("bad length accepted")
	}
	// Start code A is unsupported.
	body := []int{103, 33}
	full := append(append([]int{}, body...), checksum(body), 106)
	if _, err := Decode(widthsOf(full)); err == nil {
		t.Error("start A accepted")
	}
}
