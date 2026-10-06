package marc

import (
	"bytes"
	"strings"
	"testing"
)

func TestNormalizeISBN(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"978-7-02-000220-7", "9787020002207", true},
		{"0-306-40615-2", "9780306406157", true},   // ISBN-10 -> 978
		{"0-8044-2957-X", "9780804429573", true},   // trailing X
		{"979-10-90636-07-1", "9791090636071", true},
		{"9787020002208", "", false},               // bad check digit
		{"9771234567890", "", false},               // 977 prefix not allowed
		{"0-306-40615-3", "", false},               // bad ISBN-10 check
		{"978702000220", "", false},                // wrong length
		{"97870200022X7", "", false},               // X not allowed in 13
	}
	for _, c := range cases {
		got, ok := NormalizeISBN(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("NormalizeISBN(%q) = %q,%v want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestTransitiveAssociationAndConflict(t *testing.T) {
	// A-B share one ISBN, B-C share another: all three form one group.
	mk := func(title string, isbns ...string) []byte {
		var sb strings.Builder
		for _, isbn := range isbns {
			sb.WriteString("  \x1fa" + isbn)
		}
		return buildRecord(t, []testField{
			{"245", "10\x1fa" + title},
			{"020", sb.String()},
		}, nil)
	}
	a := mk("同一题名", "0-306-40615-2")
	b := mk("同一  题名", "9780306406157", "9787020002207")
	c := mk("不同题名", "978-7-02-000220-7")
	d := mk("孤立记录", "9787020002208") // invalid ISBN: no association

	records, bad, stopErr := ReadAll(bytes.NewReader(concat(a, b, c, d)))
	if stopErr != nil || len(bad) != 0 {
		t.Fatalf("bad=%v stop=%v", bad, stopErr)
	}
	if len(records) != 4 {
		t.Fatalf("got %d records", len(records))
	}
	entries := make([]Entry, len(records))
	for i, r := range records {
		entries[i] = ExtractEntry(r, i)
	}
	groups := BuildGroups(entries)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	g := groups[0]
	if len(g.Members) != 3 {
		t.Fatalf("group members = %v", g.Members)
	}
	// "同一题名" vs "同一  题名" collapse to equal; "不同题名" conflicts.
	if !g.Conflict {
		t.Error("expected conflict for differing titles")
	}
	if groups[1].Conflict {
		t.Error("singleton group should not conflict")
	}
	if len(entries[3].ISBNs) != 0 || len(entries[3].InvalidISBNs) != 1 {
		t.Errorf("invalid ISBN should not associate: %+v", entries[3])
	}
}

func TestBadRecordIsolationAndStop(t *testing.T) {
	good := buildRecord(t, []testField{{"001", "ok1"}}, nil)
	// Corrupt: valid leader length but field missing terminator.
	badRec := buildRecord(t, []testField{{"001", "bad"}}, nil)
	badRec[len(badRec)-2] = 'X' // break the field terminator
	good2 := buildRecord(t, []testField{{"001", "ok2"}}, nil)

	records, bad, stopErr := ReadAll(bytes.NewReader(concat(good, badRec, good2)))
	if stopErr != nil {
		t.Fatalf("unexpected stop: %v", stopErr)
	}
	if len(records) != 2 || len(bad) != 1 {
		t.Fatalf("records=%d bad=%d", len(records), len(bad))
	}
	if bad[0].Offset != len(good) || bad[0].Length != len(badRec) {
		t.Errorf("bad record boundary = %+v", bad[0])
	}
	if ExtractEntry(records[1], 1).ControlNumber != "ok2" {
		t.Error("did not resume at correct record after isolation")
	}

	// Non-UTF-8 record is rejected but isolated by its leader length.
	notUTF8 := buildRecord(t, []testField{{"245", "10\x1fa\xff\xfe"}}, nil)
	records, bad, stopErr = ReadAll(bytes.NewReader(concat(notUTF8, good)))
	if stopErr != nil || len(records) != 1 || len(bad) != 1 {
		t.Fatalf("utf8 case: records=%d bad=%d stop=%v", len(records), len(bad), stopErr)
	}

	// Implausible leader length: stop and report position.
	garbage := append(concat(good), []byte("xxxxx")...)
	records, bad, stopErr = ReadAll(bytes.NewReader(garbage))
	if stopErr == nil {
		t.Fatal("expected stop on implausible leader length")
	}
	pe, ok := stopErr.(*ParseError)
	if !ok || pe.Offset != len(good) {
		t.Errorf("stop error = %v", stopErr)
	}
	if len(records) != 1 {
		t.Errorf("records before stop = %d", len(records))
	}
}

func concat(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}
