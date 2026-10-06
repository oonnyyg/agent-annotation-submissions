// Package isbn validates and normalizes ISBN-10 / ISBN-13 values.
package isbn

import "strings"

// Normalize strips spaces and hyphens, validates the ISBN-10 or ISBN-13
// check digit, and returns the canonical ISBN-13 form. ISBN-10 values are
// converted to the 978 prefix; ISBN-13 values must start with 978 or 979.
// ok is false for anything invalid.
func Normalize(raw string) (canonical string, ok bool) {
	s := strings.NewReplacer(" ", "", "-", "").Replace(raw)
	switch len(s) {
	case 10:
		if !valid10(s) {
			return "", false
		}
		core := "978" + s[:9]
		return core + string(check13(core)), true
	case 13:
		if !strings.HasPrefix(s, "978") && !strings.HasPrefix(s, "979") {
			return "", false
		}
		if !valid13(s) {
			return "", false
		}
		return s, true
	}
	return "", false
}

func valid10(s string) bool {
	sum := 0
	for i := 0; i < 10; i++ {
		c := s[i]
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c == 'X' && i == 9:
			d = 10
		default:
			return false
		}
		sum += d * (10 - i)
	}
	return sum%11 == 0
}

func valid13(s string) bool {
	sum := 0
	for i := 0; i < 13; i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return sum%10 == 0
}

func check13(first12 string) byte {
	sum := 0
	for i := 0; i < 12; i++ {
		d := int(first12[i] - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	return byte('0' + (10-sum%10)%10)
}
