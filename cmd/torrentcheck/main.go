// Command torrentcheck verifies a v1 torrent's content files against the
// piece hashes in its metainfo and prints a JSON report. It is read-only:
// it never contacts peers and never modifies the checked files.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"example.com/torrent-piece-auditor/internal/audit"
	"example.com/torrent-piece-auditor/internal/torrent"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: %s <torrent-file> <content-root>\n", os.Args[0])
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fatal(err)
	}
	meta, err := torrent.Parse(data)
	if err != nil {
		fatal(err)
	}
	report, err := audit.Run(meta, os.Args[2])
	if err != nil {
		fatal(err)
	}
	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))
	if report.Summary.PiecesCorrupt > 0 || report.Summary.PiecesUnverifiable > 0 ||
		report.Summary.FilesMissing > 0 || report.Summary.FilesLengthBad > 0 {
		os.Exit(1)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "torrentcheck:", err)
	os.Exit(2)
}
