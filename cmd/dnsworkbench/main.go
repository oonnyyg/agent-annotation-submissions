// dnsworkbench decodes raw DNS messages to JSON and encodes supported JSON
// back to wire format. It never touches the network.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"example.com/dns-wire-workbench/internal/dnswire"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "decode":
		err = runDecode(os.Args[2:])
	case "encode":
		err = runEncode(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		report(err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: dnsworkbench decode [-in packet.bin] [-out packet.json]")
	fmt.Fprintln(os.Stderr, "       dnsworkbench encode [-in packet.json] [-out packet.bin]")
}

// report prints errors as structured JSON; WireError carries offset+reason.
func report(err error) {
	if we, ok := err.(*dnswire.WireError); ok {
		out, _ := json.Marshal(we)
		fmt.Fprintln(os.Stderr, string(out))
		return
	}
	out, _ := json.Marshal(map[string]any{"offset": -1, "reason": err.Error()})
	fmt.Fprintln(os.Stderr, string(out))
}

func parseFlags(args []string) (in, out string, err error) {
	for i := 0; i < len(args); i++ {
		if i+1 >= len(args) {
			return "", "", fmt.Errorf("flag %s needs a value", args[i])
		}
		switch args[i] {
		case "-in":
			in = args[i+1]
		case "-out":
			out = args[i+1]
		default:
			return "", "", fmt.Errorf("unknown flag %s", args[i])
		}
		i++
	}
	return in, out, nil
}

func readInput(path string) ([]byte, error) {
	if path == "" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// writeOutput only writes after the payload is fully built, so a failed run
// never truncates or overwrites an existing output file.
func writeOutput(path string, data []byte) error {
	if path == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func runDecode(args []string) error {
	in, out, err := parseFlags(args)
	if err != nil {
		return err
	}
	raw, err := readInput(in)
	if err != nil {
		return err
	}
	msg, err := dnswire.DecodeMessage(raw)
	if err != nil {
		return err
	}
	doc, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return err
	}
	return writeOutput(out, append(doc, '\n'))
}

func runEncode(args []string) error {
	in, out, err := parseFlags(args)
	if err != nil {
		return err
	}
	raw, err := readInput(in)
	if err != nil {
		return err
	}
	var msg dnswire.Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	wire, err := dnswire.EncodeMessage(&msg)
	if err != nil {
		return err
	}
	return writeOutput(out, wire)
}
