package decimal

import (
	"fmt"
	"strings"
)

// Decimal is a finite base-10 number kept as a sign and two integer parts so
// comparisons never pass through floating point. Value = sign * intPart +
// sign * fracPart / 10^fracScale; fracPart has no trailing zeros.
type Decimal struct {
	neg       bool
	intPart   string // canonical digits, no leading zeros ("0" allowed)
	fracPart  string // canonical digits, no trailing zeros ("" for integers)
}

// Parse accepts optional sign, digits, and optional "." fractional part.
// Exponents and blank strings are rejected; the caller treats empty as null.
func Parse(s string) (Decimal, error) {
	raw := s
	neg := false
	switch {
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	case strings.HasPrefix(s, "-"):
		neg = true
		s = s[1:]
	}
	if s == "" {
		return Decimal{}, fmt.Errorf("invalid decimal %q", raw)
	}
	intPart := s
	fracPart := ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		intPart = s[:dot]
		fracPart = s[dot+1:]
		if strings.IndexByte(fracPart, '.') >= 0 {
			return Decimal{}, fmt.Errorf("invalid decimal %q", raw)
		}
	}
	if intPart == "" {
		intPart = "0"
	}
	if !allDigits(intPart) || (fracPart != "" && !allDigits(fracPart)) {
		return Decimal{}, fmt.Errorf("invalid decimal %q", raw)
	}
	intPart = strings.TrimLeft(intPart, "0")
	if intPart == "" {
		intPart = "0"
	}
	fracPart = strings.TrimRight(fracPart, "0")
	zero := intPart == "0" && fracPart == ""
	if zero {
		neg = false
	}
	return Decimal{neg: neg, intPart: intPart, fracPart: fracPart}, nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Compare returns -1, 0, or 1 like bytes.Compare.
func Compare(a, b Decimal) int {
	if a.neg != b.neg {
		if a.neg {
			return -1
		}
		return 1
	}
	mag := compareMag(a, b)
	if a.neg {
		return -mag
	}
	return mag
}

func compareMag(a, b Decimal) int {
	if len(a.intPart) != len(b.intPart) {
		if len(a.intPart) < len(b.intPart) {
			return -1
		}
		return 1
	}
	if a.intPart != b.intPart {
		if a.intPart < b.intPart {
			return -1
		}
		return 1
	}
	fa, fb := a.fracPart, b.fracPart
	if len(fa) != len(fb) {
		max := len(fa)
		if len(fb) > max {
			max = len(fb)
		}
		fa = padRight(fa, max)
		fb = padRight(fb, max)
	}
	if fa == fb {
		return 0
	}
	if fa < fb {
		return -1
	}
	return 1
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	var b strings.Builder
	b.Grow(n)
	b.WriteString(s)
	for i := len(s); i < n; i++ {
		b.WriteByte('0')
	}
	return b.String()
}
