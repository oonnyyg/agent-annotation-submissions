// Command packcheck inspects a Git pack file offline, reconstructs all
// objects, and exports them with a manifest. It never invokes git and
// never touches any repository.
package main

import (
	"flag"
	"fmt"
	"os"

	pack "example.com/git-pack-object-reader"
)

func main() {
	var (
		packPath = flag.String("pack", "", "path to the .pack file to inspect (required)")
		outDir   = flag.String("out", "", "output directory for objects and manifest (required, must not exist)")
		maxSize  = flag.Int64("max-object-size", pack.DefaultLimits.MaxObjectSize, "maximum reconstructed object size in bytes")
		maxDepth = flag.Int("max-delta-depth", pack.DefaultLimits.MaxDeltaDepth, "maximum delta chain depth")
	)
	flag.Parse()

	if *packPath == "" || *outDir == "" {
		fmt.Fprintln(os.Stderr, "usage: packcheck -pack <file.pack> -out <dir> [-max-object-size N] [-max-delta-depth N]")
		os.Exit(2)
	}

	data, err := os.ReadFile(*packPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "packcheck: read pack: %v\n", err)
		os.Exit(1)
	}

	f, err := pack.Parse(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "packcheck: invalid pack: %v\n", err)
		os.Exit(1)
	}

	objects, err := f.Resolve(pack.Limits{MaxObjectSize: *maxSize, MaxDeltaDepth: *maxDepth})
	if err != nil {
		fmt.Fprintf(os.Stderr, "packcheck: resolve objects: %v\n", err)
		os.Exit(1)
	}

	if err := pack.Export(*outDir, objects); err != nil {
		fmt.Fprintf(os.Stderr, "packcheck: export: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("pack: %d entries, version %d, checksum %x\n", f.Declared, f.Version, f.Checksum)
	fmt.Print(pack.Manifest(objects))
	fmt.Printf("exported %d objects to %s\n", len(objects), *outDir)
}
