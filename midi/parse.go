// Package midi 实现离线标准 MIDI 文件分析：解析事件、
// 还原真实时间上的音符区间并统计同时发声数。
package midi

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// 事件类型。
const (
	EvNoteOff     = 0x80
	EvNoteOn      = 0x90
	EvPolyAfter   = 0xA0
	EvControl     = 0xB0
	EvProgram     = 0xC0
	EvChanAfter   = 0xD0
	EvPitchBend   = 0xE0
	EvMeta        = 0xFF
	EvSysex       = 0xF0
	EvSysexEscape = 0xF7
)

// 元事件类型。
const (
	MetaEndOfTrack = 0x2F
	MetaTempo      = 0x51
)

// 控制器编号。
const CCSustain = 64

// DefaultTempo 为默认速度：每四分音符 500000 微秒。
const DefaultTempo = 500000

// Event 是一条已解析的 MIDI 事件。
type Event struct {
	AbsTick uint64 // 绝对 tick
	Track   int    // 轨道原序号（按文件中出现顺序）
	Order   int    // 轨内事件顺序
	Status  byte   // 状态字节（0xFF 为元事件，0xF0/0xF7 为系统独占）
	Channel byte   // 通道（仅通道事件有效）
	Data1   byte
	Data2   byte
	Meta    byte   // 元事件类型
	Tempo   uint32 // 速度元事件载荷：微秒/四分音符
}

// File 是解析后的 MIDI 文件。
type File struct {
	Format   uint16
	Division uint16 // 每四分音符 tick 数（仅支持正时基）
	Events   []Event
}

// Parse 解析标准 MIDI 文件（零型与一型，正时基）。
func Parse(data []byte) (*File, error) {
	r := &reader{data: data}
	magic, err := r.bytes(4)
	if err != nil {
		return nil, fmt.Errorf("读取头块标识失败: %w", err)
	}
	if string(magic) != "MThd" {
		return nil, fmt.Errorf("缺少 MThd 头块")
	}
	hlen, err := r.u32()
	if err != nil {
		return nil, fmt.Errorf("读取头块长度失败: %w", err)
	}
	if hlen < 6 {
		return nil, fmt.Errorf("头块长度 %d 小于 6", hlen)
	}
	format, err := r.u16()
	if err != nil {
		return nil, fmt.Errorf("读取格式失败: %w", err)
	}
	ntrks, err := r.u16()
	if err != nil {
		return nil, fmt.Errorf("读取轨道数失败: %w", err)
	}
	division, err := r.u16()
	if err != nil {
		return nil, fmt.Errorf("读取时基失败: %w", err)
	}
	if format == 2 {
		return nil, fmt.Errorf("不支持二型 MIDI 文件")
	}
	if format > 2 {
		return nil, fmt.Errorf("未知 MIDI 格式 %d", format)
	}
	if division&0x8000 != 0 {
		return nil, fmt.Errorf("不支持 SMPTE 时基 (division=0x%04X)", division)
	}
	if division == 0 {
		return nil, fmt.Errorf("时基为 0 非法")
	}
	// 跳过头块剩余字节。
	if err := r.skip(int(hlen) - 6); err != nil {
		return nil, fmt.Errorf("头块截断: %w", err)
	}

	f := &File{Format: format, Division: division}
	for t := 0; t < int(ntrks); t++ {
		evs, err := parseTrack(r, t)
		if err != nil {
			return nil, fmt.Errorf("轨道 %d: %w", t, err)
		}
		f.Events = append(f.Events, evs...)
	}
	// 稳定排序：绝对 tick、轨道原序号、轨内顺序。
	sort.SliceStable(f.Events, func(i, j int) bool {
		a, b := f.Events[i], f.Events[j]
		if a.AbsTick != b.AbsTick {
			return a.AbsTick < b.AbsTick
		}
		if a.Track != b.Track {
			return a.Track < b.Track
		}
		return a.Order < b.Order
	})
	return f, nil
}

