// farecap 是本地公共交通计价工具：读取 JSON 费率配置，
// 按 JSONL 导入乘车记录或撤销指令，输出 JSON 结果并保存状态。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/transit-fare-capping/internal/farecap"
)

type inputLine struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Card   string `json:"card"`
	Board  string `json:"board,omitempty"`
	Alight string `json:"alight,omitempty"`
	From   int    `json:"from,omitempty"`
	To     int    `json:"to,omitempty"`
	Target string `json:"target,omitempty"`
}

type outputLine struct {
	Line         int              `json:"line"`
	Type         string           `json:"type"`
	Status       string           `json:"status"`
	Reason       string           `json:"reason,omitempty"`
	ChargedCents int              `json:"chargedCents,omitempty"`
	CardTotal    int              `json:"cardTotalCents,omitempty"`
	Charges      []farecap.Charge `json:"charges,omitempty"`
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "farecap: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	cfgPath := flag.String("config", "config.json", "费率配置 JSON 路径")
	statePath := flag.String("state", "state.json", "状态文件路径")
	inputPath := flag.String("input", "", "JSONL 输入路径（缺省为标准输入）")
	flag.Parse()

	cfgFile, err := os.Open(*cfgPath)
	if err != nil {
		fail("打开配置失败: %v", err)
	}
	cfg, err := farecap.LoadConfig(cfgFile)
	cfgFile.Close()
	if err != nil {
		fail("加载配置失败: %v", err)
	}

	st := farecap.NewState()
	if data, err := os.ReadFile(*statePath); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, st); err != nil {
			fail("解析状态文件失败: %v", err)
		}
		if st.Cards == nil {
			st.Cards = map[string]*farecap.CardState{}
		}
	}
	eng := farecap.NewEngine(cfg, st)

	in := os.Stdin
	if *inputPath != "" {
		f, err := os.Open(*inputPath)
		if err != nil {
			fail("打开输入失败: %v", err)
		}
		defer f.Close()
		in = f
	}

	enc := json.NewEncoder(os.Stdout)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var in2 inputLine
		out := outputLine{Line: lineNo}
		if err := json.Unmarshal(raw, &in2); err != nil {
			out.Status = "rejected"
			out.Reason = "非法 JSON: " + err.Error()
			enc.Encode(out)
			continue
		}
		out.Type = in2.Type
		switch in2.Type {
		case "ride":
			res := eng.AddRide(farecap.Ride{
				ID: in2.ID, Card: in2.Card, Board: in2.Board,
				Alight: in2.Alight, From: in2.From, To: in2.To,
			})
			out.Status, out.Reason = res.Status, res.Reason
			out.ChargedCents, out.CardTotal = res.Charged, res.CardCharged
		case "void":
			res := eng.AddVoid(farecap.VoidRecord{ID: in2.ID, Card: in2.Card, Target: in2.Target})
			out.Status, out.Reason = res.Status, res.Reason
			out.CardTotal = res.CardCharged
		case "query":
			charges, total := eng.Query(in2.Card)
			out.Status = "ok"
			out.Charges = charges
			out.CardTotal = total
		default:
			out.Status = "rejected"
			out.Reason = "未知指令类型: " + in2.Type
		}
		enc.Encode(out)
	}
	if err := sc.Err(); err != nil {
		fail("读取输入失败: %v", err)
	}

	data, err := json.MarshalIndent(eng.State(), "", "  ")
	if err != nil {
		fail("序列化状态失败: %v", err)
	}
	if err := os.WriteFile(*statePath, data, 0o644); err != nil {
		fail("保存状态失败: %v", err)
	}
}
