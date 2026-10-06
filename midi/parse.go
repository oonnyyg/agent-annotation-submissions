// Package midi parses Standard MIDI Files (format 0 and 1, PPQN division
// only) and exposes a merged, stably ordered event stream.
package midi

import (
	"encoding/binary"
	"fmt"
)

// EventKind classifies a parsed event.
type EventKind int

const (
	EventNoteOff EventKind = iota
	EventNoteOn
	EventPolyAftertouch
	EventControlChange
	EventProgramChange
	EventChannelAftertouch
	EventPitchBend
	EventMeta
	EventTempo // meta 0x51, MicrosPerQuarter set
	EventOther // skipped meta/sysex payloads
)

// Event is a single decoded MIDI event with absolute tick position.
type Event struct {
	Tick  uint64 // absolute tick within the merged timeline
	Track int    // source track index (file order)
	Seq   int    // index of the event within its track
	Kind  EventKind

	Channel    uint8
	Note       uint8
	Velocity   uint8
	Controller uint8
	Value      uint8
	Pitch      int16

	MetaType         uint8
	MicrosPerQuarter uint32 // valid when Kind == EventTempo
	Length           int    // payload length for skipped meta/sysex events
}

// File is a parsed Standard MIDI File.
type File struct {
	Format   uint16
	Division uint16 // ticks per quarter note (PPQN)
	Events   []Event
	// MaxTick is the largest absolute tick reached by any event.
	MaxTick uint64
}

// ParseError identifies a byte offset within the file where parsing failed.
type ParseError struct {
	Offset int
	Msg    string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("midi: parse error at offset %d: %s", e.Offset, e.Msg)
}

func perr(offset int, format string, args ...any) *ParseError {
	return &ParseError{Offset: offset, Msg: fmt.Sprintf(format, args...)}
}

type reader struct {
	data []byte
	pos  int
}

func (r *reader) remaining() int { return len(r.data) - r.pos }

