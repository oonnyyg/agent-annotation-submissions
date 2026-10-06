// Command image-layer-auditor reconstructs a filesystem manifest from an
// ordered list of uncompressed tar layers, fully offline.
//
// Usage:
//
//	image-layer-auditor analyze  layers.json
//	image-layer-auditor manifest layers.json
//	image-layer-auditor history  layers.json <path>
//
// layers.json is a JSON array of tar file paths in layer order.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"example.com/image-layer-auditor/audit"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: image-layer-auditor <analyze|manifest|history> layers.json [path]")
	}
	cmd := args[0]
	files, err := readLayerList(args[1])
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	switch cmd {
	case "analyze":
		res, err := audit.Analyze(files)
		if err != nil {
			return err
		}
		return enc.Encode(res)
	case "manifest":
		res, err := audit.Analyze(files)
		if err != nil {
			return err
		}
		if res.Stopped {
			fmt.Fprintln(os.Stderr, "warning:", res.Error)
		}
		return enc.Encode(res.Manifest)
	case "history":
		if len(args) < 3 {
			return fmt.Errorf("history requires a path argument")
		}
		events, err := audit.PathHistory(files, args[2])
		if err != nil {
			return err
		}
		if events == nil {
			events = []audit.Event{}
		}
		return enc.Encode(events)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func readLayerList(name string) ([]string, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var files []string
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", name, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s: empty layer list", name)
	}
	return files, nil
}
