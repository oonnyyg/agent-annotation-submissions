package copybook

import (
	"fmt"
	"math/big"
	"strings"
)

// encodeText encodes an ASCII string into buf, right-padding with spaces.
func encodeText(buf []byte, s string) error {
	if len(s) > len(buf) {
		return fmt.Errorf("text %q exceeds field length %d", s, len(buf))
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c > 0x7e {
			return fmt.Errorf("text contains non-printable or non-ASCII byte 0x%02x", c)
		}
		buf[i] = c
	}
	for i := len(s); i < len(buf); i++ {
		buf[i] = ' '
	}
	return nil
}

func decodeText(buf []byte) (string, error) {
	for i, c := range buf {
		if c < 0x20 || c > 0x7e {
			return "", fmt.Errorf("byte %d: non-printable or non-ASCII byte 0x%02x", i, c)
		}
	}
	return string(buf), nil
}

// parseDecimal parses a decimal string with the given scale into a scaled
// integer. It rejects any value that would lose precision.
func parseDecimal(s string, digits, scale int, signed bool) (*big.Int, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return nil, fmt.Errorf("empty numeric value")
	}
	neg := false
	if t[0] == '+' || t[0] == '-' {
		neg = t[0] == '-'
		t = t[1:]
	}
	if neg && !signed {
		return nil, fmt.Errorf("negative value %q for unsigned field", s)
	}
	intPart, fracPart := t, ""
	if idx := strings.IndexByte(t, '.'); idx >= 0 {
		intPart, fracPart = t[:idx], t[idx+1:]
	}
	if intPart == "" && fracPart == "" {
		return nil, fmt.Errorf("invalid numeric value %q", s)
	}
	for _, c := range intPart + fracPart {
		if c < '0' || c > '9' {
			return nil, fmt.Errorf("invalid numeric value %q", s)
		}
	}
	if len(fracPart) > scale {
		return nil, fmt.Errorf("value %q has more than %d decimal places", s, scale)
	}
	fracPart += strings.Repeat("0", scale-len(fracPart))
	intDigits := strings.TrimLeft(intPart, "0")
	if len(intDigits) > digits {
		return nil, fmt.Errorf("value %q exceeds %d integer digits", s, digits)
	}
	scaled, ok := new(big.Int).SetString(intPart+fracPart, 10)
	if !ok {
		return nil, fmt.Errorf("invalid numeric value %q", s)
	}
	if neg && scaled.Sign() != 0 {
		scaled.Neg(scaled)
	}
	return scaled, nil
}

// formatScaled renders a scaled integer as a decimal string with the given
// number of implied decimal places.
func formatScaled(v *big.Int, scale int) string {
	neg := v.Sign() < 0
	abs := new(big.Int).Abs(v)
	digits := abs.String()
	if scale == 0 {
		if neg {
			return "-" + digits
		}
		return digits
	}
	if len(digits) <= scale {
		digits = strings.Repeat("0", scale-len(digits)+1) + digits
	}
	out := digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
	if neg {
		return "-" + out
	}
	return out
}

// encodeDisplay encodes an unsigned zoned decimal, zero-padded on the left.
func encodeDisplay(buf []byte, s string, digits, scale int) error {
	v, err := parseDecimal(s, digits, scale, false)
	if err != nil {
		return err
	}
	str := v.String()
	if len(str) > len(buf) {
		return fmt.Errorf("value %q does not fit in %d digits", s, len(buf))
	}
	for i := range buf {
		buf[i] = '0'
	}
	copy(buf[len(buf)-len(str):], str)
	return nil
}

func decodeDisplay(buf []byte, digits, scale int) (string, error) {
	for i, c := range buf {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("byte %d: invalid display digit 0x%02x", i, c)
		}
	}
	v, ok := new(big.Int).SetString(string(buf), 10)
	if !ok {
		return "", fmt.Errorf("invalid display value")
	}
	return formatScaled(v, scale), nil
}

// encodeComp3 encodes a signed packed decimal. Zero is always written with
// the positive sign nibble 0xC.
func encodeComp3(buf []byte, s string, digits, scale int) error {
	v, err := parseDecimal(s, digits, scale, true)
	if err != nil {
		return err
	}
	sign := byte(0x0C)
	if v.Sign() < 0 {
		sign = 0x0D
		v = new(big.Int).Abs(v)
	}
	str := v.String()
	total := digits + scale
	if len(str) > total {
		return fmt.Errorf("value %q does not fit in %d digits", s, total)
	}
	str = strings.Repeat("0", total-len(str)) + str
	// total+1 nibbles including the sign; pad left with a zero nibble when
	// the digit count is even.
	nibbles := make([]byte, 0, total+1)
	for i := 0; i < len(str); i++ {
		nibbles = append(nibbles, str[i]-'0')
	}
	nibbles = append(nibbles, sign)
	if len(nibbles)%2 != 0 {
		nibbles = append([]byte{0}, nibbles...)
	}
	if len(nibbles)/2 != len(buf) {
		return fmt.Errorf("internal size mismatch for comp-3 field")
	}
	for i := range buf {
		buf[i] = nibbles[2*i]<<4 | nibbles[2*i+1]
	}
	return nil
}

func decodeComp3(buf []byte, digits, scale int) (string, error) {
	total := digits + scale
	nibbles := make([]byte, 0, 2*len(buf))
	for _, b := range buf {
		nibbles = append(nibbles, b>>4, b&0x0F)
	}
	sign := nibbles[len(nibbles)-1]
	if sign != 0x0C && sign != 0x0D {
		return "", fmt.Errorf("invalid comp-3 sign nibble 0x%X (want C or D)", sign)
	}
	digitNibbles := nibbles[:len(nibbles)-1]
	if len(digitNibbles) > total {
		// Even digit count: leading pad nibble must be zero.
		if digitNibbles[0] != 0 {
			return "", fmt.Errorf("non-zero pad nibble 0x%X", digitNibbles[0])
		}
		digitNibbles = digitNibbles[1:]
	}
	var sb strings.Builder
	for _, n := range digitNibbles {
		if n > 9 {
			return "", fmt.Errorf("invalid comp-3 digit nibble 0x%X", n)
		}
		sb.WriteByte('0' + n)
	}
	v, ok := new(big.Int).SetString(sb.String(), 10)
	if !ok {
		return "", fmt.Errorf("invalid comp-3 value")
	}
	if sign == 0x0D && v.Sign() != 0 {
		v.Neg(v)
	}
	return formatScaled(v, scale), nil
}

// encodeField encodes value into buf according to the node kind.
func encodeField(n *Node, buf []byte, value string) error {
	switch n.Kind {
	case KindText:
		return encodeText(buf, value)
	case KindDisplay:
		return encodeDisplay(buf, value, n.Digits, n.Scale)
	case KindComp3:
		return encodeComp3(buf, value, n.Digits, n.Scale)
	}
	return fmt.Errorf("unknown field kind")
}

// decodeField decodes buf into a decimal or text string.
func decodeField(n *Node, buf []byte) (string, error) {
	switch n.Kind {
	case KindText:
		return decodeText(buf)
	case KindDisplay:
		return decodeDisplay(buf, n.Digits, n.Scale)
	case KindComp3:
		return decodeComp3(buf, n.Digits, n.Scale)
	}
	return "", fmt.Errorf("unknown field kind")
}
