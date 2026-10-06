package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	jsonOut := flag.Bool("json", false, "emit the audit report as JSON")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [-json] <file>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Offline HTTP/1.1 request-boundary auditor. Analyzes a local\n")
		fmt.Fprintf(os.Stderr, "binary file as a strict HTTP/1.1 request stream. Does not\n")
		fmt.Fprintf(os.Stderr, "listen on ports or forward traffic.\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	path := flag.Arg(0)
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	result := Audit(path, data)
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(2)
		}
	} else {
		printText(result)
	}
	if result.Error != nil {
		os.Exit(1)
	}
}

func printText(r *AuditResult) {
	fmt.Printf("file: %s (%d bytes)\n", r.File, r.TotalBytes)
	for _, req := range r.Requests {
		fmt.Printf("request #%d: %s %s\n", req.Index, req.Method, req.Target)
		fmt.Printf("  offsets: [%d, %d)\n", req.StartOffset, req.EndOffset)
		fmt.Printf("  body: %d bytes, sha256=%s\n", req.BodyLength, req.BodySHA256)
	}
	fmt.Printf("complete requests: %d, trailing bytes: %d\n", r.RequestCount, r.TrailingBytes)
	if r.Error != nil {
		fmt.Printf("error at offset %d: %s\n", r.Error.Offset, r.Error.Message)
	}
}
