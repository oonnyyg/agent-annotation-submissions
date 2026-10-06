package midi

import (
	"fmt"
	"sort"
)

// SustainPedalController is the MIDI CC number for the sustain pedal.
const SustainPedalController = 64

// Note is a reconstructed note with key and sounding intervals.
type Note struct {
	Track    int
	Channel  uint8
	Pitch    uint8
	Velocity uint8

	KeyOnTick    uint64
	KeyOffTick   uint64 // tick where the key was released
	SoundEndTick uint64 // tick where the sound actually stops

	KeyOnSeconds    float64
	KeyOffSeconds   float64
	SoundEndSeconds float64

	// Unclosed is true when no note-off existed and the note was cut at
	// the file end (reported as an anomaly).
	Unclosed bool
	// PedalTruncated is true when a pedal-sustained note was still
	// sounding at the file end and was cut there.
	PedalTruncated bool
}

// AnomalyType classifies a consistency problem found during analysis.
type AnomalyType string

const (
	AnomalyUnmatchedNoteOff AnomalyType = "unmatched_note_off"
	AnomalyUnclosedNote     AnomalyType = "unclosed_note"
)

// Anomaly describes a note-pairing problem.
type Anomaly struct {
	Type    AnomalyType `json:"type"`
	Track   int         `json:"track"`
	Channel uint8       `json:"channel"`
	Pitch   uint8       `json:"pitch"`
	Tick    uint64      `json:"tick"`
	Message string      `json:"message"`
}

type noteKey struct {
	track   int
	channel uint8
	pitch   uint8
}

type pedalKey struct {
	track   int
	channel uint8
}

// Analysis is the result of reconstructing notes from a parsed file.
type Analysis struct {
	Notes     []Note
	Anomalies []Anomaly
	// MaxPolyphony per track index, computed over [start, end) sounding
	// intervals in seconds; zero-duration notes are ignored.
	MaxPolyphony map[int]int
}

// Analyze reconstructs notes from the merged event stream.
func (f *File) Analyze() *Analysis {
	tempo := f.NewTempoMap()
	a := &Analysis{MaxPolyphony: map[int]int{}}

	open := map[noteKey][]*Note{}      // key held, sounding, FIFO
	sustained := map[noteKey][]*Note{} // key released, held by pedal
	pedalDown := map[pedalKey]bool{}

	finalize := func(n *Note) {
		n.KeyOnSeconds = tempo.Seconds(n.KeyOnTick)
		n.KeyOffSeconds = tempo.Seconds(n.KeyOffTick)
		n.SoundEndSeconds = tempo.Seconds(n.SoundEndTick)
		a.Notes = append(a.Notes, *n)
	}

	for _, ev := range f.MergedEvents() {
		switch ev.Kind {
		case EventNoteOn:
			if ev.Velocity > 0 {
				k := noteKey{ev.Track, ev.Channel, ev.Note}
				open[k] = append(open[k], &Note{
					Track: ev.Track, Channel: ev.Channel,
					Pitch: ev.Note, Velocity: ev.Velocity,
					KeyOnTick: ev.Tick,
				})
				continue
			}
			// Velocity-0 note-on acts as note-off.
			fallthrough
		case EventNoteOff:
			k := noteKey{ev.Track, ev.Channel, ev.Note}
			queue := open[k]
			if len(queue) == 0 {
				a.Anomalies = append(a.Anomalies, Anomaly{
					Type: AnomalyUnmatchedNoteOff, Track: ev.Track,
					Channel: ev.Channel, Pitch: ev.Note, Tick: ev.Tick,
					Message: fmt.Sprintf("note-off for pitch %d without matching note-on", ev.Note),
				})
				continue
			}
			n := queue[0]
			open[k] = queue[1:]
			n.KeyOffTick = ev.Tick
			if pedalDown[pedalKey{ev.Track, ev.Channel}] {
				sustained[k] = append(sustained[k], n)
			} else {
				n.SoundEndTick = ev.Tick
				finalize(n)
			}
		case EventControlChange:
			if ev.Controller != SustainPedalController {
				continue
			}
			pk := pedalKey{ev.Track, ev.Channel}
			if ev.Value >= 64 {
				pedalDown[pk] = true
				continue
			}
			pedalDown[pk] = false
			// Pedal released: end everything still sustained on this
			// track/channel.
			for k, list := range sustained {
				if k.track != ev.Track || k.channel != ev.Channel {
					continue
				}
				for _, n := range list {
					n.SoundEndTick = ev.Tick
					finalize(n)
				}
				delete(sustained, k)
			}
		}
	}

	// File end: cut anything still sounding at the maximum tick.
	for k, queue := range open {
		for _, n := range queue {
			n.KeyOffTick = f.MaxTick
			n.SoundEndTick = f.MaxTick
			n.Unclosed = true
			finalize(n)
			a.Anomalies = append(a.Anomalies, Anomaly{
				Type: AnomalyUnclosedNote, Track: k.track,
				Channel: k.channel, Pitch: k.pitch, Tick: f.MaxTick,
				Message: fmt.Sprintf("note-on for pitch %d never closed; cut at file end tick %d", k.pitch, f.MaxTick),
			})
		}
	}
	for _, list := range sustained {
		for _, n := range list {
			n.SoundEndTick = f.MaxTick
			n.PedalTruncated = true
			finalize(n)
		}
	}

	// Deterministic output order: onset tick, then track, channel, pitch.
	sort.SliceStable(a.Notes, func(i, j int) bool {
		x, y := a.Notes[i], a.Notes[j]
		if x.KeyOnTick != y.KeyOnTick {
			return x.KeyOnTick < y.KeyOnTick
		}
		if x.Track != y.Track {
			return x.Track < y.Track
		}
		if x.Channel != y.Channel {
			return x.Channel < y.Channel
		}
		return x.Pitch < y.Pitch
	})

	a.MaxPolyphony = maxPolyphony(a.Notes)
	return a
}

type sweepPoint struct {
	sec   float64
	delta int
}

// maxPolyphony computes, per track, the maximum number of notes sounding
// simultaneously over half-open [start, end) intervals in seconds.
func maxPolyphony(notes []Note) map[int]int {
	byTrack := map[int][]sweepPoint{}
	for _, n := range notes {
		if n.SoundEndSeconds <= n.KeyOnSeconds {
			continue // zero-duration notes do not count
		}
		byTrack[n.Track] = append(byTrack[n.Track],
			sweepPoint{n.KeyOnSeconds, +1},
			sweepPoint{n.SoundEndSeconds, -1},
		)
	}
	result := map[int]int{}
	for track, pts := range byTrack {
		// Sort by time; ends (-1) before starts (+1) at equal times so
		// half-open intervals do not overlap.
		sort.Slice(pts, func(i, j int) bool {
			if pts[i].sec != pts[j].sec {
				return pts[i].sec < pts[j].sec
			}
			return pts[i].delta < pts[j].delta
		})
		cur, max := 0, 0
		for _, p := range pts {
			cur += p.delta
			if cur > max {
				max = cur
			}
		}
		result[track] = max
	}
	return result
}
