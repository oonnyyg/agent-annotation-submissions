package main

import "fmt"

// Config 描述电梯与楼层的静态参数。
type Config struct {
	MinFloor     int `json:"minFloor"`
	MaxFloor     int `json:"maxFloor"`
	InitialFloor int `json:"initialFloor"`
	Capacity     int `json:"capacity"`
	TravelTime   int `json:"travelTimePerFloor"`
	StopTime     int `json:"stopTime"`
}

func (c Config) validate() error {
	if c.MinFloor >= c.MaxFloor {
		return fmt.Errorf("config: require minFloor < maxFloor, got %d >= %d", c.MinFloor, c.MaxFloor)
	}
	if c.InitialFloor < c.MinFloor || c.InitialFloor > c.MaxFloor {
		return fmt.Errorf("config: initialFloor %d out of range [%d,%d]", c.InitialFloor, c.MinFloor, c.MaxFloor)
	}
	if c.Capacity <= 0 {
		return fmt.Errorf("config: capacity must be positive, got %d", c.Capacity)
	}
	if c.TravelTime <= 0 {
		return fmt.Errorf("config: travelTimePerFloor must be positive, got %d", c.TravelTime)
	}
	if c.StopTime <= 0 {
		return fmt.Errorf("config: stopTime must be positive, got %d", c.StopTime)
	}
	return nil
}

// Passenger 是一条乘客到达记录及其回放结果。
type Passenger struct {
	ID     string `json:"id"`
	Arrive int    `json:"arriveTime"`
	From   int    `json:"from"`
	To     int    `json:"to"`

	seq     int  // 输入次序，用于同刻排序
	Board   int  // 上车时刻
	Alight  int  // 下车时刻
	boarded bool // 是否已上车
	done    bool // 是否已完成行程
}

// Wait 返回乘客从到达到上车的等待时间。
func (p *Passenger) Wait() int { return p.Board - p.Arrive }

func (p *Passenger) dir() int {
	if p.To > p.From {
		return 1
	}
	return -1
}

func validatePassengers(cfg Config, ps []*Passenger) error {
	seen := map[string]bool{}
	for i, p := range ps {
		if p.ID == "" {
			return fmt.Errorf("passenger #%d: id must not be empty", i)
		}
		if seen[p.ID] {
			return fmt.Errorf("passenger %q: duplicate id", p.ID)
		}
		seen[p.ID] = true
		if p.Arrive < 0 {
			return fmt.Errorf("passenger %q: arriveTime must be >= 0, got %d", p.ID, p.Arrive)
		}
		if p.From < cfg.MinFloor || p.From > cfg.MaxFloor {
			return fmt.Errorf("passenger %q: from floor %d out of range", p.ID, p.From)
		}
		if p.To < cfg.MinFloor || p.To > cfg.MaxFloor {
			return fmt.Errorf("passenger %q: to floor %d out of range", p.ID, p.To)
		}
		if p.From == p.To {
			return fmt.Errorf("passenger %q: from and to must differ (floor %d)", p.ID, p.From)
		}
		p.seq = i
	}
	return nil
}