func parseTrack(r *reader, track int) ([]Event, error) {
	magic, err := r.bytes(4)
	if err != nil {
		return nil, fmt.Errorf("读取轨道块标识失败: %w", err)
	}
	if string(magic) != "MTrk" {
		return nil, fmt.Errorf("偏移 %d 处缺少 MTrk 轨道块", r.pos-4)
	}
	length, err := r.u32()
	if err != nil {
		return nil, fmt.Errorf("读取轨道块长度失败: %w", err)
	}
	end := r.pos + int(length)
	if end > len(r.data) {
		return nil, fmt.Errorf("轨道块声明长度 %d 超出文件剩余 %d 字节", length, len(r.data)-r.pos)
	}
	tr := &reader{data: r.data[r.pos:end]}
	r.pos = end

	var events []Event
	var abs uint64
	var running byte
	order := 0
	for tr.pos < len(tr.data) {
		delta, err := tr.varlen()
		if err != nil {
			return nil, fmt.Errorf("事件 %d 的 delta 时间: %w", order, err)
		}
		abs += uint64(delta)
		ev, err := parseEvent(tr, abs, track, order, &running)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
		order++
	}
	return events, nil
}

func parseEvent(r *reader, abs uint64, track, order int, running *byte) (Event, error) {
	ev := Event{AbsTick: abs, Track: track, Order: order}
	b, err := r.byte()
	if err != nil {
		return ev, fmt.Errorf("事件 %d 状态字节: %w", order, err)
	}
	status := b
	if b < 0x80 {
		if *running == 0 {
			return ev, fmt.Errorf("事件 %d: 运行状态缺少前置状态字节", order)
		}
		status = *running
		r.pos-- // 回退，数据字节按状态重新读取
	} else if b < 0xF0 {
		*running = b
	} else {
		*running = 0 // 系统事件清除运行状态
	}
	ev.Status = status

	switch {
	case status == EvMeta:
		meta, err := r.byte()
		if err != nil {
			return ev, fmt.Errorf("事件 %d 元事件类型: %w", order, err)
		}
		length, err := r.varlen()
		if err != nil {
			return ev, fmt.Errorf("事件 %d 元事件长度: %w", order, err)
		}
		if r.pos+int(length) > len(r.data) {
			return ev, fmt.Errorf("事件 %d 元事件长度 %d 越界", order, length)
		}
		ev.Meta = meta
		if meta == MetaTempo {
			if length != 3 {
				return ev, fmt.Errorf("事件 %d 速度元事件长度 %d 不为 3", order, length)
			}
			d := r.data[r.pos : r.pos+3]
			ev.Tempo = uint32(d[0])<<16 | uint32(d[1])<<8 | uint32(d[2])
		}
		r.pos += int(length)
	case status == EvSysex || status == EvSysexEscape:
		length, err := r.varlen()
		if err != nil {
			return ev, fmt.Errorf("事件 %d 系统独占长度: %w", order, err)
		}
		if r.pos+int(length) > len(r.data) {
			return ev, fmt.Errorf("事件 %d 系统独占长度 %d 越界", order, length)
		}
		r.pos += int(length)
	default:
		if status&0xF0 < 0x80 || status&0xF0 > 0xE0 {
			return ev, fmt.Errorf("事件 %d 非法状态字节 0x%02X", order, status)
		}
		ev.Channel = status & 0x0F
		n := 2
		if status&0xF0 == EvProgram || status&0xF0 == EvChanAfter {
			n = 1
		}
		d, err := r.bytes(n)
		if err != nil {
			return ev, fmt.Errorf("事件 %d 数据字节: %w", order, err)
		}
		ev.Data1 = d[0]
		if n == 2 {
			ev.Data2 = d[1]
		}
	}
	return ev, nil
}

type reader struct {
	data []byte
	pos  int
}

func (r *reader) byte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("偏移 %d 处意外截断", r.pos)
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

func (r *reader) bytes(n int) ([]byte, error) {
	if r.pos+n > len(r.data) {
		return nil, fmt.Errorf("偏移 %d 处需要 %d 字节但仅剩 %d", r.pos, n, len(r.data)-r.pos)
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

func (r *reader) skip(n int) error {
	if n < 0 || r.pos+n > len(r.data) {
		return fmt.Errorf("偏移 %d 处跳过 %d 字节越界", r.pos, n)
	}
	r.pos += n
	return nil
}

func (r *reader) u16() (uint16, error) {
	b, err := r.bytes(2)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b), nil
}

func (r *reader) u32() (uint32, error) {
	b, err := r.bytes(4)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(b), nil
}

// varlen 解析 MIDI 变长整数（最多 4 字节）。
func (r *reader) varlen() (uint32, error) {
	var v uint32
	for i := 0; i < 4; i++ {
		b, err := r.byte()
		if err != nil {
			return 0, fmt.Errorf("变长整数截断: %w", err)
		}
		v = v<<7 | uint32(b&0x7F)
		if b&0x80 == 0 {
			return v, nil
		}
	}
	return 0, fmt.Errorf("偏移 %d 处变长整数超过 4 字节", r.pos)
}
