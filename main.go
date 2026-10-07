package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// 用法：jsaudit <schema.json> <instances.jsonl>
// 逐行输出校验结果（JSON），模式错误先于任何实例结果报告。
func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: jsaudit <schema.json> <instances.jsonl>")
		os.Exit(2)
	}
	os.Exit(run(os.Args[1], os.Args[2], os.Stdout))
}

type lineResult struct {
	Line   int      `json:"line"`
	OK     bool     `json:"ok"`
	Kind   string   `json:"kind,omitempty"` // "parse" | "schema"
	Errors []errOut `json:"errors,omitempty"`
}

type errOut struct {
	Instance string `json:"instance"`
	Schema   string `json:"schema"`
	Message  string `json:"message"`
}

func run(schemaPath, jsonlPath string, out io.Writer) int {
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		fmt.Fprintf(out, `{"line":0,"ok":false,"kind":"schema","errors":[{"instance":"#","schema":"#","message":%q}]}`+"\n",
			"cannot read schema file: "+err.Error())
		return 1
	}
	root, schemaErrs := CompileSchema(schemaData)
	if len(schemaErrs) > 0 {
		// 模式错误：在校验任何数据前拒绝，退出码 2。
		r := lineResult{Line: 0, OK: false, Kind: "schema"}
		for _, se := range schemaErrs {
			r.Errors = append(r.Errors, errOut{Instance: "#", Schema: orRoot(se.Path), Message: se.Reason})
		}
		writeJSON(out, r)
		return 2
	}

	f, err := os.Open(jsonlPath)
	if err != nil {
		fmt.Fprintf(out, `{"line":0,"ok":false,"kind":"schema","errors":[{"instance":"#","schema":"#","message":%q}]}`+"\n",
			"cannot read instances file: "+err.Error())
		return 1
	}
	defer f.Close()

	exit := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		text := strings.TrimSpace(scanner.Text())
		if lineNo == 1 {
			text = strings.TrimPrefix(text, "\ufeff") // 容忍 UTF-8 BOM
		}
		if text == "" {
			continue // 跳过空行
		}
		dec := json.NewDecoder(strings.NewReader(text))
		dec.UseNumber()
		var inst any
		if err := dec.Decode(&inst); err != nil {
			// 坏 JSON 行单独失败，后续行继续。
			writeJSON(out, lineResult{
				Line: lineNo, OK: false, Kind: "parse",
				Errors: []errOut{{Instance: "#", Schema: "#", Message: "invalid JSON: " + err.Error()}},
			})
			exit = 1
			continue
		}
		errs := Validate(root, inst)
		if len(errs) == 0 {
			writeJSON(out, lineResult{Line: lineNo, OK: true})
			continue
		}
		r := lineResult{Line: lineNo, OK: false}
		for _, e := range errs {
			r.Errors = append(r.Errors, errOut{
				Instance: orRoot(e.InstancePath),
				Schema:   orRoot(e.SchemaPath),
				Message:  e.Message,
			})
		}
		writeJSON(out, r)
		exit = 1
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(out, `{"line":0,"ok":false,"kind":"parse","errors":[{"instance":"#","schema":"#","message":%q}]}`+"\n",
			"read error: "+err.Error())
		return 1
	}
	return exit
}

func writeJSON(w io.Writer, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintln(w, string(b))
}
