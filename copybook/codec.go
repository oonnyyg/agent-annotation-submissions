package copybook

import (
	"fmt"
	"math/big"
	"strings"
)

// RecordError reports a data error at a specific record and field path.
type RecordError struct {
	Record int    // 1-based record number
	Path   string // field path, e.g. ORDER.LINES[2].PRICE
	Msg    string
}

func (e *RecordError) Error() string {
	return fmt.Sprintf("record %d, field %s: %s", e.Record, e.Path, e.Msg)
}

// encodeText converts s to a fixed-length ASCII field, right-padded with
// spaces. Non-ASCII or overlong input is rejected.
func encodeText(s string, length int) ([]byte, error) {
	if len(s) > length {
		return nil, fmt.Errorf("text of %d bytes exceeds field length %d", len(s), length)
	}
	buf := make([]byte, length)
	for i := range buf {
		buf[i] = ' '
	}
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7F {
			return nil, fmt.Errorf("non-ASCII character 0x%02X at byte %d", s[i], i)
		}
		buf[i] = s[i]
	}
	return buf, nil
}

func decodeText(b []byte) (string, error) {
	for i, c := range b {
		if c > 0x7F {
			return "", fmt.Errorf("non-ASCII byte 0x%02X at offset %d", c, i)
		}
	}
	return string(b), nil
}

// parseDecimal parses a JSON decimal string into sign and unscaled integer
// for the given scale. It rejects values with more fractional digits than
// scale allows, or more integer digits than intDigits.
func parseDecimal(s string, intDigits, scale int, signed bool) (neg bool, unscaled *big.Int, err error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return false, nil, fmt.Errorf("empty numeric value")
	}
	neg = false
	if t[0] == '-' || t[0] == '+' {
		neg = t[0] == '-'
		t = t[1:]
	}
	if neg && !signed {
		return false, nil, fmt.Errorf("negative value in unsigned field")
	}
	intPart, fracPart := t, ""
	if i := strings.IndexByte(t, '.'); i >= 0 {
		intPart, fracPart = t[:i], t[i+1:]
		if strings.IndexByte(fracPart, '.') >= 0 {
			return false, nil, fmt.Errorf("malformed decimal %q", s)
		}
	}
	if intPart == "" {
		intPart = "0"
	}
	if len(fracPart) > scale {
		return false, nil, fmt.Errorf("value %q has more than %d fractional digits", s, scale)
	}
	for _, c := range intPart + fracPart {
		if c < '0' || c > '9' {
			return false, nil, fmt.Errorf("invalid digit %q in %q", string(c), s)
		}
	}
	intTrim := strings.TrimLeft(intPart, "0")
	if len(intTrim) > intDigits {
		return false, nil, fmt.Errorf("value %q exceeds %d integer digits", s, intDigits)
	}
	fracPart += strings.Repeat("0", scale-len(fracPart))
	unscaled, ok := new(big.Int).SetString(intTrim+fracPart, 10)
	if !ok {
		unscaled = new(big.Int)
	}
	return neg, unscaled, nil
}

// formatDecimal renders an unscaled integer with the given scale as a
// canonical decimal string.
func formatDecimal(neg bool, unscaled *big.Int, scale int) string {
	digits := unscaled.String()
	if scale > 0 {
		for len(digits) <= scale {
			digits = "0" + digits
		}
		digits = digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
	}
	if neg && unscaled.Sign() != 0 {
		return "-" + digits
	}
	return digits
}

// encodeDisplay encodes a decimal string as unsigned zoned display digits.
func encodeDisplay(s string, n *Node) ([]byte, error) {
	total := n.Digits + n.Scale
	_, unscaled, err := parseDecimal(s, n.Digits, n.Scale, false)
	if err != nil {
		return nil, err
	}
	digits := unscaled.String()
	if len(digits) > total {
		return nil, fmt.Errorf("value %q exceeds %d total digits", s, total)
	}
	buf := make([]byte, total)
	for i := range buf {
		buf[i] = '0'
	}
	copy(buf[total-len(digits):], digits)
	return buf, nil
}

func decodeDisplay(b []byte, n *Node) (string, error) {
	for i, c := range b {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("invalid display digit 0x%02X at offset %d", c, i)
		}
	}
	unscaled, _ := new(big.Int).SetString(string(b), 10)
	return formatDecimal(false, unscaled, n.Scale), nil
}

// encodeComp3 encodes a signed decimal string as packed decimal. Zero is
// always written with the C (positive) sign nibble.
func encodeComp3(s string, n *Node) ([]byte, error) {
	total := n.Digits + n.Scale
	neg, unscaled, err := parseDecimal(s, n.Digits, n.Scale, true)
	if err != nil {
		return nil, err
	}
	digits := unscaled.String()
	if len(digits) > total {
		return nil, fmt.Errorf("value %q exceeds %d total digits", s, total)
	}
	digits = strings.Repeat("0", total-len(digits)) + digits
	sign := byte(0x0C)
	if neg && unscaled.Sign() != 0 {
		sign = 0x0D
	}
	nibbles := make([]byte, 0, total+1)
	if total%2 == 0 {
		nibbles = append(nibbles, 0)
	}
	for i := 0; i < len(digits); i++ {
		nibbles = append(nibbles, digits[i]-'0')
	}
	nibbles = append(nibbles, sign)
	buf := make([]byte, len(nibbles)/2)
	for i := range buf {
		buf[i] = nibbles[2*i]<<4 | nibbles[2*i+1]
	}
	return buf, nil
}

func decodeComp3(b []byte, n *Node) (string, error) {
	total := n.Digits + n.Scale
	sign := b[len(b)-1] & 0x0F
	var neg bool
	switch sign {
	case 0x0C:
		neg = false
	case 0x0D:
		neg = true
	default:
		return "", fmt.Errorf("invalid COMP-3 sign nibble 0x%X (only C/D accepted)", sign)
	}
	digits := make([]byte, 0, total)
	nibCount := 2*len(b) - 1
	for i := 0; i < nibCount; i++ {
		var nib byte
		if i%2 == 0 {
			nib = b[i/2] >> 4
		} else {
			nib = b[i/2] & 0x0F
		}
		if nib > 9 {
			return "", fmt.Errorf("invalid COMP-3 digit nibble 0x%X at nibble %d", nib, i)
		}
		digits = append(digits, '0'+nib)
	}
	if total%2 == 0 {
		if digits[0] != '0' {
			return "", fmt.Errorf("leading pad nibble of even-digit COMP-3 must be zero")
		}
		digits = digits[1:]
	}
	unscaled, _ := new(big.Int).SetString(string(digits), 10)
	return formatDecimal(neg, unscaled, n.Scale), nil
}
