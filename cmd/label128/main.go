// Command label128 reads JSONL requests from stdin and writes one JSON
// response per line. Each request is processed independently; a failed
// line never blocks later requests.
//
// Encode request:  {"op":"encode","text":"ABC1234"}
// Encode response: {"ok":true,"codes":[...],"widths":[...]}
//
// Decode request:  {"op":"decode","widths":[2,1,1,4,...]}
// Decode response: {"ok":true,"text":"...","codes":[...],"extra":0}
//
// Failures:        {"ok":false,"error":"...","position":N}
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"example.com/code128-label-audit/label128"
)

type request struct {
	Op     string `json:"op"`
	Text   string `json:"text"`
	Widths []int  `json:"widths"`
}

type response struct {
	OK       bool   `json:"ok"`
	Codes    []int  `json:"codes,omitempty"`
	Widths   []int  `json:"widths,omitempty"`
	Text     string `json:"text,omitempty"`
	Extra    *int   `json:"extra,omitempty"`
	Error    string `json:"error,omitempty"`
	Position int    `json:"position"`
}

func fail(msg string, pos int) response {
	return response{OK: false, Error: msg, Position: pos}
}

func handle(req request) response {
	switch req.Op {
	case "encode":
		codes, widths, err := label128.Encode(req.Text)
		if err != nil {
			pos := -1
			if ee, ok := err.(*label128.EncodeError); ok {
				pos = ee.Position
			}
			return fail(err.Error(), pos)
		}
		return response{OK: true, Codes: codes, Widths: widths, Position: -1}
	case "decode":
		res, err := label128.Decode(req.Widths)
		if err != nil {
			pos := -1
			if de, ok := err.(*label128.DecodeError); ok {
				pos = de.Module
			}
			return fail(err.Error(), pos)
		}
		return response{OK: true, Text: res.Text, Codes: res.Codes, Extra: &res.Extra, Position: -1}
	default:
		return fail(fmt.Sprintf("unknown op %q: want \"encode\" or \"decode\"", req.Op), -1)
	}
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()
	enc := json.NewEncoder(writer)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var resp response
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			resp = fail("invalid JSON: "+err.Error(), -1)
		} else {
			resp = handle(req)
		}
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintf(os.Stderr, "write response: %v\n", err)
			return
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read input: %v\n", err)
		os.Exit(1)
	}
}
