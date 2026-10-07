// Command packinspect recovers objects from a Git pack file (v2, SHA-1)
// offline and writes a verification manifest plus raw object files.
// It never invokes git and never modifies any repository.
package main

import (
	"flag"
	"fmt"
	"os"

	"example.com/git-pack-object-reader/pack"
)

func main() {
	packPath := flag.String("pack", "", "path to the .pack file (required)")
	outDir := flag.String("out", "", "output directory to create (required, must not exist)")
	maxObj := flag.Int64("max-object-size", pack.DefaultLimits.MaxObjectSize, "maximum recovered object size in bytes")
	maxDepth := flag.Int("max-delta-depth", pack.DefaultLimits.MaxDeltaDepth, "maximum delta chain depth")
	flag.Parse()

	if *packPath == "" || *outDir == "" {
		fmt.Fprintln(os.Stderr, "usage: packinspect -pack FILE -out DIR [-max-object-size N] [-max-delta-depth N]")
		os.Exit(2)
	}

	data, err := os.ReadFile(*packPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reading pack: %v\n", err)
		os.Exit(1)
	}
	p, err := pack.Parse(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	res, err := pack.Resolve(p, pack.Limits{MaxObjectSize: *maxObj, MaxDeltaDepth: *maxDepth})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := res.Export(*outDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(res.Manifest())
	fmt.Fprintf(os.Stderr, "recovered %d objects into %s\n", len(res.Records), *outDir)
}
