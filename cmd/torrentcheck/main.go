// Command torrentcheck verifies local files against a v1 .torrent file and
// prints a JSON report. It never contacts peers and never modifies content.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"example.com/torrent-piece-auditor/internal/audit"
	"example.com/torrent-piece-auditor/internal/torrent"
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: torrentcheck <file.torrent> <content-root>\n")
	os.Exit(2)
}

func main() {
	if len(os.Args) != 3 {
		usage()
	}
	torrentPath, root := os.Args[1], os.Args[2]

	data, err := os.ReadFile(torrentPath)
	if err != nil {
		fatal("read torrent: %v", err)
	}
	meta, err := torrent.Parse(data)
	if err != nil {
		fatal("%v", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		fatal("content root %q is not a directory", root)
	}
	rep, err := audit.Run(meta, root, torrentPath)
	if err != nil {
		fatal("%v", err)
	}
	out, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fatal("encode report: %v", err)
	}
	fmt.Println(string(out))
	if !rep.Summary.Verified {
		os.Exit(1)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "torrentcheck: "+format+"\n", args...)
	os.Exit(2)
}
