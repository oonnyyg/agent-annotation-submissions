package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Config is the JSON configuration read by gzipcheck.
type Config struct {
	Input            string `json:"input"`
	Output           string `json:"output,omitempty"` // empty = check only
	MaxHeaderBytes   int64  `json:"maxHeaderBytes,omitempty"`
	MaxMembers       int    `json:"maxMembers,omitempty"`
	MaxExpandedBytes int64  `json:"maxExpandedBytes,omitempty"`
}

// Report is the JSON document printed to stdout.
type Report struct {
	OK                 bool           `json:"ok"`
	Input              string         `json:"input"`
	Output             string         `json:"output,omitempty"`
	MemberCount        int            `json:"memberCount"`
	TotalExpandedBytes int64          `json:"totalExpandedBytes"`
	Members            []MemberReport `json:"members"`
	Error              *CheckError    `json:"error,omitempty"`
}

const (
	exitOK       = 0
	exitInvalid  = 1 // gzip validation failed
	exitUsageErr = 2 // config / IO / usage errors
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: gzipcheck <config.json>")
		return exitUsageErr
	}
	cfgData, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "read config:", err)
		return exitUsageErr
	}
	var cfg Config
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "parse config:", err)
		return exitUsageErr
	}
	if cfg.Input == "" {
		fmt.Fprintln(os.Stderr, "config: input is required")
		return exitUsageErr
	}

	rep := Report{Input: cfg.Input, Output: cfg.Output}
	code := exitInvalid
	members, err := check(cfg)
	rep.Members = members
	rep.MemberCount = len(members)
	for _, m := range members {
		rep.TotalExpandedBytes += m.ExpandedBytes
	}
	if err != nil {
		var ce *CheckError
		if errors.As(err, &ce) {
			rep.Error = ce
		} else {
			rep.Error = &CheckError{Kind: ErrBadHeader, Message: err.Error()}
			code = exitUsageErr
		}
	} else {
		rep.OK = true
		code = exitOK
	}
	out, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(out))
	return code
}

// check validates the input and, on full success, publishes the output file.
func check(cfg Config) ([]MemberReport, error) {
	in, err := os.Open(cfg.Input)
	if err != nil {
		return nil, err
	}
	defer in.Close()

	var tmp *os.File
	var dst io.Writer
	if cfg.Output != "" {
		if _, err := os.Stat(cfg.Output); err == nil {
			return nil, fmt.Errorf("output %q already exists", cfg.Output)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		dir := filepath.Dir(cfg.Output)
		tmp, err = os.CreateTemp(dir, ".gzipcheck-*")
		if err != nil {
			return nil, err
		}
		defer func() {
			tmp.Close()
			os.Remove(tmp.Name())
		}()
		dst = tmp
	}

	members, err := checkMembers(in, dst, Limits{
		MaxHeaderBytes:   cfg.MaxHeaderBytes,
		MaxMembers:       cfg.MaxMembers,
		MaxExpandedBytes: cfg.MaxExpandedBytes,
	})
	if err != nil {
		return members, err
	}
	if tmp != nil {
		if err := tmp.Close(); err != nil {
			return members, err
		}
		if err := os.Rename(tmp.Name(), cfg.Output); err != nil {
			return members, err
		}
	}
	return members, nil
}
