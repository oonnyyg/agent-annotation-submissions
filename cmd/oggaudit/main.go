// Command oggaudit audits an Ogg container file and exports its logical
// packets plus a JSON index. It validates the whole file before publishing
// any output and never decodes media payloads.
package main

import (
	"flag"
	"fmt"
	"os"

	"example.com/ogg-packet-audit/internal/ogg"
	"example.com/ogg-packet-audit/internal/oggxport"
)

func main() {
	outDir := flag.String("out", "", "output directory for packets and index.json (required)")
	maxPkt := flag.Int64("max-packet-size", ogg.DefaultMaxPacketSize, "maximum allowed packet size in bytes")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: oggaudit -out DIR [flags] FILE.ogg\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *outDir == "" || flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	rep, err := oggxport.Export(f, flag.Arg(0), *outDir, ogg.AuditOptions{MaxPacketSize: *maxPkt})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("ok: %d streams, %d packets, %d pages -> %s\n",
		rep.Streams, rep.Packets, rep.Pages, *outDir)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "oggaudit: %v\n", err)
	os.Exit(1)
}
