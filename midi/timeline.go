package midi

import "sort"

// DefaultMicrosPerQuarter is the MIDI default tempo: 120 BPM.
const DefaultMicrosPerQuarter = 500000

// MergedEvents returns all events sorted stably by absolute tick, then
// source track index, then order within the track.
func (f *File) MergedEvents() []Event {
	events := make([]Event, len(f.Events))
	copy(events, f.Events)
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.Tick != b.Tick {
			return a.Tick < b.Tick
		}
		if a.Track != b.Track {
			return a.Track < b.Track
		}
		return a.Seq < b.Seq
	})
	return events
}

type tempoSegment struct {
	startTick        uint64
	startSeconds     float64
	microsPerQuarter uint32
}

// TempoMap converts absolute ticks to seconds using the tempo events of
// every track. At a tick carrying multiple tempo changes, the last one in
// merged order governs the following segment.
type TempoMap struct {
	division uint16
	segments []tempoSegment
}

// NewTempoMap builds a tempo map from the merged event stream.
func (f *File) NewTempoMap() *TempoMap {
	m := &TempoMap{division: f.Division}
	m.segments = append(m.segments, tempoSegment{
		startTick:        0,
		startSeconds:     0,
		microsPerQuarter: DefaultMicrosPerQuarter,
	})
	for _, ev := range f.MergedEvents() {
		if ev.Kind != EventTempo {
			continue
		}
		last := &m.segments[len(m.segments)-1]
		secs := last.startSeconds + m.spanSeconds(last, ev.Tick)
		if ev.Tick == last.startTick {
			// Same tick: the later event replaces the tempo for what follows.
			last.microsPerQuarter = ev.MicrosPerQuarter
			last.startSeconds = secs
			continue
		}
		m.segments = append(m.segments, tempoSegment{
			startTick:        ev.Tick,
			startSeconds:     secs,
			microsPerQuarter: ev.MicrosPerQuarter,
		})
	}
	return m
}

func (m *TempoMap) spanSeconds(seg *tempoSegment, toTick uint64) float64 {
	delta := toTick - seg.startTick
	return float64(delta) * float64(seg.microsPerQuarter) /
		(float64(m.division) * 1e6)
}

// Seconds converts an absolute tick to seconds from the file start.
func (m *TempoMap) Seconds(tick uint64) float64 {
	// Segments are few; a linear scan from the end is fine and simple.
	for i := len(m.segments) - 1; i >= 0; i-- {
		seg := &m.segments[i]
		if tick >= seg.startTick {
			return seg.startSeconds + m.spanSeconds(seg, tick)
		}
	}
	return 0
}
