package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type QuantileReport struct {
	Q     float64  `json:"q"`
	Value *float64 `json:"value"` // 零请求时为 null
}

type BucketReport struct {
	UpperBound string `json:"upper_bound"` // 无穷桶为 "+Inf"
	Count      uint64 `json:"count"`
}

type Report struct {
	Instances         []string         `json:"instances"`
	Start             int64            `json:"start"`
	End               int64            `json:"end"`
	TotalRequests     uint64           `json:"total_requests"`
	AvgLatency        *float64         `json:"avg_latency"` // 零请求时为 null
	RequestsPerSecond float64          `json:"requests_per_second"`
	Quantiles         []QuantileReport `json:"quantiles"`
	Buckets           []BucketReport   `json:"buckets"`
	RestartGaps       []RestartGap     `json:"restart_gaps"`
}

func buildReport(res *WindowResult, instances []string, qs []float64) (*Report, error) {
	rep := &Report{
		Instances:         instances,
		Start:             res.Start,
		End:               res.End,
		TotalRequests:     res.Total,
		RequestsPerSecond: res.RequestsPerSecond(),
		RestartGaps:       res.Gaps,
	}
	if rep.RestartGaps == nil {
		rep.RestartGaps = []RestartGap{}
	}
	if avg, ok := res.AvgLatency(); ok {
		rep.AvgLatency = &avg
	}
	for _, q := range qs {
		v, ok, err := res.Quantile(q)
		if err != nil {
			return nil, err
		}
		qr := QuantileReport{Q: q}
		if ok {
			qr.Value = &v
		}
		rep.Quantiles = append(rep.Quantiles, qr)
	}
	for b, c := range res.BucketDiff {
		ub := "+Inf"
		if b < len(res.Bounds) {
			ub = strconv.FormatFloat(res.Bounds[b], 'g', -1, 64)
		}
		rep.Buckets = append(rep.Buckets, BucketReport{UpperBound: ub, Count: c})
	}
	return rep, nil
}

func writeCSV(w io.Writer, rep *Report) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"bucket_upper_bound", "requests"}); err != nil {
		return err
	}
	for _, b := range rep.Buckets {
		if err := cw.Write([]string{b.UpperBound, strconv.FormatUint(b.Count, 10)}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func parseQuantiles(s string) ([]float64, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var qs []float64
	for _, part := range strings.Split(s, ",") {
		q, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, fmt.Errorf("分位数 %q 解析失败: %w", part, err)
		}
		if q < 0 || q > 1 {
			return nil, fmt.Errorf("分位数 %v 超出 [0,1]", q)
		}
		qs = append(qs, q)
	}
	return qs, nil
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("histaudit", flag.ContinueOnError)
	fs.SetOutput(stdout)
	input := fs.String("input", "", "JSONL 快照文件路径（必填）")
	instFlag := fs.String("instances", "", "逗号分隔的实例列表；留空表示输入中的全部实例")
	start := fs.Int64("start", 0, "窗口起始时刻（整数秒，必填）")
	end := fs.Int64("end", 0, "窗口结束时刻（整数秒，必填）")
	quantiles := fs.String("quantiles", "0.5,0.9,0.99", "逗号分隔的分位数，取值 [0,1]")
	jsonOut := fs.String("json", "", "JSON 报告输出路径；留空写到标准输出")
	csvOut := fs.String("csv", "", "分段 CSV 输出路径；留空不生成")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" {
		return fmt.Errorf("必须指定 -input")
	}
	f, err := os.Open(*input)
	if err != nil {
		return err
	}
	defer f.Close()
	byInstance, err := LoadSnapshots(f)
	if err != nil {
		return err
	}
	var instances []string
	if *instFlag == "" {
		for inst := range byInstance {
			instances = append(instances, inst)
		}
		sort.Strings(instances)
	} else {
		for _, p := range strings.Split(*instFlag, ",") {
			instances = append(instances, strings.TrimSpace(p))
		}
	}
	qs, err := parseQuantiles(*quantiles)
	if err != nil {
		return err
	}
	res, err := ComputeWindow(byInstance, instances, *start, *end)
	if err != nil {
		return err
	}
	rep, err := buildReport(res, instances, qs)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if *jsonOut == "" {
		fmt.Fprintln(stdout, string(data))
	} else if err := os.WriteFile(*jsonOut, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if *csvOut != "" {
		cf, err := os.Create(*csvOut)
		if err != nil {
			return err
		}
		defer cf.Close()
		if err := writeCSV(cf, rep); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
