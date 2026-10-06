package main

import (
	"fmt"
	"sort"
)

// RestartGap 标记一次启动代号变化：旧进程末次采样后的请求无法得知。
type RestartGap struct {
	Instance string `json:"instance"`
	FromTs   int64  `json:"from_ts"`
	ToTs     int64  `json:"to_ts"`
	OldBoot  string `json:"old_boot"`
	NewBoot  string `json:"new_boot"`
}

// WindowResult 是合并后的窗口统计。
type WindowResult struct {
	Bounds     []float64
	BucketDiff []uint64 // 每个桶（含无穷桶）的非累计请求数
	Total      uint64
	SumDiff    float64
	Start      int64
	End        int64
	Gaps       []RestartGap
}

func (w *WindowResult) AvgLatency() (float64, bool) {
	if w.Total == 0 {
		return 0, false
	}
	return w.SumDiff / float64(w.Total), true
}

func (w *WindowResult) RequestsPerSecond() float64 {
	d := w.End - w.Start
	if d <= 0 {
		return 0
	}
	return float64(w.Total) / float64(d)
}

// ComputeWindow 先逐实例计算 [start,end] 内所有相邻区间差值，再合并。
func ComputeWindow(byInstance map[string][]*Snapshot, instances []string, start, end int64) (*WindowResult, error) {
	if end <= start {
		return nil, fmt.Errorf("结束时刻 %d 必须大于起始时刻 %d", end, start)
	}
	res := &WindowResult{Start: start, End: end}
	for _, inst := range instances {
		snaps := byInstance[inst]
		if len(snaps) == 0 {
			return nil, fmt.Errorf("实例 %s 没有任何快照", inst)
		}
		inWindow := []*Snapshot{}
		for _, s := range snaps {
			if s.Ts >= start && s.Ts <= end {
				inWindow = append(inWindow, s)
			}
		}
		if len(inWindow) == 0 || inWindow[0].Ts != start {
			return nil, fmt.Errorf("实例 %s 缺少起始时刻 %d 的精确快照", inst, start)
		}
		if inWindow[len(inWindow)-1].Ts != end {
			return nil, fmt.Errorf("实例 %s 缺少结束时刻 %d 的精确快照", inst, end)
		}
		for _, s := range inWindow {
			if res.Bounds == nil {
				res.Bounds = s.Bounds
			} else if !sameBounds(res.Bounds, s.Bounds) {
				return nil, fmt.Errorf("实例 %s 时刻 %d: 桶界与窗口内其他快照不一致", inst, s.Ts)
			}
		}
		nb := len(res.Bounds) + 1
		if res.BucketDiff == nil {
			res.BucketDiff = make([]uint64, nb)
		}
		for i := 1; i < len(inWindow); i++ {
			prev, cur := inWindow[i-1], inWindow[i]
			var dc []uint64
			var ds float64
			if cur.Boot == prev.Boot {
				d := make([]uint64, nb)
				for b := 0; b < nb; b++ {
					if cur.Counts[b] < prev.Counts[b] {
						return nil, fmt.Errorf("实例 %s 时刻 %d→%d: 同启动代号下桶 %d 计数下降",
							inst, prev.Ts, cur.Ts, b)
					}
					d[b] = cur.Counts[b] - prev.Counts[b]
					if b > 0 && d[b] < d[b-1] {
						return nil, fmt.Errorf("实例 %s 时刻 %d→%d: 桶差累积无序（桶 %d 差值小于前一桶）",
							inst, prev.Ts, cur.Ts, b)
					}
				}
				if cur.Sum < prev.Sum {
					return nil, fmt.Errorf("实例 %s 时刻 %d→%d: 同启动代号下累计总耗时下降",
						inst, prev.Ts, cur.Ts)
				}
				dc, ds = d, cur.Sum-prev.Sum
			} else {
				res.Gaps = append(res.Gaps, RestartGap{
					Instance: inst, FromTs: prev.Ts, ToTs: cur.Ts,
					OldBoot: prev.Boot, NewBoot: cur.Boot,
				})
				dc, ds = cur.Counts, cur.Sum
			}
			// dc 是累计计数差，转成每桶非累计计数再合并
			var prevCum uint64
			for b := 0; b < nb; b++ {
				res.BucketDiff[b] += dc[b] - prevCum
				prevCum = dc[b]
			}
			res.Total += dc[nb-1]
			res.SumDiff += ds
		}
	}
	sort.Slice(res.Gaps, func(i, j int) bool {
		if res.Gaps[i].Instance != res.Gaps[j].Instance {
			return res.Gaps[i].Instance < res.Gaps[j].Instance
		}
		return res.Gaps[i].FromTs < res.Gaps[j].FromTs
	})
	return res, nil
}

// Quantile 在合并后的窗口分布上计算分位数（q ∈ [0,1]）。
// 有限桶内线性插值，首桶下界取零；落入无穷桶返回最后有限上界。
func (w *WindowResult) Quantile(q float64) (float64, bool, error) {
	if q < 0 || q > 1 {
		return 0, false, fmt.Errorf("分位数 %v 超出 [0,1]", q)
	}
	if w.Total == 0 {
		return 0, false, nil
	}
	if q == 0 {
		return 0, true, nil
	}
	target := q * float64(w.Total)
	var cum uint64
	lower := 0.0
	for b := 0; b < len(w.BucketDiff); b++ {
		prevCum := cum
		cum += w.BucketDiff[b]
		if float64(cum) < target {
			if b < len(w.Bounds) {
				lower = w.Bounds[b]
			}
			continue
		}
		if b == len(w.Bounds) {
			return w.Bounds[len(w.Bounds)-1], true, nil // 无穷桶
		}
		upper := w.Bounds[b]
		inBucket := w.BucketDiff[b]
		if inBucket == 0 {
			return upper, true, nil
		}
		frac := (target - float64(prevCum)) / float64(inBucket)
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}
		return lower + frac*(upper-lower), true, nil
	}
	return w.Bounds[len(w.Bounds)-1], true, nil
}
