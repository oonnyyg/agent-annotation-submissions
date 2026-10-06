package marc

import "strings"

// NormalizeISBN strips spaces and hyphens, validates the ISBN-10 or
// ISBN-13 check digit, and returns the canonical 13-digit form.
// ISBN-10 may end in X. ISBN-13 must start with 978 or 979.
// It returns false for anything invalid; invalid numbers never
// establish record associations.
func NormalizeISBN(raw string) (string, bool) {
	s := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, raw)
	switch len(s) {
	case 10:
		if !validISBN10(s) {
			return "", false
		}
		return isbn10to13(s), true
	case 13:
		if !strings.HasPrefix(s, "978") && !strings.HasPrefix(s, "979") {
			return "", false
		}
		if !validISBN13(s) {
			return "", false
		}
		return s, true
	}
	return "", false
}

func validISBN10(s string) bool {
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

func validISBN13(s string) bool {
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

func isbn10to13(s string) string {
	core := "978" + s[:9]
	sum := 0
	for i := 0; i < 12; i++ {
		d := int(core[i] - '0')
		if i%2 == 1 {
			d *= 3
		}
		sum += d
	}
	check := (10 - sum%10) % 10
	return core + string(rune('0'+check))
}
