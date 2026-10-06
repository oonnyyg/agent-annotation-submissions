package midi

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// WriteCSV 输出音符表 CSV。
func WriteCSV(w io.Writer, res *Result) error {
	notes := make([]Note, len(res.Notes))
	copy(notes, res.Notes)
	sort.SliceStable(notes, func(i, j int) bool {
		if notes[i].Track != notes[j].Track {
			return notes[i].Track < notes[j].Track
		}
		if notes[i].OnTick != notes[j].OnTick {
			return notes[i].OnTick < notes[j].OnTick
		}
		return notes[i].Pitch < notes[j].Pitch
	})
	if _, err := fmt.Fprintln(w, "track,channel,pitch,velocity,on_tick,off_tick,end_tick,on_sec,off_sec,end_sec,pedal_held,truncated"); err != nil {
		return err
	}
	for _, n := range notes {
		_, err := fmt.Fprintf(w, "%d,%d,%d,%d,%d,%d,%d,%.6f,%.6f,%.6f,%t,%t\n",
			n.Track, n.Channel, n.Pitch, n.Velocity,
			n.OnTick, n.OffTick, n.EndTick,
			n.OnSec, n.OffSec, n.EndSec,
			n.PedalHeld, n.Truncated)
		if err != nil {
			return err
		}
	}
	return nil
}

type jsonOutput struct {
	EndTick      uint64         `json:"end_tick"`
	EndSec       float64        `json:"end_sec"`
	MaxPolyphony map[string]int `json:"max_polyphony"`
	Anomalies    []Anomaly      `json:"anomalies"`
}

// WriteJSON 输出异常汇总 JSON。
func WriteJSON(w io.Writer, res *Result) error {
	out := jsonOutput{
		EndTick:      res.EndTick,
		EndSec:       res.EndSec,
		MaxPolyphony: make(map[string]int),
		Anomalies:    res.Anomalies,
	}
	for track, n := range res.MaxPolyphony {
		out.MaxPolyphony[fmt.Sprintf("track_%d", track)] = n
	}
	if out.Anomalies == nil {
		out.Anomalies = []Anomaly{}
	}
	sort.SliceStable(out.Anomalies, func(i, j int) bool {
		if out.Anomalies[i].Tick != out.Anomalies[j].Tick {
			return out.Anomalies[i].Tick < out.Anomalies[j].Tick
		}
		return out.Anomalies[i].Type < out.Anomalies[j].Type
	})
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// Summary 返回简短文本摘要（供 CLI 打印）。
func Summary(res *Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "音符 %d 个，异常 %d 条，文件结束 tick %d（%.3f 秒）",
		len(res.Notes), len(res.Anomalies), res.EndTick, res.EndSec)
	tracks := make([]int, 0, len(res.MaxPolyphony))
	for t := range res.MaxPolyphony {
		tracks = append(tracks, t)
	}
	sort.Ints(tracks)
	for _, t := range tracks {
		fmt.Fprintf(&b, "\n轨道 %d 最大同时发声数 %d", t, res.MaxPolyphony[t])
	}
	return b.String()
}
