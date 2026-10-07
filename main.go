package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type fileEntry struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type batchIndex struct {
	Count int         `json:"count"`
	Files []fileEntry `json:"files"`
}

// splitToDir streams records into a staging directory and only publishes
// them to outDir once the whole document validates. outDir must not exist.
func splitToDir(r io.Reader, outDir string, path []PathSegment, maxSize int64) (int, error) {
	if _, err := os.Stat(outDir); err == nil {
		return 0, fmt.Errorf("output directory %q already exists", outDir)
	} else if !os.IsNotExist(err) {
		return 0, err
	}
	parent := filepath.Dir(outDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return 0, err
	}
	staging, err := os.MkdirTemp(parent, ".xmlsplit-staging-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(staging)

	index := batchIndex{}
	onRecord := func(data []byte) error {
		name := fmt.Sprintf("record-%06d.xml", index.Count+1)
		if err := os.WriteFile(filepath.Join(staging, name), data, 0o644); err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		index.Files = append(index.Files, fileEntry{
			Name:   name,
			Size:   int64(len(data)),
			SHA256: hex.EncodeToString(sum[:]),
		})
		index.Count++
		return nil
	}

	count, err := Run(r, path, maxSize, onRecord)
	if err != nil {
		return count, err
	}

	indexData, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return count, err
	}
	if err := os.WriteFile(filepath.Join(staging, "index.json"), append(indexData, '\n'), 0o644); err != nil {
		return count, err
	}
	if err := os.Rename(staging, outDir); err != nil {
		return count, fmt.Errorf("publishing batch: %w", err)
	}
	return count, nil
}

func main() {
	var (
		pathFlag = flag.String("path", "", "record path, e.g. {urn:a}root/{urn:b}record (required)")
		inFlag   = flag.String("in", "", "input XML file (default: stdin)")
		outFlag  = flag.String("out", "", "output batch directory, must not exist (required)")
		maxFlag  = flag.Int64("max-record-size", 16<<20, "maximum bytes per record (0 = unlimited)")
	)
	flag.Parse()
	if *pathFlag == "" || *outFlag == "" {
		fmt.Fprintln(os.Stderr, "error: -path and -out are required")
		flag.Usage()
		os.Exit(2)
	}
	path, err := ParsePath(*pathFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	var in io.Reader = os.Stdin
	if *inFlag != "" {
		f, err := os.Open(*inFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		defer f.Close()
		in = f
	}

	count, err := splitToDir(in, *outFlag, path, *maxFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("published %d record(s) to %s\n", count, *outFlag)
}
