// Command xmlsplit extracts business records from an XML stream into
// standalone, individually parseable XML documents.
//
// Usage:
//
//	xmlsplit -path '{urn:orders}orders/{urn:orders}order' -in orders.xml -out batch-001 [-max 1048576]
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"example.com/xml-record-splitter/split"
)

func main() {
	var (
		pathFlag = flag.String("path", "", "record path from the document root, e.g. {urn:a}root/{urn:b}item")
		inFlag   = flag.String("in", "", "input XML file (default: stdin)")
		outFlag  = flag.String("out", "", "new batch output directory (must not exist)")
		maxFlag  = flag.Int64("max", 16<<20, "maximum bytes per record")
	)
	flag.Parse()

	if *pathFlag == "" || *outFlag == "" {
		fmt.Fprintln(os.Stderr, "error: -path and -out are required")
		flag.Usage()
		os.Exit(2)
	}
	path, err := split.ParsePath(*pathFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	var r io.Reader = os.Stdin
	if *inFlag != "" {
		f, err := os.Open(*inFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		defer f.Close()
		r = f
	}

	n, err := split.Run(r, path, *outFlag, *maxFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xmlsplit: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("published %d record(s) to %s\n", n, *outFlag)
}
