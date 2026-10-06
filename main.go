package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	state := flag.String("state", "", "path to state file (default: $MAILBOX_STATE or ./mailbox-state.json)")
	flag.Parse()

	path := statePath(*state)
	store, err := LoadStore(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Read JSONL commands from stdin, write one JSONL response per line.
	// State is saved after every successful mutating command, so a failed
	// command never persists partial changes.
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1<<20), 16<<20)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()
	enc := json.NewEncoder(writer)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var cmd Command
		if err := json.Unmarshal(line, &cmd); err != nil {
			enc.Encode(Response{OK: false, Error: fmt.Sprintf("invalid JSON: %v", err)})
			continue
		}
		resp, changed := Execute(store, &cmd)
		if changed {
			if err := store.Save(path); err != nil {
				resp = Response{OK: false, Error: fmt.Sprintf("saving state: %v", err)}
			}
		}
		enc.Encode(resp)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
		os.Exit(1)
	}
}
