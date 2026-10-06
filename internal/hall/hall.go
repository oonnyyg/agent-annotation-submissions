// Package hall implements a small-cinema contiguous seat allocator.
package hall

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Seat is a 1-based row/column coordinate.
type Seat struct {
	Row int `json:"row"`
	Col int `json:"col"`
}

// Order is a single booking with its preferred coordinate.
type Order struct {
	ID    string `json:"id"`
	Size  int    `json:"size"`
	Pref  Seat   `json:"pref"`
	Seats []Seat `json:"seats"`
	Cost  int    `json:"cost"`
}

// Hall is the persistent state: layout plus live orders.
type Hall struct {
	Rows     int               `json:"rows"`
	Cols     int               `json:"cols"`
	Disabled map[string]bool   `json:"disabled"`
	Orders   map[string]*Order `json:"orders"`
}

// NewHall creates a hall with the given dimensions (max 20 seats total).
func NewHall(rows, cols int) (*Hall, error) {
	if rows < 1 || cols < 1 || rows*cols > 20 {
		return nil, fmt.Errorf("invalid hall size %dx%d: need positive and <=20 seats", rows, cols)
	}
	return &Hall{
		Rows:     rows,
		Cols:     cols,
		Disabled: map[string]bool{},
		Orders:   map[string]*Order{},
	}, nil
}

func key(s Seat) string { return fmt.Sprintf("%d,%d", s.Row, s.Col) }

func (h *Hall) valid(s Seat) bool {
	return s.Row >= 1 && s.Row <= h.Rows && s.Col >= 1 && s.Col <= h.Cols
}

// SetDisabled marks seats out of service (or restores them).
func (h *Hall) SetDisabled(seats []Seat, disabled bool) error {
	for _, s := range seats {
		if !h.valid(s) {
			return fmt.Errorf("seat %v out of range", s)
		}
		if disabled && h.seatOwner(s) != "" {
			return fmt.Errorf("seat %v is occupied", s)
		}
	}
	for _, s := range seats {
		if disabled {
			h.Disabled[key(s)] = true
		} else {
			delete(h.Disabled, key(s))
		}
	}
	return nil
}

func (h *Hall) seatOwner(s Seat) string {
	for _, o := range h.Orders {
		for _, os := range o.Seats {
			if os == s {
				return o.ID
			}
		}
	}
	return ""
}

// occupied returns seats taken by orders other than excludeID.
func (h *Hall) occupied(excludeID string) map[string]bool {
	occ := map[string]bool{}
	for id, o := range h.Orders {
		if id == excludeID {
			continue
		}
		for _, s := range o.Seats {
			occ[key(s)] = true
		}
	}
	return occ
}

func seatCost(s, pref Seat) int {
	d := s.Row - pref.Row
	if d < 0 {
		d = -d
	}
	c := s.Col - pref.Col
	if c < 0 {
		c = -c
	}
	return d + c
}

// candidates lists every contiguous same-row run of exactly size free seats.
// Runs never cross gaps or disabled seats.
func (h *Hall) candidates(size int, pref Seat, occ map[string]bool) [][]Seat {
	var out [][]Seat
	for r := 1; r <= h.Rows; r++ {
		c := 1
		for c <= h.Cols {
			if h.Disabled[key(Seat{r, c})] || occ[key(Seat{r, c})] {
				c++
				continue
			}
			start := c
			for c <= h.Cols && !h.Disabled[key(Seat{r, c})] && !occ[key(Seat{r, c})] {
				c++
			}
			runLen := c - start
			for i := 0; i+size <= runLen; i++ {
				run := make([]Seat, size)
				for j := range run {
					run[j] = Seat{r, start + i + j}
				}
				out = append(out, run)
			}
		}
	}
	return out
}

func runCost(run []Seat, pref Seat) int {
	total := 0
	for _, s := range run {
		total += seatCost(s, pref)
	}
	return total
}

// Save writes the hall state as JSON into dir/state.json atomically.
func (h *Hall) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "state.json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "state.json"))
}

// Load reads hall state from dir, or returns nil if none exists yet.
func Load(dir string) (*Hall, error) {
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var h Hall
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, err
	}
	if h.Disabled == nil {
		h.Disabled = map[string]bool{}
	}
	if h.Orders == nil {
		h.Orders = map[string]*Order{}
	}
	return &h, nil
}

// SortedOrders returns orders sorted by ID.
func (h *Hall) SortedOrders() []*Order {
	ids := make([]string, 0, len(h.Orders))
	for id := range h.Orders {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Order, 0, len(ids))
	for _, id := range ids {
		out = append(out, h.Orders[id])
	}
	return out
}
