package bencode

import "testing"

func TestDecodeValid(t *testing.T) {
	v, err := Decode([]byte("d1:ai1e1:bli2e3:abce1:ci-3ee"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Kind != Dict || len(v.Dict) != 3 {
		t.Fatalf("got %+v", v)
	}
	if v.Get("a").Int != 1 {
		t.Fatal("a")
	}
	l := v.Get("b")
	if l.Kind != List || l.List[0].Int != 2 || string(l.List[1].Str) != "abc" {
		t.Fatal("b")
	}
	if v.Get("c").Int != -3 {
		t.Fatal("c")
	}
}

func TestRejectTruncated(t *testing.T) {
	for _, s := range []string{"i12", "3:ab", "d1:ai1e", "li1e", "4:abc", "i", "d1:a"} {
		if _, err := Decode([]byte(s)); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func TestRejectBadIntegers(t *testing.T) {
	for _, s := range []string{"i03e", "i-0e", "i-e", "ie", "i1a2e", "i99999999999999999999e"} {
		if _, err := Decode([]byte(s)); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
	if _, err := Decode([]byte("i0e")); err != nil {
		t.Error("i0e should be valid")
	}
}

func TestRejectTrailing(t *testing.T) {
	if _, err := Decode([]byte("i1ei2e")); err == nil {
		t.Error("expected trailing data error")
	}
}

func TestDuplicateAndUnsortedKeys(t *testing.T) {
	v, err := Decode([]byte("d1:ai1e1:ai2e1:bd1:z1:x1:y1:wee"))
	if err != nil {
		t.Fatal(err)
	}
	dups := DuplicateKeys(v)
	if len(dups) != 1 || dups[0] != "/a" {
		t.Fatalf("dups: %v", dups)
	}
	uns := UnsortedKeys(v)
	if len(uns) != 1 || uns[0] != "/b" {
		t.Fatalf("unsorted: %v", uns)
	}
}

func TestRawPositions(t *testing.T) {
	data := []byte("d4:infod1:ai1eee")
	v, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	info := v.Get("info")
	if got := string(data[info.Start:info.End]); got != "d1:ai1ee" {
		t.Fatalf("raw info = %q", got)
	}
}
