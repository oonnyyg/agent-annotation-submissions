package code128

import "fmt"

// DecodeError locates a structural or checksum problem in a width sequence.
type DecodeError struct {
	// Offset is the module index in the width sequence where the problem
	// was detected (or the codeword index for checksum/logic errors).
	Offset int
	// Kind is "module", "codeword", "checksum", or "logic".
	Kind    string
	Message string
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("%s error at offset %d: %s", e.Kind, e.Offset, e.Message)
}

// DecodeResult is the outcome of decoding a width sequence.
type DecodeResult struct {
	Text      string
	Codewords []int // full sequence: start, data/latch, checksum, stop
	// ExtraCodewords is how many data/latch codewords this symbol uses
	// beyond the canonical shortest encoding of the same text.
	ExtraCodewords int
}

// Decode recovers codewords from module widths, verifies structure and
// checksum, and reconstructs the text using code sets B and C only.
func Decode(widths []int) (*DecodeResult, error) {
	if len(widths) == 0 {
		return nil, &DecodeError{Kind: "module", Message: "empty width sequence"}
	}
	for i, w := range widths {
		if w < 1 || w > 4 {
			return nil, &DecodeError{Offset: i, Kind: "module", Message: fmt.Sprintf("width %d out of range 1-4", w)}
		}
	}
	if len(widths) < 6+6+7 || (len(widths)-7)%6 != 0 {
		return nil, &DecodeError{Kind: "module", Message: fmt.Sprintf("length %d does not match 6n+7 module structure", len(widths))}
	}
	// Verify stop pattern (last 7 modules).
	stopStart := len(widths) - 7
	for j := 0; j < 7; j++ {
		if widths[stopStart+j] != int(stopPattern[j]-'0') {
			return nil, &DecodeError{Offset: stopStart + j, Kind: "module", Message: "corrupted stop pattern (expected 2331112)"}
		}
	}
	// Map each 6-module group to a codeword value.
	n := stopStart / 6
	codewords := make([]int, 0, n+1)
	for i := 0; i < n; i++ {
		p := make([]byte, 6)
		for j := 0; j < 6; j++ {
			p[j] = byte('0' + widths[i*6+j])
		}
		v, ok := patternToValue[string(p)]
		if !ok {
			return nil, &DecodeError{Offset: i * 6, Kind: "module", Message: fmt.Sprintf("module group %d (%s) matches no codeword", i, p)}
		}
		codewords = append(codewords, v)
	}
	codewords = append(codewords, ValueStop)

	start := codewords[0]
	if start != ValueStartB && start != ValueStartC {
		return nil, &DecodeError{Offset: 0, Kind: "codeword", Message: fmt.Sprintf("invalid start codeword %d (only Start B=104, Start C=105 supported)", start)}
	}
	if len(codewords) < 3 { // start + checksum + stop
		return nil, &DecodeError{Kind: "codeword", Message: "symbol too short for checksum"}
	}
	checkIdx := len(codewords) - 2
	want := Checksum(codewords[:checkIdx])
	if codewords[checkIdx] != want {
		return nil, &DecodeError{Offset: checkIdx, Kind: "checksum", Message: fmt.Sprintf("checksum mismatch: got %d, want %d", codewords[checkIdx], want)}
	}

	// Interpret data codewords (checksum excluded even if it equals a latch).
	set := setB
	if start == ValueStartC {
		set = setC
	}
	text := make([]byte, 0, len(codewords))
	for i := 1; i < checkIdx; i++ {
		v := codewords[i]
		switch set {
		case setB:
			switch {
			case v <= 94:
				text = append(text, byte(v+32))
			case v == ValueCodeC:
				set = setC
			case v == ValueCodeB:
				return nil, &DecodeError{Offset: i, Kind: "logic", Message: "redundant Code B latch while already in set B"}
			default:
				return nil, &DecodeError{Offset: i, Kind: "logic", Message: fmt.Sprintf("codeword %d not supported in set B (no set A/FNC/shift)", v)}
			}
		case setC:
			switch {
			case v <= 99:
				text = append(text, byte('0'+v/10), byte('0'+v%10))
			case v == ValueCodeB:
				set = setB
			default:
				return nil, &DecodeError{Offset: i, Kind: "logic", Message: fmt.Sprintf("codeword %d not supported in set C", v)}
			}
		}
	}
	if len(text) == 0 {
		return nil, &DecodeError{Kind: "logic", Message: "symbol encodes empty text"}
	}

	optimal, err := Encode(string(text))
	if err != nil {
		return nil, err
	}
	// Both sequences include start+checksum+stop; compare data/latch counts.
	extra := (checkIdx - 1) - (len(optimal) - 3)
	return &DecodeResult{Text: string(text), Codewords: codewords, ExtraCodewords: extra}, nil
}
