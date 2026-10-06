// Command auditor rebuilds a filesystem manifest from ordered tar layers.
//
// Usage:
//
//	auditor manifest layers.json   print the final sorted JSON manifest
//	auditor report   layers.json   print per-layer add/delete/change reports
//	auditor history  layers.json PATH   print the event history of PATH
//
// layers.json is a JSON array of local uncompressed tar layer paths,
// in application order (base layer first).
package main

import (
	"encoding/json"
	"fmt"
	"os"

	auditor "example.com/image-layer-auditor"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: auditor <manifest|report|history> layers.json [path]")
		os.Exit(2)
	}
	cmd := os.Args[1]
	layers, err := readLayerList(os.Args[2])
	if err != nil {
		fatal(err)
	}
	a := auditor.New()
	a.ApplyLayers(layers)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	switch cmd {
	case "manifest":
		emit(enc, a, a.Manifest())
	case "report":
		emit(enc, a, a.Reports)
	case "history":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: auditor history layers.json PATH")
			os.Exit(2)
		}
		emit(enc, a, a.History(os.Args[3]))
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		os.Exit(2)
	}
}

func emit(enc *json.Encoder, a *auditor.Auditor, v any) {
	out := map[string]any{"result": v}
	if a.FailedLayer >= 0 {
		out["failedLayer"] = a.FailedLayer
		out["error"] = a.Err
	}
	if err := enc.Encode(out); err != nil {
		fatal(err)
	}
	if a.FailedLayer >= 0 {
		os.Exit(1)
	}
}

func readLayerList(file string) ([]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var layers []string
	if err := json.Unmarshal(data, &layers); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return layers, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "auditor:", err)
	os.Exit(1)
}
