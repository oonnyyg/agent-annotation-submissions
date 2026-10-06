// midinotes 是离线 MIDI 音符时间线分析器。
// 用法：midinotes <input.mid> [notes.csv] [anomalies.json]
package main

import (
	"fmt"
	"os"

	"example.com/midi-note-timeline/midi"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: midinotes <input.mid> [notes.csv] [anomalies.json]")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取文件失败:", err)
		os.Exit(1)
	}
	f, err := midi.Parse(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解析失败:", err)
		os.Exit(1)
	}
	res := midi.Analyze(f)

	csvPath, jsonPath := "notes.csv", "anomalies.json"
	if len(os.Args) > 2 {
		csvPath = os.Args[2]
	}
	if len(os.Args) > 3 {
		jsonPath = os.Args[3]
	}
	if err := writeFile(csvPath, func(w *os.File) error { return midi.WriteCSV(w, res) }); err != nil {
		fmt.Fprintln(os.Stderr, "写出 CSV 失败:", err)
		os.Exit(1)
	}
	if err := writeFile(jsonPath, func(w *os.File) error { return midi.WriteJSON(w, res) }); err != nil {
		fmt.Fprintln(os.Stderr, "写出 JSON 失败:", err)
		os.Exit(1)
	}
	fmt.Println(midi.Summary(res))
	fmt.Printf("已写出 %s 与 %s\n", csvPath, jsonPath)
}

func writeFile(path string, fn func(*os.File) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return fn(f)
}
