package label128

import (
	"errors"
	"fmt"
)

// EncodeError describes why a text cannot be encoded.
type EncodeError struct {
	Position int // byte offset of the offending character, -1 if none
	Message  string
}

func (e *EncodeError) Error() string {
	if e.Position >= 0 {
		return fmt.Sprintf("position %d: %s", e.Position, e.Message)
	}
	return e.Message
}

// ValidateText reports whether text is non-empty printable ASCII
// (space through tilde).
func ValidateText(text string) error {
	if text == "" {
		return &EncodeError{Position: -1, Message: "text is empty"}
	}
	for i := 0; i < len(text); i++ {
		if text[i] < 0x20 || text[i] > 0x7e {
			return &EncodeError{Position: i, Message: fmt.Sprintf("character %q is outside printable ASCII (0x20-0x7e)", text[i])}
		}
	}
	return nil
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// codeset identifiers for the DP state.
const (
	setB = 0
	setC = 1
)

type dpState struct {
	cost int
	seq  []int
}

// better reports whether candidate (cost, seq) beats the incumbent.
// Lower cost wins; ties break on lexicographically smaller full sequence.
func better(cost int, seq []int, cur dpState, ok bool) bool {
	if !ok || cost < cur.cost {
		return true
	}
	if cost > cur.cost {
		return false
	}
	for i := 0; i < len(seq) && i < len(cur.seq); i++ {
		if seq[i] != cur.seq[i] {
			return seq[i] < cur.seq[i]
		}
	}
	return len(seq) < len(cur.seq)
}

// optimalCodes returns the minimal code sequence (start, data and switch
// codes; no checksum or stop) encoding text with code sets B and C only.
func optimalCodes(text string) []int {
	n := len(text)
	type key struct {
		pos int
		set int
	}
	best := map[key]dpState{}
	starts := []struct {
		set  int
		code int
	}{{setB, StartB}}
	if n >= 2 && isDigit(text[0]) && isDigit(text[1]) {
		starts = append(starts, struct {
			set  int
			code int
		}{setC, StartC})
	}
	// Dijkstra over (cost, seq) ordering; states are few, so a simple
	// repeated relaxation works.
	for _, s := range starts {
		best[key{0, s.set}] = dpState{cost: 1, seq: []int{s.code}}
	}
	changed := true
	for changed {
		changed = false
		for k, cur := range best {
			try := func(nk key, add int) {
				seq := make([]int, len(cur.seq)+1)
				copy(seq, cur.seq)
				seq[len(cur.seq)] = add
				inc, ok := best[nk]
				if better(cur.cost+1, seq, inc, ok) {
					best[nk] = dpState{cost: cur.cost + 1, seq: seq}
					changed = true
				}
			}
			if k.pos >= n {
				continue
			}
			switch k.set {
			case setB:
				try(key{k.pos + 1, setB}, int(text[k.pos])-32)
				if k.pos+1 < n && isDigit(text[k.pos]) && isDigit(text[k.pos+1]) {
					try(key{k.pos, setC}, CodeC)
				}
			case setC:
				if k.pos+1 < n && isDigit(text[k.pos]) && isDigit(text[k.pos+1]) {
					v := int(text[k.pos]-'0')*10 + int(text[k.pos+1]-'0')
					try(key{k.pos + 2, setC}, v)
				}
				try(key{k.pos, setB}, CodeB)
			}
		}
	}
	endB, okB := best[key{n, setB}]
	endC, okC := best[key{n, setC}]
	switch {
	case okB && (!okC || better(endB.cost, endB.seq, endC, true)):
		return endB.seq
	case okC:
		return endC.seq
	}
	return nil
}

// Encode produces the full code sequence (start, data/switch codes,
// checksum, stop) and the module widths for text.
func Encode(text string) (codes []int, widths []int, err error) {
	if err := ValidateText(text); err != nil {
		return nil, nil, err
	}
	body := optimalCodes(text)
	if body == nil {
		return nil, nil, errors.New("no encoding found")
	}
	codes = append(append([]int{}, body...), checksum(body), Stop)
	for _, c := range codes {
		widths = append(widths, patternOf(c)...)
	}
	return codes, widths, nil
}
