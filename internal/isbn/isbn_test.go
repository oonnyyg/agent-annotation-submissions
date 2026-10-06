package isbn

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"0-306-40615-2", "9780306406157", true}, // valid ISBN-10 with hyphens
		{"0306406152", "9780306406157", true},
		{"0 8044 2957 X", "9780804429573", true}, // trailing X check digit
		{"9780306406157", "9780306406157", true}, // already 13
		{"979-10-90636-07-1", "9791090636071", true},
		{"0306406153", "", false}, // bad check digit
		{"9780306406158", "", false},
		{"1234567890", "", false}, // invalid check
		{"080442957X", "9780804429573", true},
		{"9770306406157", "", false}, // 977 prefix not allowed
		{"03064061520", "", false},   // 11 digits
		{"abcdefghij", "", false},
	}
	for _, c := range cases {
		got, ok := Normalize(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Normalize(%q) = %q,%v want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}
