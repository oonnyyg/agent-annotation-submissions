package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
)

// Input is the top-level JSON document: one chain plus joint samples.
type Input struct {
	Chain   Chain    `json:"chain"`
	Samples []Sample `json:"samples"`
}

// FrameReport is one joint or tool frame in the world frame.
type FrameReport struct {
	Label string `json:"label"`
	Pose  Mat4   `json:"pose"`
}

// SampleReport is the per-sample result in the JSON detail output.
type SampleReport struct {
	Name          string        `json:"name"`
	OK            bool          `json:"ok"`
	Error         string        `json:"error,omitempty"`
	Frames        []FrameReport `json:"frames,omitempty"`
	ToolPose      *Mat4         `json:"tool_pose,omitempty"`
	Jacobian      *Jacobian     `json:"jacobian,omitempty"`
	LinearVel     *[3]float64   `json:"linear_velocity,omitempty"`
	AngularVel    *[3]float64   `json:"angular_velocity,omitempty"`
	JacobianCheck []ColumnError `json:"jacobian_check,omitempty"`
}

// Output is the top-level JSON detail document.
type Output struct {
	Chain   string         `json:"chain"`
	Samples []SampleReport `json:"samples"`
}

func processSample(c *Chain, s Sample) SampleReport {
	rep := SampleReport{Name: s.Name}
	frames, err := c.ForwardKinematics(s.Positions)
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	for i, j := range c.Joints {
		label := j.Name
		if label == "" {
			label = fmt.Sprintf("joint_%d", i)
		}
		rep.Frames = append(rep.Frames, FrameReport{Label: label, Pose: frames[i]})
	}
	tool := frames[len(frames)-1]
	rep.ToolPose = &tool
	rep.Frames = append(rep.Frames, FrameReport{Label: "tool", Pose: tool})

	jac, err := c.GeometricJacobian(s.Positions)
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	rep.Jacobian = &jac

	lin, ang, err := c.ToolVelocity(s.Positions, s.Velocities)
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	rep.LinearVel = &lin
	rep.AngularVel = &ang

	check, err := c.CheckJacobian(s.Positions)
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	rep.JacobianCheck = check
	rep.OK = true
	return rep
}

func writeCSV(w io.Writer, c *Chain, reports []SampleReport) error {
	cw := csv.NewWriter(w)
	header := []string{"sample", "ok", "error",
		"tool_x", "tool_y", "tool_z",
		"vx", "vy", "vz", "wx", "wy", "wz",
		"jac_check_max_abs_error", "jac_check_within_tolerance"}
	if err := cw.Write(header); err != nil {
		return err
	}
	f := func(v float64) string { return strconv.FormatFloat(v, 'g', 9, 64) }
	for _, r := range reports {
		row := []string{r.Name, strconv.FormatBool(r.OK), r.Error}
		if r.OK {
			p := r.ToolPose.Trans()
			row = append(row, f(p[0]), f(p[1]), f(p[2]),
				f(r.LinearVel[0]), f(r.LinearVel[1]), f(r.LinearVel[2]),
				f(r.AngularVel[0]), f(r.AngularVel[1]), f(r.AngularVel[2]))
			maxErr := 0.0
			within := true
			for _, ce := range r.JacobianCheck {
				if ce.MaxAbs > maxErr {
					maxErr = ce.MaxAbs
				}
				within = within && ce.Within
			}
			row = append(row, f(maxErr), strconv.FormatBool(within))
		} else {
			row = append(row, "", "", "", "", "", "", "", "", "", "", "")
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func run() error {
	inPath := flag.String("input", "", "path to input JSON (chain + samples), or - for stdin")
	jsonPath := flag.String("json", "", "path to JSON detail output, or - for stdout")
	csvPath := flag.String("csv", "", "path to CSV summary output, or - for stdout")
	flag.Parse()
	if *inPath == "" {
		return fmt.Errorf("-input is required")
	}
	if *jsonPath == "" && *csvPath == "" {
		return fmt.Errorf("at least one of -json or -csv is required")
	}

	var in io.Reader = os.Stdin
	if *inPath != "-" {
		f, err := os.Open(*inPath)
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}
	var input Input
	if err := json.NewDecoder(in).Decode(&input); err != nil {
		return fmt.Errorf("decoding input: %w", err)
	}
	if err := input.Chain.Validate(); err != nil {
		return fmt.Errorf("invalid chain: %w", err)
	}

	out := Output{Chain: input.Chain.Name}
	for _, s := range input.Samples {
		out.Samples = append(out.Samples, processSample(&input.Chain, s))
	}

	if *jsonPath != "" {
		var w io.Writer = os.Stdout
		if *jsonPath != "-" {
			f, err := os.Create(*jsonPath)
			if err != nil {
				return err
			}
			defer f.Close()
			w = f
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return err
		}
	}
	if *csvPath != "" {
		var w io.Writer = os.Stdout
		if *csvPath != "-" {
			f, err := os.Create(*csvPath)
			if err != nil {
				return err
			}
			defer f.Close()
			w = f
		}
		if err := writeCSV(w, &input.Chain, out.Samples); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
