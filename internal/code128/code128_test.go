package code128

import (
	"reflect"
	"testing"
)

func roundtrip(t *testing.T, text string) *DecodeResult {
	t.Helper()
	seq, err := Encode(text)
	if err != nil {
		t.Fatalf("Encode(%q): %v", text, err)
	}
	w, err := Widths(seq)
	if err != nil {
		t.Fatalf("Widths: %v", err)
	}
	res, err := Decode(w)
	if err != nil {
		t.Fatalf("Decode(%q widths): %v", text, err)
	}
	if res.Text != text {
		t.Fatalf("roundtrip %q got %q", text, res.Text)
	}
	if res.ExtraCodewords != 0 {
		t.Fatalf("optimal encoding of %q reported %d extra codewords", text, res.ExtraCodewords)
	}
	return res
}

func TestRoundtrip(t *testing.T) {
	for _, s := range []string{
		"A", " ", "~", "Hello, World!", "123456", "12345", "1",
		"AB1234CD", "12345678901234567890", "A1", "12A34", "00",
		"ABC 123 XYZ ~",
	} {
		roundtrip(t, s)
	}
}

func TestOddDigitString(t *testing.T) {
	// "12345": optimal is Start C, 12, 34, Code B, '5' -> 5 codewords
	// (start + 3 data/latch + checksum + stop = 7 total).
	seq, err := Encode("12345")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{ValueStartC, 12, 34, ValueCodeB, 21}
	if !reflect.DeepEqual(seq[:5], want) {
		t.Fatalf("got %v, want prefix %v", seq, want)
	}
	if len(seq) != 7 {
		t.Fatalf("len=%d, want 7", len(seq))
	}
}

func TestNoGreedySwitch(t *testing.T) {
	// "A12": switching to C for one pair costs latch C + pair + latch B = 3
	// codewords vs 2 in plain B; must stay in B.
	seq, err := Encode("A12")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{ValueStartB, 33, 17, 18}
	if !reflect.DeepEqual(seq[:4], want) {
		t.Fatalf("got %v, want prefix %v", seq, want)
	}
}

func TestLexicographicTie(t *testing.T) {
	// "1234AB": Start C (12,34,latchB) and Start B ('1','2','3','4',latchC? no)
	// Both options: C-start uses 1+3 data, B-start uses 4 data + ... B path:
	// '1''2''3''4''A''B' = 6, or latch C for 1234 then latch B = 1+2+1+2=6.
	// C-start total data = 4 (12,34,CodeB,'A','B' -> 5). C-start wins.
	seq, err := Encode("1234AB")
	if err != nil {
		t.Fatal(err)
	}
	if seq[0] != ValueStartC {
		t.Fatalf("start = %d, want Start C", seq[0])
	}
}

func TestChecksumAppended(t *testing.T) {
	seq, err := Encode("AB")
	if err != nil {
		t.Fatal(err)
	}
	// start=104, 'A'=33, 'B'=34 -> sum = 104 + 1*33 + 2*34 = 205; 205%103 = 102...
	// 205-103=102. checksum=102? But 102 is FNC1; as checksum it is fine.
	if got := seq[len(seq)-2]; got != Checksum(seq[:len(seq)-2]) {
		t.Fatalf("checksum %d inconsistent", got)
	}
	if seq[len(seq)-1] != ValueStop {
		t.Fatalf("last codeword %d, want stop", seq[len(seq)-1])
	}
}

func TestChecksumValueMayEqualLatch(t *testing.T) {
	// Find a short text whose checksum equals 99 or 100 and ensure decode
	// does not treat it as a latch.
	for _, target := range []int{ValueCodeC, ValueCodeB} {
		found := false
		for a := 0; a < 95 && !found; a++ {
			for b := 0; b < 95; b++ {
				text := string([]byte{byte(a + 32), byte(b + 32)})
				seq, err := Encode(text)
				if err != nil {
					t.Fatal(err)
				}
				if seq[len(seq)-2] == target {
					res := roundtrip(t, text)
					if res.Text != text {
						t.Fatalf("checksum=%d case decoded to %q", target, res.Text)
					}
					found = true
					break
				}
			}
		}
		if !found {
			t.Fatalf("no 2-char text with checksum %d", target)
		}
	}
}

func TestCorruptedStop(t *testing.T) {
	seq, _ := Encode("TEST")
	w, _ := Widths(seq)
	w[len(w)-1] = 3 // last module of stop should be 2
	_, err := Decode(w)
	de, ok := err.(*DecodeError)
	if !ok || de.Kind != "module" {
		t.Fatalf("got %v, want module DecodeError", err)
	}
}

func TestChecksumMismatch(t *testing.T) {
	seq, _ := Encode("TEST")
	w, _ := Widths(seq)
	// Corrupt a middle module so it still maps to a valid codeword but the
	// checksum fails: flip the checksum codeword by re-encoding with a
	// different checksum value.
	seq[len(seq)-2] = (seq[len(seq)-2] + 1) % 103
	w2, _ := Widths(seq)
	_, err := Decode(w2)
	de, ok := err.(*DecodeError)
	if !ok || de.Kind != "checksum" {
		t.Fatalf("got %v, want checksum DecodeError", err)
	}
	_ = w
}

func TestNonOptimalStillDecodes(t *testing.T) {
	// Encode "1234" sub-optimally: Start B, '1','2','3','4' (4 data) vs
	// optimal Start C, 12, 34 (2 data) -> extra = 2.
	seq := []int{ValueStartB, 17, 18, 19, 20}
	seq = append(seq, Checksum(seq), ValueStop)
	w, err := Widths(seq)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Decode(w)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "1234" {
		t.Fatalf("text = %q", res.Text)
	}
	if res.ExtraCodewords != 2 {
		t.Fatalf("extra = %d, want 2", res.ExtraCodewords)
	}
}

func TestMidSymbolSwitch(t *testing.T) {
	// Manually built: Start B, 'A', Code C, 12, 34, Code B, 'z'.
	seq := []int{ValueStartB, 33, ValueCodeC, 12, 34, ValueCodeB, 90}
	seq = append(seq, Checksum(seq), ValueStop)
	w, _ := Widths(seq)
	res, err := Decode(w)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "A1234z" {
		t.Fatalf("text = %q", res.Text)
	}
}

func TestEncodeRejectsBadChars(t *testing.T) {
	for _, s := range []string{"", "tab\there", "caf\u00e9", "\x7f"} {
		if _, err := Encode(s); err == nil {
			t.Fatalf("Encode(%q) succeeded, want error", s)
		}
	}
}

func TestDecodeRejectsBadStart(t *testing.T) {
	seq := []int{ValueStartA, 33}
	seq = append(seq, Checksum(seq), ValueStop)
	w, _ := Widths(seq)
	_, err := Decode(w)
	de, ok := err.(*DecodeError)
	if !ok || de.Kind != "codeword" {
		t.Fatalf("got %v, want codeword DecodeError", err)
	}
}
