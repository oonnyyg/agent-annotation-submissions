package code128

import (
	"errors"
	"fmt"
)

// EncodeError describes why a text cannot be encoded.
type EncodeError struct {
	Position int
	Message  string
}

func (e *EncodeError) Error() string {
	return fmt.Sprintf("position %d: %s", e.Position, e.Message)
}

// ValidateText checks the supported character range (space..tilde).
func ValidateText(text string) error {
	if text == "" {
		return errors.New("text must not be empty")
	}
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c < 0x20 || c > 0x7e {
			return &EncodeError{Position: i, Message: fmt.Sprintf("character %q outside printable ASCII range 0x20-0x7e", c)}
		}
	}
	return nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

const (
	setB = 0
	setC = 1
)

// transition is one possible next codeword from a DP state.
type transition struct {
	value int // codeword value emitted
	next  int // next text position
	toSet int // code set after emitting
}

func transitions(text string, pos, set int) []transition {
	var out []transition
	if set == setB {
		out = append(out, transition{int(text[pos]) - 32, pos + 1, setB})
		if pos+2 <= len(text) && isDigit(text[pos]) && isDigit(text[pos+1]) {
			out = append(out, transition{ValueCodeC, pos, setC})
		}
	} else {
		if pos+2 <= len(text) && isDigit(text[pos]) && isDigit(text[pos+1]) {
			out = append(out, transition{int(text[pos]-'0')*10 + int(text[pos+1]-'0'), pos + 2, setC})
		}
		out = append(out, transition{ValueCodeB, pos, setB})
	}
	return out
}

// Encode returns the optimal codeword sequence (start, data/latch, checksum,
// stop) for text, minimizing the number of start+data+latch codewords and
// breaking ties by the lexicographically smallest full codeword sequence.
func Encode(text string) ([]int, error) {
	if err := ValidateText(text); err != nil {
		return nil, err
	}
	n := len(text)
	const inf = int(^uint(0) >> 1)
	// best[pos][set] = minimal codewords to encode text[pos:] while in set.
	best := make([][2]int, n+1)
	best[n] = [2]int{0, 0}
	for pos := n - 1; pos >= 0; pos-- {
		best[pos] = [2]int{inf, inf}
		for set := 0; set < 2; set++ {
			for _, tr := range transitions(text, pos, set) {
				if c := best[tr.next][tr.toSet]; c != inf && 1+c < best[pos][set] {
					best[pos][set] = 1 + c
				}
			}
		}
	}
	// Choose the start set: minimal count, tie -> smaller start value (B < C).
	startSet := setB
	if best[0][setC] < best[0][setB] {
		startSet = setC
	}
	start := ValueStartB
	if startSet == setC {
		start = ValueStartC
	}
	// Greedy reconstruction: at each state pick the smallest codeword value
	// among transitions that attain the optimum.
	seq := []int{start}
	pos, set := 0, startSet
	for pos < n {
		chosen := transition{value: inf}
		for _, tr := range transitions(text, pos, set) {
			if best[tr.next][tr.toSet] != inf && 1+best[tr.next][tr.toSet] == best[pos][set] && tr.value < chosen.value {
				chosen = tr
			}
		}
		seq = append(seq, chosen.value)
		pos, set = chosen.next, chosen.toSet
	}
	seq = append(seq, Checksum(seq))
	seq = append(seq, ValueStop)
	return seq, nil
}

// Checksum computes the modulo-103 check value for a sequence that starts
// with the start codeword and contains only data/latch codewords after it.
func Checksum(seq []int) int {
	sum := seq[0]
	for i := 1; i < len(seq); i++ {
		sum += i * seq[i]
	}
	return sum % 103
}

// Widths converts a full codeword sequence (including stop) into the
// alternating bar/space module widths, excluding quiet zones.
func Widths(seq []int) ([]int, error) {
	var out []int
	for i, v := range seq {
		var p string
		if v == ValueStop {
			if i != len(seq)-1 {
				return nil, fmt.Errorf("stop codeword at index %d is not last", i)
			}
			p = stopPattern
		} else if v >= 0 && v < len(patterns) {
			p = patterns[v]
		} else {
			return nil, fmt.Errorf("invalid codeword value %d at index %d", v, i)
		}
		for j := 0; j < len(p); j++ {
			out = append(out, int(p[j]-'0'))
		}
	}
	return out, nil
}
