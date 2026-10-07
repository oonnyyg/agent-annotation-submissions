
// Command tagsync manages an observed-remove tag set for one replica.
// It reads JSONL commands from stdin and writes one JSON response per line.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/observed-remove-tags/internal/orset"
)

type command struct {
	Op       string          `json:"op"`
	Tag      string          `json:"tag,omitempty"`
	Snapshot json.RawMessage `json:"snapshot,omitempty"`
}

func respond(ok bool, extra map[string]any, err error) {
	out := map[string]any{"ok": ok}
	for k, v := range extra {
		out[k] = v
	}
	if err != nil {
		out["error"] = err.Error()
	}
	data, _ := json.Marshal(out)
	fmt.Println(string(data))
}

func main() {
	statePath := flag.String("state", "tagsync-state.json", "path to the replica state file")
	replica := flag.String("replica", "", "replica identity (required on first run)")
	flag.Parse()

	st, err := orset.Load(*statePath, *replica)
	if err != nil {
		respond(false, nil, err)
		os.Exit(1)
	}
	dirty := false
	defer func() {
		if dirty {
			if err := st.Save(*statePath); err != nil {
				respond(false, nil, err)
				os.Exit(1)
			}
		}
	}()

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var cmd command
		if err := json.Unmarshal(line, &cmd); err != nil {
			respond(false, nil, fmt.Errorf("invalid command: %w", err))
			continue
		}
		switch cmd.Op {
		case "id":
			respond(true, map[string]any{"replica": st.Replica}, nil)
		case "add":
			d, err := st.Add(cmd.Tag)
			if err == nil {
				dirty = true
			}
			respond(err == nil, map[string]any{"dot": d.String()}, err)
		case "remove":
			dots := st.Remove(cmd.Tag)
			if len(dots) > 0 {
				dirty = true
			}
			strs := make([]string, len(dots))
			for i, d := range dots {
				strs[i] = d.String()
			}
			respond(true, map[string]any{"removed": strs}, nil)
		case "list":
			respond(true, map[string]any{"tags": st.Tags()}, nil)
		case "explain":
			respond(true, map[string]any{"evidence": st.Explain(cmd.Tag)}, nil)
		case "export":
			respond(true, map[string]any{"snapshot": st.Export()}, nil)
		case "merge":
			if len(cmd.Snapshot) == 0 {
				respond(false, nil, fmt.Errorf("merge requires a snapshot field"))
				continue
			}
			err := st.MergeJSON(cmd.Snapshot)
			if err == nil {
				dirty = true
			}
			respond(err == nil, nil, err)
		default:
			respond(false, nil, fmt.Errorf("unknown op %q", cmd.Op))
		}
	}
	if err := sc.Err(); err != nil {
		respond(false, nil, err)
		os.Exit(1)
	}
}
