package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Input 是 CLI 读取的 JSON 文档结构。
type Input struct {
	Config     Config       `json:"config"`
	Passengers []*Passenger `json:"passengers"`
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	var in io.Reader = stdin
	if len(args) > 1 {
		return fmt.Errorf("usage: elevator-replay [input.json] < input.json")
	}
	if len(args) == 1 {
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}
	var input Input
	dec := json.NewDecoder(in)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := input.Config.validate(); err != nil {
		return err
	}
	if err := validatePassengers(input.Config, input.Passengers); err != nil {
		return err
	}

	sim := NewSim(input.Config, input.Passengers)
	sim.Run()

	fmt.Fprintln(stdout, "== 事件日志 ==")
	for _, l := range sim.logs {
		fmt.Fprintln(stdout, l)
	}
	fmt.Fprintln(stdout, "== 乘客行程 ==")
	for _, p := range sim.all {
		fmt.Fprintf(stdout, "乘客 %s: 到达=%d 上车=%d 下车=%d 等待=%d 行程=%d->%d\n",
			p.ID, p.Arrive, p.Board, p.Alight, p.Wait(), p.From, p.To)
	}
	sum := sim.Summary()
	fmt.Fprintln(stdout, "== 运行汇总 ==")
	fmt.Fprintf(stdout, "乘客总数=%d 完成时刻=%d 最大载客=%d/%d 满载滞留人次=%d\n",
		sum.Passengers, sum.FinishTime, sum.MaxLoad, input.Config.Capacity, sum.FullRejects)
	avg := 0.0
	if sum.Passengers > 0 {
		avg = float64(sum.TotalWait) / float64(sum.Passengers)
	}
	fmt.Fprintf(stdout, "总等待=%d 平均等待=%.2f 最大等待=%d\n", sum.TotalWait, avg, sum.MaxWait)
	return nil
}
