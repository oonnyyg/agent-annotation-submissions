package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	dataDir := flag.String("data", defaultDataDir(), "directory holding the store file")
	flag.Parse()
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mailboxd:", err)
		os.Exit(1)
	}
	store, err := LoadStore(filepath.Join(*dataDir, "store.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "mailboxd:", err)
		os.Exit(1)
	}
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	enc := json.NewEncoder(out)
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		cmd, err := ParseCommand(line)
		if err != nil {
			enc.Encode(fail(err))
			continue
		}
		enc.Encode(Execute(store, cmd))
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "mailboxd: read:", err)
		os.Exit(1)
	}
}

func defaultDataDir() string {
	if dir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(dir, ".mailbox-sequence-store")
	}
	return ".mailbox-sequence-store"
}
