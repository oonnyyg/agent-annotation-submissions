// hpacktool is an offline HPACK (RFC 7541 subset) replay tool. It reads
// JSONL commands on stdin (or from -in file) and prints one JSON result
// per line describing encode/decode results and dynamic table changes.
//
// Input line forms:
//
//	{"headers":[{"name":"...","value":"...","sensitive":true}], "table_size":4096}
//	    Encode the header list, then decode the produced block (round trip).
//	    "table_size" optionally changes the negotiated capacity first.
//	{"hex":"8284..."}
//	    Decode a raw compressed block with the decoder only.
//	{"table_size":1024}
//	    Change negotiated capacity; the encoder emits the update at the
//	    start of the next block.
//	{"reset":"encoder"|"decoder"|"both"}
//	    Clear a failed direction so it can be used again.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/hpack-table-workbench/hpack"
)

type command struct {
	Headers   []hpack.HeaderField `json:"headers"`
	Hex       string              `json:"hex"`
	TableSize *int                `json:"table_size"`
	Reset     string              `json:"reset"`
}

type fieldOut struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

type result struct {
	Line    int                 `json:"line"`
	Action  string              `json:"action"`
	Block   string              `json:"block_hex,omitempty"`
	Headers []fieldOut          `json:"headers,omitempty"`
	Table   []hpack.EvictRecord `json:"table_log,omitempty"`
	Error   string              `json:"error,omitempty"`
}

func fieldsOut(fs []hpack.HeaderField) []fieldOut {
	out := make([]fieldOut, 0, len(fs))
	for _, f := range fs {
		out = append(out, fieldOut{Name: f.Name, Value: f.Value, Sensitive: f.Sensitive})
	}
	return out
}

func main() {
	inPath := flag.String("in", "", "input JSONL file (default stdin)")
	maxSize := flag.Int("max-size", 4096, "initial negotiated dynamic table size")
	flag.Parse()

	var r io.Reader = os.Stdin
	if *inPath != "" {
		f, err := os.Open(*inPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		defer f.Close()
		r = f
	}

	enc := hpack.NewEncoder(*maxSize)
	dec := hpack.NewDecoder(*maxSize)
	out := json.NewEncoder(os.Stdout)

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		text := sc.Text()
		if text == "" {
			continue
		}
		var cmd command
		res := result{Line: line}
		if err := json.Unmarshal([]byte(text), &cmd); err != nil {
			res.Action = "error"
			res.Error = fmt.Sprintf("block %d: invalid JSON: %v", line, err)
			out.Encode(res)
			continue
		}
		switch {
		case cmd.Reset != "":
			res.Action = "reset"
			switch cmd.Reset {
			case "encoder":
				enc.Reset()
			case "decoder":
				dec.Reset()
			case "both":
				enc.Reset()
				dec.Reset()
			default:
				res.Error = fmt.Sprintf("block %d: unknown reset target %q", line, cmd.Reset)
			}
		case cmd.Hex != "":
			res.Action = "decode"
			block, err := hex.DecodeString(cmd.Hex)
			if err != nil {
				res.Error = fmt.Sprintf("block %d: bad hex: %v", line, err)
				break
			}
			if cmd.TableSize != nil {
				dec.SetMaxSize(*cmd.TableSize)
			}
			fields, log, err := dec.Decode(block)
			if err != nil {
				res.Error = fmt.Sprintf("block %d: %v", line, err)
				break
			}
			res.Headers = fieldsOut(fields)
			res.Table = log
		case cmd.Headers != nil:
			res.Action = "roundtrip"
			if cmd.TableSize != nil {
				enc.SetMaxSize(*cmd.TableSize)
				dec.SetMaxSize(*cmd.TableSize)
			}
			block, elog, err := enc.Encode(cmd.Headers)
			if err != nil {
				res.Error = fmt.Sprintf("block %d: encode: %v", line, err)
				break
			}
			res.Block = hex.EncodeToString(block)
			res.Table = elog
			fields, dlog, err := dec.Decode(block)
			if err != nil {
				res.Error = fmt.Sprintf("block %d: decode: %v", line, err)
				break
			}
			res.Headers = fieldsOut(fields)
			res.Table = append(res.Table, dlog...)
		default:
			if cmd.TableSize != nil {
				res.Action = "resize"
				enc.SetMaxSize(*cmd.TableSize)
				dec.SetMaxSize(*cmd.TableSize)
			} else {
				res.Action = "error"
				res.Error = fmt.Sprintf("block %d: empty command", line)
			}
		}
		out.Encode(res)
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
