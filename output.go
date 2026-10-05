package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
)

// Report is the internal audit JSON document.
type Report struct {
	Purpose      string        `json:"purpose"`
	PublicFile   string        `json:"publicFile"`
	Steps        []int         `json:"steps"`
	Cost         int           `json:"cost"`
	Suppressed   int           `json:"suppressed"`
	Kept         int           `json:"kept"`
	MinGroupSize int           `json:"minGroupSize"`
	MinDistinct  int           `json:"minDistinctSensitive"`
	Budget       int           `json:"budget"`
	Groups       []GroupReport `json:"groups"`
	Rows         []RowReport   `json:"rows"`
	Disclaimer   string        `json:"disclaimer"`
}

// WritePublicCSV writes only generalized quasi-identifier columns and the
// sensitive column. Record ids and suppressed rows are never included.
func WritePublicCSV(path string, ds *Dataset, ev *Evaluation) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create public CSV: %w", err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	header := append(append([]string{}, ds.QIColumns...), "sensitive")
	if err := w.Write(header); err != nil {
		return err
	}
	for _, k := range ev.Kept {
		row := append(append([]string{}, k.QI...), k.Sensitive)
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// WriteReport writes the internal audit JSON.
func WriteReport(path string, ds *Dataset, cfg *Config, ev *Evaluation, publicPath string) error {
	rep := Report{
		Purpose:      "internal audit only: group checks, per-row keep/drop decisions and cost; not for publication",
		PublicFile:   publicPath,
		Steps:        ev.Steps,
		Cost:         ev.Cost,
		Suppressed:   ev.Suppressed,
		Kept:         len(ev.Kept),
		MinGroupSize: cfg.MinGroupSize,
		MinDistinct:  cfg.MinDistinctSens,
		Budget:       cfg.Budget,
		Groups:       ev.Groups,
		Rows:         ev.Rows,
		Disclaimer:   "guarantees hold only under this task's stated model (k-group size and l-distinct sensitive values on the chosen quasi-identifiers); re-identification via other channels is not excluded",
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}
