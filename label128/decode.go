package label128

import "fmt"

// DecodeError locates a structural or checksum failure in a width
// sequence. Module is the index into the width slice where the problem
// was detected (-1 when not applicable).
type DecodeError struct {
	Module  int
	Message string
}

func (e *DecodeError) Error() string {
	if e.Module >= 0 {
		return fmt.Sprintf("module %d: %s", e.Module, e.Message)
	}
	return e.Message
}

// lookupPattern finds the code value for a 6-width symbol pattern.
func lookupPattern(widths []int) (int, bool) {
	var key [6]int
	copy(key[:], widths)
	for v := 0; v <= 105; v++ {
		s := widthPatterns[v]
		match := true
		for i := 0; i < 6; i++ {
			if int(s[i]-'0') != key[i] {
				match = false
				break
			}
		}
		if match {
			return v, true
		}
	}
	return -1, false
}

var stopPattern = []int{2, 3, 3, 1, 1, 1, 2}

// DecodeResult carries the decoded text plus audit information.
type DecodeResult struct {
	Text  string
	Codes []int // full sequence: start, data/switch, checksum, stop
	Extra int   // codewords beyond the canonical minimal encoding
}

// Decode recovers the text from a module width sequence (bars and spaces
// alternating, starting with a bar, including the stop symbol's final
// bar, excluding quiet zones).
func Decode(widths []int) (*DecodeResult, error) {
	if len(widths) < 6*3+7 {
		return nil, &DecodeError{Module: -1, Message: fmt.Sprintf("width sequence too short: %d entries, need at least %d", len(widths), 6*3+7)}
	}
	rem := len(widths) - 7
	if rem%6 != 0 {
		return nil, &DecodeError{Module: -1, Message: fmt.Sprintf("length %d is not 7 (stop) plus a multiple of 6", len(widths))}
	}
	for i, w := range widths {
		if w <= 0 {
			return nil, &DecodeError{Module: i, Message: fmt.Sprintf("width %d is not a positive integer", w)}
		}
	}
	// Verify stop symbol.
	stopAt := len(widths) - 7
	for i, w := range stopPattern {
		if widths[stopAt+i] != w {
			return nil, &DecodeError{Module: stopAt + i, Message: fmt.Sprintf("stop symbol mismatch: got width %d, want %d", widths[stopAt+i], w)}
		}
	}
	nsym := rem / 6
	codes := make([]int, 0, nsym+1)
	for s := 0; s < nsym; s++ {
		v, ok := lookupPattern(widths[s*6 : s*6+6])
		if !ok {
			return nil, &DecodeError{Module: s * 6, Message: fmt.Sprintf("symbol %d does not match any Code 128 pattern", s)}
		}
		codes = append(codes, v)
	}
	codes = append(codes, Stop)
	// Start code.
	if codes[0] != StartB && codes[0] != StartC {
		return nil, &DecodeError{Module: 0, Message: fmt.Sprintf("invalid start code %d: only B (%d) and C (%d) are supported", codes[0], StartB, StartC)}
	}
	// Checksum is the last code before stop.
	if len(codes) < 3 {
		return nil, &DecodeError{Module: -1, Message: "no checksum symbol present"}
	}
	check := codes[len(codes)-2]
	body := codes[:len(codes)-2]
	if want := checksum(body); want != check {
		return nil, &DecodeError{Module: (len(codes) - 2) * 6, Message: fmt.Sprintf("checksum mismatch: symbol value %d, computed %d", check, want)}
	}
	text, err := interpret(body)
	if err != nil {
		return nil, err
	}
	minimal := optimalCodes(text)
	extra := len(body) - len(minimal)
	return &DecodeResult{Text: text, Codes: codes, Extra: extra}, nil
}

// interpret walks the data/switch codes (start included, checksum and
// stop excluded) and rebuilds the text. A checksum-position value that
// equals a switch code never reaches this function.
func interpret(body []int) (string, error) {
	set := setB
	if body[0] == StartC {
		set = setC
	}
	out := make([]byte, 0, len(body))
	for i := 1; i < len(body); i++ {
		c := body[i]
		switch set {
		case setB:
			switch {
			case c == CodeC:
				set = setC
			case c >= 0 && c <= 94:
				out = append(out, byte(c+32))
			default:
				return "", &DecodeError{Module: i * 6, Message: fmt.Sprintf("code %d is not valid in code set B", c)}
			}
		case setC:
			switch {
			case c == CodeB:
				set = setB
			case c >= 0 && c <= 99:
				out = append(out, byte('0'+c/10), byte('0'+c%10))
			default:
				return "", &DecodeError{Module: i * 6, Message: fmt.Sprintf("code %d is not valid in code set C", c)}
			}
		}
	}
	if len(out) == 0 {
		return "", &DecodeError{Module: -1, Message: "symbol sequence encodes empty text"}
	}
	return string(out), nil
}
