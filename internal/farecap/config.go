package farecap

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// FareRule maps a coverage width (maxZone-minZone+1) to a fare in integer cents.
type FareRule struct {
	Width int `json:"width"`
	Fare  int `json:"fare"`
}

// Config is the JSON rate configuration.
type Config struct {
	// UTCOffsetSeconds is the fixed local UTC offset; no DST is applied.
	UTCOffsetSeconds int `json:"utc_offset_seconds"`
	// ServiceDayStartHour is the local hour (0-23) at which a service day starts.
	ServiceDayStartHour int `json:"service_day_start_hour"`
	// TransferWindowSeconds: a later ride joins a transfer group when its
	// boarding time minus the group's first boarding is strictly less than this.
	TransferWindowSeconds int64 `json:"transfer_window_seconds"`
	// Fares indexed by coverage width; must be monotonically non-decreasing.
	Fares []FareRule `json:"fares"`
	// DailyCap and WeeklyCap are non-negative caps in integer cents.
	DailyCap  int `json:"daily_cap"`
	WeeklyCap int `json:"weekly_cap"`
}

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) Validate() error {
	if c.UTCOffsetSeconds < -12*3600 || c.UTCOffsetSeconds > 14*3600 {
		return fmt.Errorf("utc_offset_seconds out of range: %d", c.UTCOffsetSeconds)
	}
	if c.ServiceDayStartHour < 0 || c.ServiceDayStartHour > 23 {
		return fmt.Errorf("service_day_start_hour must be in [0,23]: %d", c.ServiceDayStartHour)
	}
	if c.TransferWindowSeconds <= 0 {
		return fmt.Errorf("transfer_window_seconds must be positive: %d", c.TransferWindowSeconds)
	}
	if c.DailyCap < 0 || c.WeeklyCap < 0 {
		return fmt.Errorf("caps must be non-negative")
	}
	if c.WeeklyCap < c.DailyCap {
		return fmt.Errorf("weekly_cap (%d) must be >= daily_cap (%d)", c.WeeklyCap, c.DailyCap)
	}
	if len(c.Fares) == 0 {
		return fmt.Errorf("fares must not be empty")
	}
	widths := make(map[int]int, len(c.Fares))
	for i, f := range c.Fares {
		if f.Width <= 0 {
			return fmt.Errorf("fare[%d]: width must be a positive integer", i)
		}
		if f.Fare < 0 {
			return fmt.Errorf("fare[%d]: fare must be non-negative", i)
		}
		if prev, ok := widths[f.Width]; ok {
			return fmt.Errorf("fare[%d]: duplicate width %d (fare %d vs %d)", i, f.Width, prev, f.Fare)
		}
		widths[f.Width] = f.Fare
	}
	sorted := make([]FareRule, len(c.Fares))
	copy(sorted, c.Fares)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Width < sorted[j].Width })
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Fare < sorted[i-1].Fare {
			return fmt.Errorf("fares must be monotonically non-decreasing: width %d fare %d < width %d fare %d",
				sorted[i].Width, sorted[i].Fare, sorted[i-1].Width, sorted[i-1].Fare)
		}
	}
	return nil
}

// FareFor returns the fare for the given coverage width, or false when missing.
func (c *Config) FareFor(width int) (int, bool) {
	for _, f := range c.Fares {
		if f.Width == width {
			return f.Fare, true
		}
	}
	return 0, false
}
