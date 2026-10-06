package main

import (
	"flag"
	"fmt"
	"os"

	"example.com/rolling-delta-transfer/delta"
)

func usage() {
	fmt.Fprintf(os.Stderr, `usage:
  blockdelta signature -old FILE -block N -out SIG.json
  blockdelta delta     -sig SIG.json -new FILE -out DELTA.json
  blockdelta apply     -old FILE -delta DELTA.json -out TARGET
`)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "signature":
		fs := flag.NewFlagSet("signature", flag.ExitOnError)
		old := fs.String("old", "", "old (basis) file")
		block := fs.Int("block", 4096, "block size in bytes (positive integer)")
		out := fs.String("out", "", "signature output path")
		fs.Parse(os.Args[2:])
		if *old == "" || *out == "" {
			usage()
		}
		err = delta.WriteSignature(*old, *block, *out)
	case "delta":
		fs := flag.NewFlagSet("delta", flag.ExitOnError)
		sig := fs.String("sig", "", "signature file")
		newF := fs.String("new", "", "new file")
		out := fs.String("out", "", "delta output path")
		fs.Parse(os.Args[2:])
		if *sig == "" || *newF == "" || *out == "" {
			usage()
		}
		var stats delta.Stats
		stats, err = delta.WriteDelta(*sig, *newF, *out)
		if err == nil {
			fmt.Printf("reused %d bytes, literal %d bytes\n", stats.Reused, stats.Literal)
		}
	case "apply":
		fs := flag.NewFlagSet("apply", flag.ExitOnError)
		old := fs.String("old", "", "old (basis) file")
		d := fs.String("delta", "", "delta file")
		out := fs.String("out", "", "target output path (must differ from old)")
		fs.Parse(os.Args[2:])
		if *old == "" || *d == "" || *out == "" {
			usage()
		}
		var stats delta.Stats
		stats, err = delta.Apply(*old, *d, *out)
		if err == nil {
			fmt.Printf("rebuilt %s: reused %d bytes, literal %d bytes\n", *out, stats.Reused, stats.Literal)
		}
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
