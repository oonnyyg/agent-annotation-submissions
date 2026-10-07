// Command cbc converts fixed-length binary records to and from JSONL using
// a strict subset of COBOL copybook definitions. Standard library only.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"example.com/copybook-record-codec/copybook"
)

func usage() {
	fmt.Fprintf(os.Stderr, `usage:
  cbc layout  -copybook FILE
  cbc to-json -copybook FILE -in BIN -out JSONL
  cbc to-bin  -copybook FILE -in JSONL -out BIN
`)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	var cpyPath, inPath, outPath string
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) {
			usage()
		}
		switch args[i] {
		case "-copybook":
			cpyPath = args[i+1]
		case "-in":
			inPath = args[i+1]
		case "-out":
			outPath = args[i+1]
		default:
			usage()
		}
	}
	if cpyPath == "" {
		usage()
	}
	if err := run(cmd, cpyPath, inPath, outPath); err != nil {
		fmt.Fprintf(os.Stderr, "cbc: %v\n", err)
		os.Exit(1)
	}
}

func loadLayout(cpyPath string) (*copybook.Layout, error) {
	src, err := os.ReadFile(cpyPath)
	if err != nil {
		return nil, err
	}
	root, err := copybook.Parse(string(src))
	if err != nil {
		return nil, err
	}
	return copybook.Compile(root)
}

func run(cmd, cpyPath, inPath, outPath string) error {
	layout, err := loadLayout(cpyPath)
	if err != nil {
		return err
	}
	switch cmd {
	case "layout":
		b, err := layout.MarshalLayout()
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(append(b, '\n'))
		return err
	case "to-json":
		return toJSON(layout, inPath, outPath)
	case "to-bin":
		return toBin(layout, inPath, outPath)
	}
	usage()
	return nil
}

// writeAtomic writes data to path via a temp file in the same directory, so
// a failed conversion never publishes partial output.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".cbc-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func toJSON(layout *copybook.Layout, inPath, outPath string) error {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	rl := layout.RecordLength
	if len(data)%rl != 0 {
		return fmt.Errorf("input has %d trailing bytes beyond the last full record (record length %d)", len(data)%rl, rl)
	}
	var out bytes.Buffer
	for recNo, off := 1, 0; off < len(data); recNo, off = recNo+1, off+rl {
		v, err := layout.DecodeRecord(data[off : off+rl])
		if err != nil {
			return fmt.Errorf("record %d: %w", recNo, err)
		}
		line, err := copybook.MarshalJSONValue(v)
		if err != nil {
			return fmt.Errorf("record %d: %w", recNo, err)
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	return writeAtomic(outPath, out.Bytes())
}

func toBin(layout *copybook.Layout, inPath, outPath string) error {
	f, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer f.Close()
	var out bytes.Buffer
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	recNo := 0
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		recNo++
		var v any
		dec := json.NewDecoder(bytes.NewReader(line))
		if err := dec.Decode(&v); err != nil {
			return fmt.Errorf("record %d: invalid JSON: %w", recNo, err)
		}
		rec, err := layout.EncodeRecord(v)
		if err != nil {
			return fmt.Errorf("record %d: %w", recNo, err)
		}
		out.Write(rec)
	}
	if err := sc.Err(); err != nil && err != io.EOF {
		return err
	}
	return writeAtomic(outPath, out.Bytes())
}
