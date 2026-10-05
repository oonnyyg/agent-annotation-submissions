package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "anonymize":
		err = runAnonymize(os.Args[2:])
	case "check":
		err = runCheck(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  anonymizer anonymize -data in.csv -hierarchy h.json -k 2 -l 2 -budget 3 \
      -weights 1,1 -suppress-cost 1 -public out.csv -report report.json
  anonymizer check -data in.csv -hierarchy h.json -k 2 -l 2 -budget 3 \
      -weights 1,1 -suppress-cost 1 -steps 1,0`)
}

func commonFlags(fs *flag.FlagSet) (data, hier *string, k, l, budget, scost *int, weights *string) {
	data = fs.String("data", "", "input CSV file")
	hier = fs.String("hierarchy", "", "hierarchy JSON file")
	k = fs.Int("k", 0, "minimum group size (positive integer)")
	l = fs.Int("l", 0, "minimum distinct sensitive values per group (positive integer)")
	budget = fs.Int("budget", 0, "maximum suppressed rows (non-negative integer)")
	scost = fs.Int("suppress-cost", 0, "cost per suppressed record (positive integer)")
	weights = fs.String("weights", "", "comma-separated positive integer weights per QI column")
	return
}

func loadAndConfig(data, hier string, k, l, budget, scost int, weights string) (*Dataset, map[string]*Hierarchy, *Config, error) {
	ds, err := LoadDataset(data)
	if err != nil {
		return nil, nil, nil, err
	}
	hs, err := LoadHierarchies(hier)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := ValidateLeaves(ds, hs); err != nil {
		return nil, nil, nil, err
	}
	ws, err := parseInts(weights)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("weights: %w", err)
	}
	cfg := &Config{MinGroupSize: k, MinDistinctSens: l, Budget: budget, Weights: ws, SuppressCost: scost}
	if err := cfg.Validate(len(ds.QIColumns)); err != nil {
		return nil, nil, nil, err
	}
	return ds, hs, cfg, nil
}

func runAnonymize(args []string) error {
	fs := flag.NewFlagSet("anonymize", flag.ContinueOnError)
	data, hier, k, l, budget, scost, weights := commonFlags(fs)
	pub := fs.String("public", "", "output public CSV file")
	rep := fs.String("report", "", "output internal report JSON file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *data == "" || *hier == "" || *pub == "" || *rep == "" {
		return fmt.Errorf("-data, -hierarchy, -public and -report are required")
	}
	ds, hs, cfg, err := loadAndConfig(*data, *hier, *k, *l, *budget, *scost, *weights)
	if err != nil {
		return err
	}
	best, err := Search(ds, hs, cfg)
	if err != nil {
		return err
	}
	if err := WritePublicCSV(*pub, ds, best); err != nil {
		return err
	}
	if err := WriteReport(*rep, ds, cfg, best, *pub); err != nil {
		return err
	}
	fmt.Printf("steps=%v cost=%d suppressed=%d kept=%d\n", best.Steps, best.Cost, best.Suppressed, len(best.Kept))
	return nil
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	data, hier, k, l, budget, scost, weights := commonFlags(fs)
	stepsStr := fs.String("steps", "", "comma-separated generalization steps per QI column")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *data == "" || *hier == "" || *stepsStr == "" {
		return fmt.Errorf("-data, -hierarchy and -steps are required")
	}
	ds, hs, cfg, err := loadAndConfig(*data, *hier, *k, *l, *budget, *scost, *weights)
	if err != nil {
		return err
	}
	steps, err := parseInts(*stepsStr)
	if err != nil {
		return fmt.Errorf("steps: %w", err)
	}
	for i, s := range steps {
		if s < 0 {
			return fmt.Errorf("steps: value %d at position %d is negative", s, i+1)
		}
	}
	ev, err := Evaluate(ds, hs, cfg, steps)
	if err != nil {
		return err
	}
	fmt.Printf("feasible=%v cost=%d suppressed=%d kept=%d\n", ev.Feasible, ev.Cost, ev.Suppressed, len(ev.Kept))
	if !ev.Feasible {
		fmt.Println("reason:", ev.FailReason)
	}
	for _, g := range ev.Groups {
		fmt.Printf("group %v size=%d distinctSensitive=%d ok=%v\n", g.Key, g.Size, g.DistinctSens, g.OK)
	}
	return nil
}

func parseInts(s string) ([]int, error) {
	if s == "" {
		return nil, fmt.Errorf("empty list")
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", p)
		}
		out = append(out, n)
	}
	return out, nil
}
