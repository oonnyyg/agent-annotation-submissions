package farecap

import (
	"fmt"
	"sort"
	"time"
)

// RideCharge is the recomputed outcome for one effective ride.
type RideCharge struct {
	RideID  string `json:"ride_id"`
	GroupID int    `json:"group_id"`
	DayKey  string `json:"service_day"`
	WeekKey string `json:"service_week"`
	Charge  int    `json:"charge_cents"`
}

// DayTotal aggregates a service day for one card.
type DayTotal struct {
	Key         string       `json:"day"`
	WeekKey     string       `json:"week"`
	Total       int          `json:"total_cents"`
	RideCharges []RideCharge `json:"rides"`
}

// WeekTotal aggregates a service week for one card.
type WeekTotal struct {
	Key   string `json:"week"`
	Total int    `json:"total_cents"`
}

// CardResult is the fully recomputed billing result for one card.
type CardResult struct {
	Card    string       `json:"card"`
	Days    []DayTotal   `json:"days"`
	Weeks   []WeekTotal  `json:"weeks"`
	Rides   []RideCharge `json:"rides"`
	Charged int          `json:"charged_cents"`
}

type transferGroup struct {
	id      int
	first   time.Time
	minZone int
	maxZone int
	dayKey  string
	weekKey string
}

func (g *transferGroup) width() int { return g.maxZone - g.minZone + 1 }

// Recompute fully recomputes billing for a card's effective rides,
// ordered by boarding time then record ID.
func Recompute(card string, rides []Ride, cfg *Config, clock *Clock) (CardResult, error) {
	ordered := make([]Ride, len(rides))
	copy(ordered, rides)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Board.Equal(ordered[j].Board) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Board.Before(ordered[j].Board)
	})

	result := CardResult{Card: card}
	dayMap := map[string]*DayTotal{}
	weekMap := map[string]*WeekTotal{}
	dayOrder, weekOrder := []string{}, []string{}

	var cur *transferGroup
	nextGroupID := 1
	for _, r := range ordered {
		dayKey, weekKey := clock.DayKey(r.Board), clock.WeekKey(r.Board)
		if cur == nil ||
			cur.dayKey != dayKey ||
			r.Board.Sub(cur.first) >= time.Duration(cfg.TransferWindowSeconds)*time.Second {
			cur = &transferGroup{
				id:      nextGroupID,
				first:   r.Board,
				minZone: min(r.From, r.To),
				maxZone: max(r.From, r.To),
				dayKey:  dayKey,
				weekKey: weekKey,
			}
			nextGroupID++
		} else {
			cur.minZone = min(cur.minZone, r.From, r.To)
			cur.maxZone = max(cur.maxZone, r.From, r.To)
		}

		newFare, ok := cfg.FareFor(cur.width())
		if !ok {
			return CardResult{}, fmt.Errorf("card %s ride %s: no fare configured for coverage width %d", card, r.ID, cur.width())
		}

		day := dayMap[dayKey]
		if day == nil {
			day = &DayTotal{Key: dayKey, WeekKey: weekKey}
			dayMap[dayKey] = day
			dayOrder = append(dayOrder, dayKey)
		}
		week := weekMap[weekKey]
		if week == nil {
			week = &WeekTotal{Key: weekKey}
			weekMap[weekKey] = week
			weekOrder = append(weekOrder, weekKey)
		}

		prevFare := sumGroupCharges(day.RideCharges, cur.id)
		delta := newFare - prevFare
		if delta < 0 {
			return CardResult{}, fmt.Errorf("internal error: non-monotonic group fare (prev %d new %d)", prevFare, newFare)
		}
		dayRemaining := cfg.DailyCap - day.Total
		weekRemaining := cfg.WeeklyCap - week.Total
		charge := minInt(delta, dayRemaining, weekRemaining)
		if charge < 0 {
			charge = 0
		}

		day.Total += charge
		week.Total += charge
		day.RideCharges = append(day.RideCharges, RideCharge{
			RideID:  r.ID,
			GroupID: cur.id,
			DayKey:  dayKey,
			WeekKey: weekKey,
			Charge:  charge,
		})
	}

	for _, k := range dayOrder {
		d := dayMap[k]
		result.Days = append(result.Days, *d)
		for _, rc := range d.RideCharges {
			result.Rides = append(result.Rides, rc)
		}
		result.Charged += d.Total
	}
	for _, k := range weekOrder {
		result.Weeks = append(result.Weeks, *weekMap[k])
	}
	return result, nil
}

func sumGroupCharges(charges []RideCharge, groupID int) int {
	total := 0
	for _, rc := range charges {
		if rc.GroupID == groupID {
			total += rc.Charge
		}
	}
	return total
}

func minInt(vals ...int) int {
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

// Overlaps reports whether two rides on the same card overlap.
// Intervals are half-open: touching at board/alight instants is allowed.
func Overlaps(a, b Ride) bool {
	return a.Board.Before(b.Alight) && b.Board.Before(a.Alight)
}
