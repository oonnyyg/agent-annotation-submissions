// dnswire is an offline DNS wire-format workbench. It decodes raw binary
// messages to JSON and encodes supported JSON documents back to wire
// format. It never performs network I/O.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"example.com/dns-wire-workbench/dnswire"
)

func usage() {
	fmt.Fprintf(os.Stderr, `usage:
  dnswire decode <input.bin> [output.json]
  dnswire encode <input.json> [output.bin]

Input "-" reads stdin; output "-" or omitted writes stdout.
Existing output files are never overwritten on failure.
`)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 3 {
		usage()
	}
	mode, inPath := os.Args[1], os.Args[2]
	outPath := "-"
	if len(os.Args) > 3 {
		outPath = os.Args[3]
	}
	var err error
	switch mode {
	case "decode":
		err = runDecode(inPath, outPath)
	case "encode":
		err = runEncode(inPath, outPath)
	default:
		usage()
	}
	if err != nil {
		var we *dnswire.WireError
		if errors.As(err, &we) {
			b, _ := json.Marshal(we)
			fmt.Fprintf(os.Stderr, `{"error":%s}`+"\n", b)
		} else {
			fmt.Fprintf(os.Stderr, `{"error":{"offset":-1,"reason":%q}}`+"\n", err.Error())
		}
		os.Exit(1)
	}
}

func readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// writeOutput writes atomically: failures never clobber an existing file.
func writeOutput(path string, data []byte) error {
	if path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dnswire-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func runDecode(inPath, outPath string) error {
	raw, err := readInput(inPath)
	if err != nil {
		return err
	}
	msg, err := dnswire.DecodeMessage(raw)
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return err
	}
	return writeOutput(outPath, append(out, '\n'))
}

func runEncode(inPath, outPath string) error {
	raw, err := readInput(inPath)
	if err != nil {
		return err
	}
	var doc dnswire.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	msg, err := doc.ToMessage()
	if err != nil {
		return err
	}
	wire, err := dnswire.EncodeMessage(msg)
	if err != nil {
		return err
	}
	return writeOutput(outPath, wire)
}
