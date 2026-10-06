package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/rolling-delta-transfer/internal/delta"
)

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "blockdelta:", err)
	os.Exit(1)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func cmdSig(args []string) {
	fs := flag.NewFlagSet("sig", flag.ExitOnError)
	block := fs.Int("block", 4096, "block size in bytes (positive integer)")
	in := fs.String("in", "", "old (basis) file")
	out := fs.String("out", "", "signature output file (JSON)")
	fs.Parse(args)
	if *in == "" || *out == "" {
		fs.Usage()
		os.Exit(2)
	}
	f, err := os.Open(*in)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	sig, err := delta.BuildSignature(f, *block)
	if err != nil {
		fatal(err)
	}
	if err := writeJSON(*out, sig); err != nil {
		fatal(err)
	}
	fmt.Printf("signature: %d blocks, block size %d, file size %d\n",
		len(sig.Blocks), sig.BlockSize, sig.FileSize)
}

func cmdDelta(args []string) {
	fs := flag.NewFlagSet("delta", flag.ExitOnError)
	sigPath := fs.String("sig", "", "signature file (JSON)")
	in := fs.String("in", "", "new file")
	out := fs.String("out", "", "delta output file (JSON)")
	fs.Parse(args)
	if *sigPath == "" || *in == "" || *out == "" {
		fs.Usage()
		os.Exit(2)
	}
	var sig delta.Signature
	if err := readJSON(*sigPath, &sig); err != nil {
		fatal(err)
	}
	newData, err := os.ReadFile(*in)
	if err != nil {
		fatal(err)
	}
	d, reused, literal, err := delta.Generate(&sig, newData)
	if err != nil {
		fatal(err)
	}
	if err := writeJSON(*out, d); err != nil {
		fatal(err)
	}
	fmt.Printf("delta: %d ops, reused %d bytes, literal %d bytes (target %d bytes)\n",
		len(d.Ops), reused, literal, d.TargetSize)
}

func cmdApply(args []string) {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	sigPath := fs.String("sig", "", "signature file (JSON)")
	deltaPath := fs.String("delta", "", "delta file (JSON)")
	oldPath := fs.String("old", "", "old (basis) file")
	out := fs.String("out", "", "target output path (must differ from old file)")
	fs.Parse(args)
	if *sigPath == "" || *deltaPath == "" || *oldPath == "" || *out == "" {
		fs.Usage()
		os.Exit(2)
	}
	var sig delta.Signature
	if err := readJSON(*sigPath, &sig); err != nil {
		fatal(err)
	}
	if err := sig.Validate(); err != nil {
		fatal(fmt.Errorf("invalid signature: %w", err))
	}
	var d delta.Delta
	if err := readJSON(*deltaPath, &d); err != nil {
		fatal(err)
	}
	if err := delta.ApplyToFile(&d, &sig, *oldPath, *out); err != nil {
		fatal(err)
	}
	fmt.Printf("applied: wrote %d bytes to %s\n", d.TargetSize, *out)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: blockdelta <sig|delta|apply> [flags]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "sig":
		cmdSig(os.Args[2:])
	case "delta":
		cmdDelta(os.Args[2:])
	case "apply":
		cmdApply(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
		os.Exit(2)
	}
}
