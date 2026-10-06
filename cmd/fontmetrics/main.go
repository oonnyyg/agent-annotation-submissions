package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/sfnt-text-metrics/internal/metrics"
	"example.com/sfnt-text-metrics/internal/sfnt"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fontmetrics:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("fontmetrics", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fmt.Errorf("usage: fontmetrics <font.ttf> <request.json|->")
	}

	fontData, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("reading font: %w", err)
	}
	reqData, err := readRequest(fs.Arg(1))
	if err != nil {
		return err
	}

	var req metrics.Request
	if err := json.Unmarshal(reqData, &req); err != nil {
		return fmt.Errorf("parsing request: %w", err)
}
	if req.Size <= 0 {
		return fmt.Errorf("request: size must be a positive number")
	}

	font, err := sfnt.Parse(fontData)
	if err != nil {
		return err
	}
	resp, err := metrics.Measure(font, &req)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}

func readRequest(path string) ([]byte, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading request: %w", err)
	}
	return data, nil
}
