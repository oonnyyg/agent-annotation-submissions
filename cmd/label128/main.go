// Command label128 reads JSONL requests from stdin and writes one JSON
// response per line. Each request is handled independently; a failed line
// never blocks later requests.
//
// Encode request:  {"op":"encode","text":"ABC1234"}
// Decode request:  {"op":"decode","widths":[2,1,1,2,1,4,...]}
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"example.com/code128-label-audit/internal/code128"
)

type request struct {
	Op     string `json:"op"`
	Text   string `json:"text"`
	Widths []int  `json:"widths"`
}

type response struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Offset    *int   `json:"offset,omitempty"`
	Text      string `json:"text,omitempty"`
	Codewords []int  `json:"codewords,omitempty"`
	Widths    []int  `json:"widths,omitempty"`
	Extra     *int   `json:"extra_codewords,omitempty"`
}

func intPtr(v int) *int { return &v }

func handle(req request) response {
	switch req.Op {
	case "encode":
		seq, err := code128.Encode(req.Text)
		if err != nil {
			r := response{Error: err.Error()}
			var ee *code128.EncodeError
			if asEncodeError(err, &ee) {
				r.Kind = "character"
				r.Offset = intPtr(ee.Position)
			}
			return r
		}
		w, err := code128.Widths(seq)
		if err != nil {
			return response{Error: err.Error()}
		}
		return response{OK: true, Codewords: seq, Widths: w}
	case "decode":
		res, err := code128.Decode(req.Widths)
		if err != nil {
			r := response{Error: err.Error()}
			var de *code128.DecodeError
			if asDecodeError(err, &de) {
				r.Kind = de.Kind
				r.Offset = intPtr(de.Offset)
			}
			return r
		}
		return response{OK: true, Text: res.Text, Codewords: res.Codewords, Extra: intPtr(res.ExtraCodewords)}
	default:
		return response{Error: fmt.Sprintf("unknown op %q (want \"encode\" or \"decode\")", req.Op)}
	}
}

func asEncodeError(err error, target **code128.EncodeError) bool {
	if e, ok := err.(*code128.EncodeError); ok {
		*target = e
		return true
	}
	return false
}

func asDecodeError(err error, target **code128.DecodeError) bool {
	if e, ok := err.(*code128.DecodeError); ok {
		*target = e
		return true
	}
	return false
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	wr := bufio.NewWriter(os.Stdout)
	defer wr.Flush()
	enc := json.NewEncoder(wr)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(raw, &req); err != nil {
			enc.Encode(response{Error: fmt.Sprintf("line %d: invalid JSON: %v", line, err)})
			continue
		}
		enc.Encode(handle(req))
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read error:", err)
		os.Exit(1)
	}
}
