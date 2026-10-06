package tzif

import (
	"strings"
	"testing"
)

func goodFile() []byte {
	locals := []LocalTime{{Offset: 0, Abbr: "UTC"}, {Offset: 3600, Abbr: "X"}}
	return buildTZif('3', []int64{1000, 2000}, []int{0, 1}, locals, 0)
}

func TestParseV3(t *testing.T) {
	if _, err := Parse(goodFile()); err != nil {
		t.Fatal(err)
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func([]byte) []byte
		want   string
	}{
		{"truncated", func(b []byte) []byte { return b[:len(b)-3] }, "truncated"},
		{"bad magic", func(b []byte) []byte { b[0] = 'X'; return b }, "bad magic"},
		{"bad version", func(b []byte) []byte { b[4] = '1'; return b }, "unsupported version"},
		{"leap seconds", func(b []byte) []byte {
			return buildTZif('2', []int64{1000, 2000}, []int{0, 1},
				[]LocalTime{{Offset: 0, Abbr: "UTC"}, {Offset: 3600, Abbr: "X"}}, 1)
		}, "leap-second"},
		{"unsorted", func(b []byte) []byte {
			return buildTZif('2', []int64{2000, 1000}, []int{0, 1},
				[]LocalTime{{Offset: 0, Abbr: "UTC"}, {Offset: 3600, Abbr: "X"}}, 0)
		}, "not strictly after"},
		{"too few transitions", func(b []byte) []byte {
			return buildTZif('2', []int64{1000}, []int{0},
				[]LocalTime{{Offset: 0, Abbr: "UTC"}}, 0)
		}, "at least two"},
		{"type index out of range", func(b []byte) []byte {
			return buildTZif('2', []int64{1000, 2000}, []int{0, 5},
				[]LocalTime{{Offset: 0, Abbr: "UTC"}, {Offset: 3600, Abbr: "X"}}, 0)
		}, "references type index"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.mutate(goodFile()))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not contain %q", err, c.want)
			}
			pe, ok := err.(*ParseError)
			if !ok {
				t.Fatalf("error is %T, want *ParseError", err)
			}
			if pe.Offset < 0 || pe.Offset > len(c.mutate(goodFile())) {
				t.Fatalf("byte offset %d out of range", pe.Offset)
			}
		})
	}
}