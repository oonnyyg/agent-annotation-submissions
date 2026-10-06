package midi

import "fmt"

// Note 是一个还原出的音符区间。
type Note struct {
	Track     int
	Channel   byte
	Pitch     byte
	Velocity  byte
	OnTick    uint64 // 按键 tick
	OffTick   uint64 // 抬键 tick（踏板踩下时声音仍延续）
	EndTick   uint64 // 声音结束 tick
	OnSec     float64
	OffSec    float64
	EndSec    float64
	PedalHeld bool // 声音结束曾被延音踏板推迟
	Truncated bool // 文件结束仍未关闭，已截到全文件最大 tick
}

// Anomaly 描述一条异常。
type Anomaly struct {
	Type    string `json:"type"`
	Track   int    `json:"track"`
	Channel byte   `json:"channel"`
	Pitch   byte   `json:"pitch"`
	Tick    uint64 `json:"tick"`
	Message string `json:"message"`
}

// Result 是分析结果。
type Result struct {
	Notes        []Note
	Anomalies    []Anomaly
	MaxPolyphony map[int]int // 每轨道最大同时发声数
	EndTick      uint64
	EndSec       float64
}

// key 标识一个 (轨道, 通道, 音高) 键位。
type key struct {
	track   int
	channel byte
	pitch   byte
}

// chanKey 标识一个 (轨道, 通道)。
type chanKey struct {
	track   int
	channel byte
}

// openNote 跟踪进行中的音符。
type openNote struct {
	note Note
	held bool // 已抬键但被踏板延住
}

// Analyze 配对音符、处理延音踏板并统计发声数。
func Analyze(f *File) *Result {
	tl := NewTimeline(f)
	endTick := f.MaxTick()

	open := make(map[key][]*openNote)     // 等待抬键的音符（FIFO）
	held := make(map[chanKey][]*openNote) // 踏板延住的音符
	pedal := make(map[chanKey]bool)

	var notes []Note
	var anomalies []Anomaly

	closeNote := func(on *openNote, end uint64) {
		on.note.EndTick = end
		notes = append(notes, on.note)
	}

	for _, ev := range f.Events {
		switch ev.Status & 0xF0 {
		case EvNoteOn:
			k := key{ev.Track, ev.Channel, ev.Data1}
			if ev.Data2 == 0 {
				// 零力度开启视为关闭
				handleNoteOff(open, held, pedal, k, ev, closeNote, &anomalies)
				break
			}
			open[k] = append(open[k], &openNote{note: Note{
				Track: ev.Track, Channel: ev.Channel, Pitch: ev.Data1,
				Velocity: ev.Data2, OnTick: ev.AbsTick,
			}})
		case EvNoteOff:
			k := key{ev.Track, ev.Channel, ev.Data1}
			handleNoteOff(open, held, pedal, k, ev, closeNote, &anomalies)
		case EvControl:
			if ev.Data1 != CCSustain {
				break
			}
			ck := chanKey{ev.Track, ev.Channel}
			if ev.Data2 >= 64 {
				pedal[ck] = true
			} else {
				pedal[ck] = false
				// 松开踏板：结束该轨道通道所有被延住的音符
				for _, on := range held[ck] {
					closeNote(on, ev.AbsTick)
				}
				delete(held, ck)
			}
		}
	}

	// 文件结束仍未关闭的音符：截到全文件最大 tick 并标记。
	for k, q := range open {
		for _, on := range q {
			on.note.OffTick = endTick
			on.note.Truncated = true
			closeNote(on, endTick)
			anomalies = append(anomalies, Anomaly{
				Type: "unclosed_note", Track: k.track, Channel: k.channel,
				Pitch: k.pitch, Tick: on.note.OnTick,
				Message: fmt.Sprintf("音符在文件结束时仍未关闭，已截到 tick %d", endTick),
			})
		}
	}
	// 未结束的踏板延音同样截到文件结束。
	for _, q := range held {
		for _, on := range q {
			closeNote(on, endTick)
		}
	}

	res := &Result{
		Notes:     notes,
		Anomalies: anomalies,
		EndTick:   endTick,
		EndSec:    tl.Seconds(endTick),
	}
	for i := range res.Notes {
		n := &res.Notes[i]
		n.OnSec = tl.Seconds(n.OnTick)
		n.OffSec = tl.Seconds(n.OffTick)
		n.EndSec = tl.Seconds(n.EndTick)
	}
	res.MaxPolyphony = maxPolyphony(res.Notes)
	return res
}

// noteOff 处理一次抬键：从 FIFO 队列取出最早开启的音符。
func handleNoteOff(open map[key][]*openNote, held map[chanKey][]*openNote,
	pedal map[chanKey]bool, k key, ev Event,
	closeNote func(*openNote, uint64), anomalies *[]Anomaly) {

	q := open[k]
	if len(q) == 0 {
		*anomalies = append(*anomalies, Anomaly{
			Type: "unmatched_note_off", Track: k.track, Channel: k.channel,
			Pitch: k.pitch, Tick: ev.AbsTick,
			Message: fmt.Sprintf("tick %d 处的关闭没有匹配的开启", ev.AbsTick),
		})
		return
	}
	on := q[0]
	open[k] = q[1:]
	on.note.OffTick = ev.AbsTick
	ck := chanKey{k.track, k.channel}
	if pedal[ck] {
		// 踏板踩下：只记抬键，声音延续到踏板松开
		on.held = true
		on.note.PedalHeld = true
		held[ck] = append(held[ck], on)
	} else {
		closeNote(on, ev.AbsTick)
	}
}

// maxPolyphony 以左闭右开声音区间计算每轨道最大同时发声数，零时长不计。
func maxPolyphony(notes []Note) map[int]int {
	type point struct {
		sec   float64
		delta int
	}
	byTrack := make(map[int][]point)
	for _, n := range notes {
		if n.EndSec <= n.OnSec {
			continue // 零时长不计
		}
		byTrack[n.Track] = append(byTrack[n.Track],
			point{n.OnSec, 1}, point{n.EndSec, -1})
	}
	out := make(map[int]int)
	for track, pts := range byTrack {
		// 同一时刻先处理结束（-1）再处理开始（+1），实现左闭右开。
		for i := 0; i < len(pts); i++ {
			for j := i + 1; j < len(pts); j++ {
				if pts[j].sec < pts[i].sec ||
					(pts[j].sec == pts[i].sec && pts[j].delta < pts[i].delta) {
					pts[i], pts[j] = pts[j], pts[i]
				}
			}
		}
		cur, max := 0, 0
		for _, p := range pts {
			cur += p.delta
			if cur > max {
				max = cur
			}
		}
		out[track] = max
	}
	return out
}
