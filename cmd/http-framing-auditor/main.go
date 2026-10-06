// Command http-framing-auditor audits a local file as a stream of strict
// HTTP/1.1 requests and prints a JSON report. It never opens sockets.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/http-framing-auditor/auditor"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: http-framing-auditor <file>\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(2)
	}
	rep := auditor.Audit(data)
	out, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(2)
	}
	fmt.Println(string(out))
	if rep.Error != nil {
		os.Exit(1)
	}
}
