package midi

import (
	"math"
	"testing"
)

func almostEq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func analyzeOne(t *testing.T, data []byte) *Analysis {
	t.Helper()
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return f.Analyze()
}

func TestTempoChangeIntegration(t *testing.T) {
	// Division 480. Default 500000 us/qn for the first quarter,
	// then 250000 us/qn from tick 480 on.
	data := newSMF(1, 480).
		track(tempo(0, 500000), endOfTrack(0)).
		track(
			noteOn(0, 0, 60, 100),
			tempo(480, 250000),
			noteOff(480, 0, 60, 0),
			endOfTrack(0),
		).
		build()
	a := analyzeOne(t, data)
	if len(a.Notes) != 1 {
		t.Fatalf("want 1 note, got %d", len(a.Notes))
	}
	n := a.Notes[0]
	// 480 ticks at 500000 us/qn + 480 ticks at 250000 us/qn = 0.75 s.
	if !almostEq(n.SoundEndSeconds, 0.75) {
		t.Fatalf("sound end = %v, want 0.75", n.SoundEndSeconds)
	}
	if !almostEq(n.KeyOffSeconds, 0.75) {
		t.Fatalf("key off = %v, want 0.75", n.KeyOffSeconds)
	}
}

func TestSameTickTempoLastWins(t *testing.T) {
	// Two tempo events at tick 0 in the same track: the later one
	// governs the following segment.
	data := newSMF(0, 480).track(
		tempo(0, 250000),
		tempo(0, 1000000),
		noteOn(0, 0, 60, 100),
		noteOff(480, 0, 60, 0),
		endOfTrack(0),
	).build()
	a := analyzeOne(t, data)
	if !almostEq(a.Notes[0].SoundEndSeconds, 1.0) {
		t.Fatalf("sound end = %v, want 1.0", a.Notes[0].SoundEndSeconds)
	}
}

func TestOverlappingSamePitchFIFO(t *testing.T) {
	// Two note-ons for the same key; offs close them FIFO.
	data := newSMF(0, 480).track(
		noteOn(0, 0, 60, 100),
		noteOn(240, 0, 60, 100),
		noteOff(240, 0, 60, 0), // tick 480: closes first note
		noteOff(240, 0, 60, 0), // tick 720: closes second note
		endOfTrack(0),
	).build()
	a := analyzeOne(t, data)
	if len(a.Notes) != 2 || len(a.Anomalies) != 0 {
		t.Fatalf("notes=%d anomalies=%d", len(a.Notes), len(a.Anomalies))
	}
	first, second := a.Notes[0], a.Notes[1]
	if first.KeyOnTick != 0 || first.KeyOffTick != 480 {
		t.Fatalf("first note %+v", first)
	}
	if second.KeyOnTick != 240 || second.KeyOffTick != 720 {
		t.Fatalf("second note %+v", second)
	}
	if a.MaxPolyphony[0] != 2 {
		t.Fatalf("max polyphony = %d, want 2", a.MaxPolyphony[0])
	}
}

func TestVelocityZeroNoteOnCloses(t *testing.T) {
	data := newSMF(0, 480).track(
		noteOn(0, 0, 60, 100),
		noteOn(480, 0, 60, 0),
		endOfTrack(0),
	).build()
	a := analyzeOne(t, data)
	if len(a.Notes) != 1 || a.Notes[0].KeyOffTick != 480 {
		t.Fatalf("notes %+v", a.Notes)
	}
	if len(a.Anomalies) != 0 {
		t.Fatalf("unexpected anomalies %+v", a.Anomalies)
	}
}

func TestUnmatchedNoteOffAnomaly(t *testing.T) {
	data := newSMF(0, 480).track(
		noteOff(0, 0, 60, 0),
		endOfTrack(0),
	).build()
	a := analyzeOne(t, data)
	if len(a.Notes) != 0 {
		t.Fatalf("notes %+v", a.Notes)
	}
	if len(a.Anomalies) != 1 || a.Anomalies[0].Type != AnomalyUnmatchedNoteOff {
		t.Fatalf("anomalies %+v", a.Anomalies)
	}
}

func TestUnclosedNoteCutAtFileEnd(t *testing.T) {
	data := newSMF(1, 480).
		track(noteOn(0, 0, 60, 100), endOfTrack(960)).
		track(noteOn(0, 1, 64, 100), noteOff(1440, 1, 64, 0), endOfTrack(0)).
		build()
	a := analyzeOne(t, data)
	if len(a.Notes) != 2 {
		t.Fatalf("notes %+v", a.Notes)
	}
	var unclosed *Note
	for i := range a.Notes {
		if a.Notes[i].Unclosed {
			unclosed = &a.Notes[i]
		}
	}
	if unclosed == nil {
		t.Fatal("expected one unclosed note")
	}
	// MaxTick is 1440 (the other track's note-off).
	if unclosed.SoundEndTick != 1440 || unclosed.KeyOffTick != 1440 {
		t.Fatalf("unclosed note cut at %d, want 1440", unclosed.SoundEndTick)
	}
	if len(a.Anomalies) != 1 || a.Anomalies[0].Type != AnomalyUnclosedNote {
		t.Fatalf("anomalies %+v", a.Anomalies)
	}
}

