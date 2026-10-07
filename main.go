package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

var out = json.NewEncoder(os.Stdout)

func emit(v any) {
	if err := out.Encode(v); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func main() {
	dir := flag.String("dir", ".", "replica state directory")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fatal(fmt.Errorf("usage: tagsync -dir D <init|add|remove|list|explain|export|merge> ..."))
	}
	cmd, rest := args[0], args[1:]

	switch cmd {
	case "init":
		fs := flag.NewFlagSet("init", flag.ExitOnError)
		replica := fs.String("replica", "", "unique replica identity")
		fs.Parse(rest)
		if *replica == "" {
			fatal(fmt.Errorf("init requires -replica"))
		}
		s, err := LoadStore(*dir, *replica)
		if err != nil {
			fatal(err)
		}
		if err := SaveStore(*dir, s); err != nil {
			fatal(err)
		}
		emit(map[string]any{"cmd": "init", "replica": s.Replica, "next_seq": s.NextSeq})

	case "add":
		if len(rest) == 0 {
			fatal(fmt.Errorf("add requires at least one tag"))
		}
		s := mustLoad(*dir)
		for _, tag := range rest {
			ev := s.Add(tag)
			emit(map[string]any{"cmd": "add", "tag": tag, "mark": ev.Mark})
		}
		mustSave(*dir, s)

	case "remove":
		if len(rest) == 0 {
			fatal(fmt.Errorf("remove requires at least one tag"))
		}
		s := mustLoad(*dir)
		for _, tag := range rest {
			marks := s.Remove(tag)
			emit(map[string]any{"cmd": "remove", "tag": tag, "tombstoned": marks})
		}
		mustSave(*dir, s)

	case "list":
		s := mustLoad(*dir)
		emit(map[string]any{"cmd": "list", "tags": s.VisibleTags()})

	case "explain":
		s := mustLoad(*dir)
		only := ""
		if len(rest) > 0 {
			only = rest[0]
		}
		for _, ev := range s.Explain(only) {
			emit(ev)
		}

	case "export":
		s := mustLoad(*dir)
		emit(s.ExportSnapshot())

	case "merge":
		if len(rest) != 1 {
			fatal(fmt.Errorf("merge requires one snapshot file path"))
		}
		data, err := os.ReadFile(rest[0])
		if err != nil {
			fatal(err)
		}
		var snap Snapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			fatal(fmt.Errorf("invalid snapshot: %w", err))
		}
		s := mustLoad(*dir)
		if err := s.MergeSnapshot(snap); err != nil {
			fatal(err)
		}
		mustSave(*dir, s)
		emit(map[string]any{"cmd": "merge", "from": snap.Replica, "tags": s.VisibleTags()})

	default:
		fatal(fmt.Errorf("unknown command %q", cmd))
	}
}

func mustLoad(dir string) *Store {
	s, err := LoadStore(dir, "")
	if err != nil {
		fatal(err)
	}
	return s
}

func mustSave(dir string, s *Store) {
	if err := SaveStore(dir, s); err != nil {
		fatal(err)
	}
}
