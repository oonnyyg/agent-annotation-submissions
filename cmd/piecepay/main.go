// 计件结算工具：按 JSONL 接收操作并逐行返回 JSON，状态保存在指定目录。
//
// 用法:
//
//	go run ./cmd/piecepay -dir <状态目录>
//
// 标准输入每行一个 JSON 操作，标准输出每行一个 JSON 结果。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	dir := flag.String("dir", "piecepay-data", "状态保存目录")
	flag.Parse()
	if err := run(*dir, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "piecepay:", err)
		os.Exit(1)
	}
}

func run(dir string, in *os.File, out *os.File) error {
	st, err := loadState(dir)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(trimSpace(line)) == 0 {
			continue
		}
		var req Request
		var resp Response
		dirty := false
		if err := json.Unmarshal(line, &req); err != nil {
			resp = fail("第 %d 行 JSON 解析失败: %v", lineNo, err)
		} else {
			resp, dirty = dispatch(st, req)
		}
		if dirty {
			if err := saveState(dir, st); err != nil {
				resp = fail("保存状态失败: %v", err)
			}
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\r') {
		b = b[1:]
	}
	for len(b) > 0 {
		c := b[len(b)-1]
		if c != ' ' && c != '\t' && c != '\r' {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}
