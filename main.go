package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
)

// SampleResult is the per-sample JSON detail. Failed samples carry only the
// error message; no pose fields are emitted for them.
type SampleResult struct {
	Name        string        `json:"name"`
	OK          bool          `json:"ok"`
	Error       string        `json:"error,omitempty"`
	JointFrames []PoseJSON    `json:"jointFrames,omitempty"`
	Tool        *PoseJSON     `json:"toolPose,omitempty"`
	Jacobian    [][]float64   `json:"jacobian,omitempty"`
	Twist       *Twist        `json:"toolTwist,omitempty"`
	Checks      []ColumnCheck `json:"jacobianCheck,omitempty"`
}

type Report struct {
	Samples []SampleResult `json:"samples"`
}

func processSample(c *Chain, s Sample) SampleResult {
	res := SampleResult{Name: s.Name}
	fail := func(err error) SampleResult {
		res.Error = err.Error()
		return res
	}
	if len(s.Qd) != len(c.Joints) {
		return fail(fmt.Errorf("expected %d joint velocities, got %d", len(c.Joints), len(s.Qd)))
	}
	frames, tool, err := c.ForwardKinematics(s.Q)
	if err != nil {
		return fail(err)
	}
	jac, _, err := c.GeometricJacobian(s.Q)
	if err != nil {
		return fail(err)
	}
	checks, err := c.NumericalPositionJacobian(s.Q, jac)
	if err != nil {
		return fail(err)
	}
	res.OK = true
	res.JointFrames = make([]PoseJSON, len(frames))
	for i, f := range frames {
		res.JointFrames[i] = PoseJSON{Rotation: f.R, Translation: f.P}
	}
	res.Tool = &PoseJSON{Rotation: tool.R, Translation: tool.P}
	res.Jacobian = jac.Cols
	twist := jac.Apply(s.Qd)
	res.Twist = &twist
	res.Checks = checks
	return res
}

func writeCSV(path string, results []SampleResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	header := []string{
		"sample", "status", "error",
		"pos_x", "pos_y", "pos_z",
		"lin_vx", "lin_vy", "lin_vz",
		"ang_wx", "ang_wy", "ang_wz",
		"max_jacobian_error",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range results {
		row := make([]string, len(header))
		row[0] = r.Name
		if !r.OK {
			row[1] = "rejected"
			row[2] = r.Error
		} else {
			row[1] = "ok"
			vals := []float64{
				r.Tool.Translation[0], r.Tool.Translation[1], r.Tool.Translation[2],
				r.Twist.Linear[0], r.Twist.Linear[1], r.Twist.Linear[2],
				r.Twist.Angular[0], r.Twist.Angular[1], r.Twist.Angular[2],
			}
			for i, v := range vals {
				row[3+i] = strconv.FormatFloat(v, 'g', 10, 64)
			}
			maxErr := 0.0
			for _, chk := range r.Checks {
				if chk.MaxError > maxErr {
					maxErr = chk.MaxError
				}
			}
			row[12] = strconv.FormatFloat(maxErr, 'g', 5, 64)
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

func run(chainPath, samplesPath, jsonOut, csvOut string) error {
	chainData, err := os.ReadFile(chainPath)
	if err != nil {
		return err
	}
	chain, err := decodeChain(chainData)
	if err != nil {
		return err
	}
	sampleData, err := os.ReadFile(samplesPath)
	if err != nil {
		return err
	}
	var sf SamplesFile
	if err := json.Unmarshal(sampleData, &sf); err != nil {
		return fmt.Errorf("parsing samples JSON: %w", err)
	}
	report := Report{Samples: make([]SampleResult, len(sf.Samples))}
	for i, s := range sf.Samples {
		report.Samples[i] = processSample(chain, s)
	}
	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonOut, out, 0o644); err != nil {
		return err
	}
	return writeCSV(csvOut, report.Samples)
}

func main() {
	chainPath := flag.String("chain", "chain.json", "robot chain JSON file")
	samplesPath := flag.String("samples", "samples.json", "joint samples JSON file")
	jsonOut := flag.String("json-out", "report.json", "JSON detail output file")
	csvOut := flag.String("csv-out", "summary.csv", "CSV summary output file")
	flag.Parse()
	if err := run(*chainPath, *samplesPath, *jsonOut, *csvOut); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
