// Command midinotes analyzes a Standard MIDI File offline and writes a
// CSV note table plus a JSON anomaly summary. It never plays or
// synthesizes audio.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"

	"example.com/midi-note-timeline/midi"
)

func main() {
	csvPath := flag.String("csv", "", "output CSV file for the note table (default <input>.notes.csv)")
	jsonPath := flag.String("json", "", "output JSON file for anomalies (default <input>.anomalies.json)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: midinotes [-csv out.csv] [-json out.json] <file.mid>")
		os.Exit(2)
	}
	input := flag.Arg(0)
	if *csvPath == "" {
		*csvPath = input + ".notes.csv"
	}
	if *jsonPath == "" {
		*jsonPath = input + ".anomalies.json"
	}

	data, err := os.ReadFile(input)
	if err != nil {
		fatal(err)
	}
	file, err := midi.Parse(data)
	if err != nil {
		fatal(err)
	}
	analysis := file.Analyze()

	if err := writeCSV(*csvPath, analysis); err != nil {
		fatal(err)
	}
	if err := writeJSON(*jsonPath, file, analysis); err != nil {
		fatal(err)
	}
	fmt.Printf("analyzed %s: %d notes, %d anomalies\n", input, len(analysis.Notes), len(analysis.Anomalies))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "midinotes:", err)
	os.Exit(1)
}

func writeCSV(path string, a *midi.Analysis) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{
		"track", "channel", "pitch", "velocity",
		"key_on_tick", "key_off_tick", "sound_end_tick",
		"key_on_sec", "key_off_sec", "sound_end_sec",
		"unclosed", "pedal_truncated",
	}); err != nil {
		return err
	}
	for _, n := range a.Notes {
		rec := []string{
			strconv.Itoa(n.Track),
			strconv.Itoa(int(n.Channel)),
			strconv.Itoa(int(n.Pitch)),
			strconv.Itoa(int(n.Velocity)),
			strconv.FormatUint(n.KeyOnTick, 10),
			strconv.FormatUint(n.KeyOffTick, 10),
			strconv.FormatUint(n.SoundEndTick, 10),
			strconv.FormatFloat(n.KeyOnSeconds, 'f', 9, 64),
			strconv.FormatFloat(n.KeyOffSeconds, 'f', 9, 64),
			strconv.FormatFloat(n.SoundEndSeconds, 'f', 9, 64),
			strconv.FormatBool(n.Unclosed),
			strconv.FormatBool(n.PedalTruncated),
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	return w.Error()
}

type jsonReport struct {
	Format       uint16         `json:"format"`
	Division     uint16         `json:"ticks_per_quarter"`
	MaxTick      uint64         `json:"max_tick"`
	NoteCount    int            `json:"note_count"`
	MaxPolyphony map[string]int `json:"max_polyphony_per_track"`
	Anomalies    []midi.Anomaly `json:"anomalies"`
}

func writeJSON(path string, f *midi.File, a *midi.Analysis) error {
	poly := map[string]int{}
	for track, v := range a.MaxPolyphony {
		poly[strconv.Itoa(track)] = v
	}
	anomalies := a.Anomalies
	if anomalies == nil {
		anomalies = []midi.Anomaly{}
	}
	report := jsonReport{
		Format:       f.Format,
		Division:     f.Division,
		MaxTick:      f.MaxTick,
		NoteCount:    len(a.Notes),
		MaxPolyphony: poly,
		Anomalies:    anomalies,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
