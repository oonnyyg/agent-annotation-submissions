package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"example.com/copybook-record-codec/copybook"
)

const usage = `copycodec - fixed-length record <-> JSONL converter (copybook subset)

usage:
  copycodec layout <copybook> <layout.json>
  copycodec decode <copybook> <input.bin> <output.jsonl>
  copycodec encode <copybook> <input.jsonl> <output.bin>

All output files are written atomically: on any conversion error the
output file is left untouched and input files are never modified.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "layout":
		err = runLayout(os.Args[2:])
	case "decode":
		err = runConvert(os.Args[2:], true)
	case "encode":
		err = runConvert(os.Args[2:], false)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "copycodec: "+err.Error())
		os.Exit(1)
	}
}

func loadLayout(copybookPath string) (*copybook.Layout, error) {
	src, err := os.ReadFile(copybookPath)
	if err != nil {
		return nil, err
	}
	fields, err := copybook.Parse(string(src))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", copybookPath, err)
	}
	return copybook.Compile(fields)
}

func runLayout(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("layout requires <copybook> and <layout.json>")
	}
	l, err := loadLayout(args[0])
	if err != nil {
		return err
	}
	j, err := l.LayoutJSON()
	if err != nil {
		return err
	}
	return writeAtomic(args[1], append(j, '\n'))
}

func runConvert(args []string, decode bool) error {
	if len(args) != 3 {
		return fmt.Errorf("requires <copybook> <input> <output>")
	}
	l, err := loadLayout(args[0])
	if err != nil {
		return err
	}
	in, err := os.Open(args[1])
	if err != nil {
		return err
	}
	defer in.Close()
	var buf bytes.Buffer
	if decode {
		err = copybook.BinToJSONL(l, in, &buf)
	} else {
		err = copybook.JSONLToBin(l, in, &buf)
	}
	if err != nil {
		return err
	}
	return writeAtomic(args[2], buf.Bytes())
}

// writeAtomic writes data to a temp file in the same directory and renames
// it, so a failed conversion never leaves a partial output behind.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".copycodec-*")
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
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
