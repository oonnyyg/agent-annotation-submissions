package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

func decodeJSON(data []byte) (interface{}, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("trailing data after JSON value")
	}
	return v, nil
}

func run(schemaPath, instancesPath string, out io.Writer) int {
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		fmt.Fprintf(out, "error: cannot read schema: %v\n", err)
		return 2
	}
	raw, err := decodeJSON(schemaBytes)
	if err != nil {
		fmt.Fprintf(out, "error: schema is not valid JSON: %v\n", err)
		return 2
	}
	schema, err := CompileSchema(raw)
	if err != nil {
		fmt.Fprintf(out, "error: %v\n", err)
		return 2
	}
	file, err := os.Open(instancesPath)
	if err != nil {
		fmt.Fprintf(out, "error: cannot read instances: %v\n", err)
		return 2
	}
	defer file.Close()

	exitCode := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		inst, err := decodeJSON(line)
		if err != nil {
			fmt.Fprintf(out, "line %d: INVALID JSON: %v\n", lineNo, err)
			exitCode = 1
			continue
		}
		errs := schema.Validate(inst)
		if len(errs) == 0 {
			fmt.Fprintf(out, "line %d: OK\n", lineNo)
			continue
		}
		exitCode = 1
		for _, e := range errs {
			fmt.Fprintf(out, "line %d: REJECT: %s\n", lineNo, e.String())
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(out, "error: reading instances: %v\n", err)
		return 2
	}
	return exitCode
}

func main() {
	schemaPath := flag.String("schema", "", "path to JSON schema file (required)")
	flag.Parse()
	args := flag.Args()
	if *schemaPath == "" || len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: jsonaudit -schema schema.json instances.jsonl")
		os.Exit(2)
	}
	os.Exit(run(*schemaPath, args[0], os.Stdout))
}
