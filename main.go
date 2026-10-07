package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

type Event struct {
	Op       string   `json:"op"`
	Route    string   `json:"route,omitempty"`
	Train    string   `json:"train,omitempty"`
	Switch   string   `json:"switch,omitempty"`
	Position Position `json:"position,omitempty"`
}

type Decision struct {
	Line     int       `json:"line"`
	Op       string    `json:"op"`
	Decision string    `json:"decision"`
	Reason   string    `json:"reason"`
	Snapshot *Snapshot `json:"snapshot,omitempty"`
}

func replay(il *Interlocking, in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	enc := json.NewEncoder(out)
	line := 0
	for sc.Scan() {
		line++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var ev Event
		d := Decision{Line: line}
		if err := json.Unmarshal(raw, &ev); err != nil {
			d.Op = "?"
			d.Decision = "rejected"
			d.Reason = fmt.Sprintf("事件 JSON 非法: %v", err)
			if err := enc.Encode(d); err != nil {
				return err
			}
			continue
		}
		d.Op = ev.Op
		var err error
		switch ev.Op {
		case "request":
			err = il.RequestRoute(ev.Route)
		case "cancel":
			err = il.CancelRoute(ev.Route)
		case "enter":
			err = il.Enter(ev.Train, ev.Route)
		case "advance":
			err = il.Advance(ev.Train)
		case "set_switch":
			err = il.SetSwitch(ev.Switch, ev.Position)
		case "snapshot":
			snap := il.Snapshot()
			d.Snapshot = &snap
		default:
			err = fmt.Errorf("未知操作 %q", ev.Op)
		}
		if err != nil {
			d.Decision = "rejected"
			d.Reason = err.Error()
		} else {
			d.Decision = "accepted"
			d.Reason = "ok"
		}
		if err := enc.Encode(d); err != nil {
			return err
		}
	}
	return sc.Err()
}

func main() {
	cfgPath := flag.String("config", "", "进路配置 JSON 文件路径（必需）")
	eventsPath := flag.String("events", "", "事件 JSONL 文件路径（缺省读标准输入）")
	flag.Parse()
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "缺少 -config 参数")
		os.Exit(2)
	}
	cfgFile, err := os.Open(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开配置失败: %v\n", err)
		os.Exit(1)
	}
	defer cfgFile.Close()
	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置非法，回放被拒绝: %v\n", err)
		os.Exit(1)
	}
	var in io.Reader = os.Stdin
	if *eventsPath != "" {
		f, err := os.Open(*eventsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "打开事件文件失败: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		in = f
	}
	if err := replay(NewInterlocking(cfg), in, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "回放失败: %v\n", err)
		os.Exit(1)
	}
}
