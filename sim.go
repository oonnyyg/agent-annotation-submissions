package main

import "fmt"

const (
	dirDown = -1
	dirIdle = 0
	dirUp   = 1
)

// Config holds the building and elevator parameters.
type Config struct {
	MinFloor   int `json:"minFloor"`
	MaxFloor   int `json:"maxFloor"`
	StartFloor int `json:"startFloor"`
	Capacity   int `json:"capacity"`
	// TravelTime is the cost of moving one floor.
	TravelTime int `json:"travelTimePerFloor"`
	// DwellTime is the cost of one stop (door open + close).
	DwellTime int `json:"dwellTime"`
}

// PassengerInput is one arrival record from the JSON input.
type PassengerInput struct {
	ID     string `json:"id"`
	Arrive int    `json:"arrive"`
	From   int    `json:"from"`
	To     int    `json:"to"`
}

// Input is the top-level JSON document.
type Input struct {
	Config
	Passengers []PassengerInput `json:"passengers"`
}

func validate(in *Input) error {
	c := &in.Config
	if c.MinFloor >= c.MaxFloor {
		return fmt.Errorf("invalid floor range [%d, %d]", c.MinFloor, c.MaxFloor)
	}
	if c.StartFloor < c.MinFloor || c.StartFloor > c.MaxFloor {
		return fmt.Errorf("startFloor %d out of range", c.StartFloor)
	}
	if c.Capacity <= 0 {
		return fmt.Errorf("capacity must be positive")
	}
	if c.TravelTime <= 0 {
		return fmt.Errorf("travelTimePerFloor must be positive")
	}
	if c.DwellTime <= 0 {
		return fmt.Errorf("dwellTime must be positive")
	}
	seen := map[string]bool{}
	for i, p := range in.Passengers {
		if p.ID == "" {
			return fmt.Errorf("passenger #%d: empty id", i)
		}
		if seen[p.ID] {
			return fmt.Errorf("passenger %q: duplicate id", p.ID)
		}
		seen[p.ID] = true
		if p.Arrive < 0 {
			return fmt.Errorf("passenger %q: negative arrive time", p.ID)
		}
		if p.From < c.MinFloor || p.From > c.MaxFloor || p.To < c.MinFloor || p.To > c.MaxFloor {
			return fmt.Errorf("passenger %q: floor out of range", p.ID)
		}
		if p.From == p.To {
			return fmt.Errorf("passenger %q: from equals to", p.ID)
		}
	}
	return nil
}

// Passenger is a simulated rider.
type Passenger struct {
	PassengerInput
	Index  int // input order, breaks ties for same arrival time
	Board  int // door-open instant when the passenger boards, -1 while waiting
	Alight int // arrival instant when the passenger leaves, -1 while riding
}

func (p *Passenger) dir() int {
	if p.To > p.From {
		return dirUp
	}
	return dirDown
}

func dirName(d int) string {
	switch d {
	case dirUp:
		return "up"
	case dirDown:
		return "down"
	}
	return "idle"
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