func (r *reader) bytes(n int) ([]byte, error) {
	if n < 0 || r.remaining() < n {
		return nil, perr(r.pos, "unexpected end of data (need %d bytes, have %d)", n, r.remaining())
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b, nil
}

func (r *reader) u8() (byte, error) {
	b, err := r.bytes(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
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

// varint reads a MIDI variable-length quantity (max 4 bytes).
func (r *reader) varint() (uint32, error) {
	var v uint32
	for i := 0; i < 4; i++ {
		b, err := r.u8()
		if err != nil {
			return 0, err
		}
		v = v<<7 | uint32(b&0x7f)
		if b&0x80 == 0 {
			return v, nil
		}
	}
	return 0, perr(r.pos, "variable-length quantity exceeds 4 bytes")
}

// Parse decodes a Standard MIDI File. Only formats 0 and 1 with PPQN
// (ticks-per-quarter-note) division are supported.
func Parse(data []byte) (*File, error) {
	r := &reader{data: data}
	chunk, err := r.bytes(4)
	if err != nil {
		return nil, perr(0, "file too short for header chunk")
	}
	if string(chunk) != "MThd" {
		return nil, perr(0, "missing MThd header")
	}
	hlen, err := r.u32()
	if err != nil {
		return nil, err
	}
	if hlen < 6 {
		return nil, perr(int(r.pos)-4, "header chunk length %d < 6", hlen)
	}
	format, err := r.u16()
	if err != nil {
		return nil, err
	}
	ntrks, err := r.u16()
	if err != nil {
		return nil, err
	}
	division, err := r.u16()
	if err != nil {
		return nil, err
	}
	if format == 2 {
		return nil, perr(8, "format 2 files are not supported")
	}
	if format > 2 {
		return nil, perr(8, "unknown MIDI format %d", format)
	}
	if division&0x8000 != 0 {
		return nil, perr(12, "SMPTE time division is not supported")
	}
	if division == 0 {
		return nil, perr(12, "division must be > 0 ticks per quarter note")
	}
	// Skip any extra header bytes beyond the mandatory 6.
	if _, err := r.bytes(int(hlen) - 6); err != nil {
		return nil, err
	}

	f := &File{Format: format, Division: division}
	for t := 0; t < int(ntrks); t++ {
		events, maxTick, err := parseTrack(r, t)
		if err != nil {
			return nil, err
		}
		f.Events = append(f.Events, events...)
		if maxTick > f.MaxTick {
			f.MaxTick = maxTick
		}
	}
	return f, nil
}

func parseTrack(r *reader, track int) ([]Event, uint64, error) {
	chunkStart := r.pos
	chunk, err := r.bytes(4)
	if err != nil {
		return nil, 0, perr(chunkStart, "missing MTrk chunk for track %d", track)
	}
	if string(chunk) != "MTrk" {
		return nil, 0, perr(chunkStart, "expected MTrk chunk for track %d, found %q", track, chunk)
	}
	length, err := r.u32()
	if err != nil {
		return nil, 0, err
	}
	bodyStart := r.pos
	body, err := r.bytes(int(length))
	if err != nil {
		return nil, 0, err
	}
	tr := &reader{data: body}
	events, maxTick, err := parseTrackEvents(tr, track)
	if err != nil {
		return nil, 0, offsetErr(err, bodyStart)
	}
	return events, maxTick, nil
}

func parseTrackEvents(tr *reader, track int) ([]Event, uint64, error) {
	var events []Event
	var tick uint64
	var running byte
	seq := 0
	for tr.remaining() > 0 {
		delta, err := tr.varint()
		if err != nil {
			return nil, 0, err
		}
		tick += uint64(delta)
		status, err := tr.u8()
		if err != nil {
			return nil, 0, err
		}
		var ev Event
		ev.Tick = tick
		ev.Track = track
		ev.Seq = seq
		seq++
		switch {
		case status == 0xff:
			mt, err := tr.u8()
			if err != nil {
				return nil, 0, err
			}
			n, err := tr.varint()
			if err != nil {
				return nil, 0, err
			}
			payload, err := tr.bytes(int(n))
			if err != nil {
				return nil, 0, err
			}
			ev.MetaType = mt
			ev.Length = int(n)
			if mt == 0x51 && n == 3 {
				ev.Kind = EventTempo
				ev.MicrosPerQuarter = uint32(payload[0])<<16 | uint32(payload[1])<<8 | uint32(payload[2])
			} else {
				ev.Kind = EventMeta
			}
			running = 0 // meta events clear running status
		case status == 0xf0 || status == 0xf7:
			n, err := tr.varint()
			if err != nil {
				return nil, 0, err
			}
			if _, err := tr.bytes(int(n)); err != nil {
				return nil, 0, err
			}
			ev.Kind = EventOther
			ev.Length = int(n)
			running = 0
		case status >= 0x80 && status < 0xf0:
			running = status
			if err := parseChannelEvent(tr, &ev, status); err != nil {
				return nil, 0, err
			}
		case status < 0x80:
			if running == 0 {
				return nil, 0, perr(tr.pos-1, "data byte 0x%02x without running status", status)
			}
			tr.pos-- // the data byte is the first payload byte
			if err := parseChannelEvent(tr, &ev, running); err != nil {
				return nil, 0, err
			}
		default:
			return nil, 0, perr(tr.pos-1, "unsupported status byte 0x%02x", status)
		}
		events = append(events, ev)
	}
	return events, tick, nil
}

// offsetErr converts a track-relative parse error into a file offset.
func offsetErr(err error, base int) error {
	if pe, ok := err.(*ParseError); ok {
		return &ParseError{Offset: pe.Offset + base, Msg: pe.Msg}
	}
	return err
}

func parseChannelEvent(tr *reader, ev *Event, status byte) error {
	kind := status & 0xf0
	ev.Channel = status & 0x0f
	data1, err := tr.u8()
	if err != nil {
		return err
	}
	if data1&0x80 != 0 {
		return perr(tr.pos-1, "expected data byte, got 0x%02x", data1)
	}
	twoBytes := true
	switch kind {
	case 0x80:
		ev.Kind = EventNoteOff
	case 0x90:
		ev.Kind = EventNoteOn
	case 0xa0:
		ev.Kind = EventPolyAftertouch
	case 0xb0:
		ev.Kind = EventControlChange
	case 0xc0:
		ev.Kind = EventProgramChange
		twoBytes = false
	case 0xd0:
		ev.Kind = EventChannelAftertouch
		twoBytes = false
	case 0xe0:
		ev.Kind = EventPitchBend
	}
	data2 := byte(0)
	if twoBytes {
		data2, err = tr.u8()
		if err != nil {
			return err
		}
		if data2&0x80 != 0 {
			return perr(tr.pos-1, "expected data byte, got 0x%02x", data2)
		}
	}
	switch ev.Kind {
	case EventNoteOff, EventNoteOn:
		ev.Note = data1
		ev.Velocity = data2
	case EventControlChange:
		ev.Controller = data1
		ev.Value = data2
	case EventPitchBend:
		ev.Pitch = int16(data1) | int16(data2)<<7
	default:
		ev.Note = data1
		ev.Value = data2
	}
	return nil
}