func TestSustainPedalExtendsAndReOnKeepsSustain(t *testing.T) {
	// Pedal down at 0; note off at 480 only lifts the key; pedal up at
	// 960 ends the sound. A re-on of the same pitch at 240 must not
	// swallow the sustained note.
	data := newSMF(0, 480).track(
		cc(0, 0, SustainPedalController, 127),
		noteOn(0, 0, 60, 100),
		noteOn(240, 0, 60, 100),               // tick 240: second on, same key
		noteOff(240, 0, 60, 0),                // tick 480: key off (FIFO first note)
		cc(480, 0, SustainPedalController, 0), // tick 960: pedal up
		noteOff(240, 0, 60, 0),                // tick 1200: second key off, no pedal
		endOfTrack(0),
	).build()
	a := analyzeOne(t, data)
	if len(a.Notes) != 2 || len(a.Anomalies) != 0 {
		t.Fatalf("notes=%d anomalies=%+v", len(a.Notes), a.Anomalies)
	}
	first, second := a.Notes[0], a.Notes[1]
	if first.KeyOffTick != 480 || first.SoundEndTick != 960 {
		t.Fatalf("sustained note %+v", first)
	}
	if second.KeyOffTick != 1200 || second.SoundEndTick != 1200 {
		t.Fatalf("re-on note %+v", second)
	}
}

func TestSustainPedalTruncatedAtFileEnd(t *testing.T) {
	data := newSMF(0, 480).track(
		cc(0, 0, SustainPedalController, 64), // boundary value: down
		noteOn(0, 0, 60, 100),
		noteOff(480, 0, 60, 0),
		endOfTrack(480), // file ends at tick 960 with pedal still down
	).build()
	a := analyzeOne(t, data)
	if len(a.Notes) != 1 {
		t.Fatalf("notes %+v", a.Notes)
	}
	n := a.Notes[0]
	if !n.PedalTruncated || n.SoundEndTick != 960 || n.KeyOffTick != 480 {
		t.Fatalf("pedal-truncated note %+v", n)
	}
	if len(a.Anomalies) != 0 {
		t.Fatalf("pedal truncation must not be an anomaly: %+v", a.Anomalies)
	}
}

func TestPedalIndependentPerTrackAndChannel(t *testing.T) {
	// Pedal on channel 0 must not sustain channel 1; pedal on track 0
	// must not sustain track 1.
	data := newSMF(1, 480).
		track(
			cc(0, 0, SustainPedalController, 127),
			noteOn(0, 0, 60, 100),
			noteOn(0, 1, 62, 100),
			noteOff(480, 0, 60, 0),
			noteOff(0, 1, 62, 0),
			endOfTrack(480),
		).
		track(
			noteOn(0, 0, 64, 100),
			noteOff(480, 0, 64, 0),
			endOfTrack(480),
		).
		build()
	a := analyzeOne(t, data)
	if len(a.Notes) != 3 {
		t.Fatalf("notes %+v", a.Notes)
	}
	for _, n := range a.Notes {
		sustained := n.Track == 0 && n.Channel == 0
		if sustained && n.SoundEndTick != 960 {
			t.Fatalf("ch0 note should sustain to pedal-up-less file end? %+v", n)
		}
		if !sustained && n.SoundEndTick != 480 {
			t.Fatalf("non-sustained note %+v", n)
		}
	}
}

func TestMaxPolyphonyHalfOpen(t *testing.T) {
	// Three overlapping notes on track 0; a back-to-back pair on track 1
	// must not overlap (half-open intervals).
	data := newSMF(1, 480).
		track(
			noteOn(0, 0, 60, 90),
			noteOn(0, 0, 64, 90),
			noteOn(240, 0, 67, 90),
			noteOff(240, 0, 60, 0),
			noteOff(0, 0, 64, 0),
			noteOff(0, 0, 67, 0),
			endOfTrack(0),
		).
		track(
			noteOn(0, 0, 60, 90),
			noteOff(480, 0, 60, 0),
			noteOn(0, 0, 62, 90),
			noteOff(480, 0, 62, 0),
			endOfTrack(0),
		).
		build()
	a := analyzeOne(t, data)
	if a.MaxPolyphony[0] != 3 {
		t.Fatalf("track 0 polyphony = %d, want 3", a.MaxPolyphony[0])
	}
	if a.MaxPolyphony[1] != 1 {
		t.Fatalf("track 1 polyphony = %d, want 1", a.MaxPolyphony[1])
	}
}

func TestZeroDurationNoteExcludedFromPolyphony(t *testing.T) {
	data := newSMF(0, 480).track(
		noteOn(0, 0, 60, 90),
		noteOff(0, 0, 60, 0),
		noteOn(0, 0, 64, 90),
		noteOff(480, 0, 64, 0),
		endOfTrack(0),
	).build()
	a := analyzeOne(t, data)
	if len(a.Notes) != 2 {
		t.Fatalf("notes %+v", a.Notes)
	}
	if a.MaxPolyphony[0] != 1 {
		t.Fatalf("polyphony = %d, want 1 (zero-duration ignored)", a.MaxPolyphony[0])
	}
}
