package farecap

import "time"

// Clock derives fixed-offset service periods.
// A service day starts at ServiceDayStartHour local time; the service week
// starts at the same local hour on Monday. There is no DST.
type Clock struct {
	loc       *time.Location
	startHour int
}

func NewClock(cfg *Config) *Clock {
	loc := time.FixedZone("local", cfg.UTCOffsetSeconds)
	return &Clock{loc: loc, startHour: cfg.ServiceDayStartHour}
}

// Local returns the instant rendered in the fixed-offset local zone.
func (c *Clock) Local(t time.Time) time.Time { return t.In(c.loc) }

// ServiceDayStart returns the UTC start instant of the service day containing t.
func (c *Clock) ServiceDayStart(t time.Time) time.Time {
	l := t.In(c.loc)
	start := time.Date(l.Year(), l.Month(), l.Day(), c.startHour, 0, 0, 0, c.loc)
	if l.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	return start.UTC()
}

// ServiceWeekStart returns the UTC start instant of the service week
// (Monday at startHour local) containing t.
func (c *Clock) ServiceWeekStart(t time.Time) time.Time {
	dayStart := c.ServiceDayStart(t).In(c.loc)
	daysSinceMonday := (int(dayStart.Weekday()) + 6) % 7 // Monday=0 ... Sunday=6
	return dayStart.AddDate(0, 0, -daysSinceMonday).UTC()
}

// DayKey identifies a service day by its start instant's local calendar date.
func (c *Clock) DayKey(t time.Time) string {
	s := c.ServiceDayStart(t).In(c.loc)
	return s.Format("2006-01-02")
}

// WeekKey identifies a service week by the local date of its Monday start.
func (c *Clock) WeekKey(t time.Time) string {
	s := c.ServiceWeekStart(t).In(c.loc)
	return s.Format("2006-01-02")
}
