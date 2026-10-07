package main

import "fmt"

// Sim 是单台电梯集选运行的离线回放器。
type Sim struct {
	cfg Config
	all []*Passenger // 按输入次序

	waiting []*Passenger // 已到达未上车，按 (到达时刻, 输入次序)
	onboard []*Passenger

	floor int
	dir   int // 0 空闲, +1 上行, -1 下行
	time  int

	pickup *Passenger // 空载直达接送目标，非空时途中不停靠

	nextArrivalIdx int // 下一条未登记的乘客记录（all 按到达时刻排序视图）
	arrivalOrder   []*Passenger

	arriveAt int  // 电梯到达下一楼层的时刻；moving 为 false 时无意义
	moving   bool // 电梯是否有在途计划

	logs        []string
	fullRejects int // 因满载未能上车的人次
	maxLoad     int
}

func dirName(d int) string {
	switch d {
	case 1:
		return "上行"
	case -1:
		return "下行"
	}
	return "空闲"
}

func (s *Sim) logf(format string, args ...any) {
	s.logs = append(s.logs, fmt.Sprintf("t=%d %s", s.time, fmt.Sprintf(format, args...)))
}

// forwardWork 判断沿方向 d 是否还有工作需要继续：车内目的地在前方，
// 或前方有等待起点，或当前层有同方向等待者。
func (s *Sim) forwardWork(d int) bool {
	for _, p := range s.onboard {
		if (d == 1 && p.To > s.floor) || (d == -1 && p.To < s.floor) {
			return true
		}
	}
	for _, p := range s.waiting {
		if p.From == s.floor && p.dir() == d {
			return true
		}
		if (d == 1 && p.From > s.floor) || (d == -1 && p.From < s.floor) {
			return true
		}
	}
	return false
}

// arriveFloor 电梯到达下一楼层。
func (s *Sim) arriveFloor() {
	s.floor += s.dir
	s.moving = false
	if s.pickup != nil {
		if s.floor != s.pickup.From {
			// 空载直达接送途中，不停靠
			s.moving = true
			s.arriveAt = s.time + s.cfg.TravelTime
			return
		}
		p := s.pickup
		s.pickup = nil
		s.dir = p.dir()
		s.logf("电梯到达 %d 层接乘客 %s，采用其方向转为%s", s.floor, p.ID, dirName(s.dir))
		s.serviceFloor()
		return
	}
	s.logf("电梯到达 %d 层（%s），载客 %d/%d", s.floor, dirName(s.dir), len(s.onboard), s.cfg.Capacity)
	s.serviceFloor()
}

// serviceFloor 在当前楼层处理下客、反向与上客。
func (s *Sim) serviceFloor() {
	alighted := false
	rest := s.onboard[:0]
	for _, p := range s.onboard {
		if p.To == s.floor {
			p.done = true
			p.Alight = s.time
			alighted = true
			s.logf("乘客 %s 在 %d 层下车", p.ID, s.floor)
		} else {
			rest = append(rest, p)
		}
	}
	s.onboard = rest

	// 方向决策：不在楼层之间改向，仅在停靠点反向
	if s.dir == 0 {
		if s.forwardWork(1) {
			s.dir = 1
		} else if s.forwardWork(-1) {
			s.dir = -1
		}
	} else if !s.forwardWork(s.dir) {
		if s.forwardWork(-s.dir) {
			s.dir = -s.dir
			s.logf("电梯在 %d 层反向，转为%s", s.floor, dirName(s.dir))
		} else {
			s.dir = 0
		}
	}

	boarded := false
	if s.dir != 0 {
		var still []*Passenger
		for _, p := range s.waiting {
			if p.From == s.floor && p.dir() == s.dir {
				if len(s.onboard) < s.cfg.Capacity {
					p.boarded = true
					p.Board = s.time
					s.onboard = append(s.onboard, p)
					boarded = true
					s.logf("乘客 %s 在 %d 层上车（载客 %d/%d）", p.ID, s.floor, len(s.onboard), s.cfg.Capacity)
				} else {
					s.fullRejects++
					s.logf("乘客 %s 因满载未能在 %d 层上车，继续等待", p.ID, s.floor)
					still = append(still, p)
				}
			} else {
				still = append(still, p)
			}
		}
		s.waiting = still
	}
	if len(s.onboard) > s.maxLoad {
		s.maxLoad = len(s.onboard)
	}

	if s.dir == 0 {
		s.logf("电梯在 %d 层进入空闲", s.floor)
		return
	}
	stop := alighted || boarded
	delay := s.cfg.TravelTime
	if stop {
		delay += s.cfg.StopTime
		s.logf("电梯在 %d 层停靠开门，耗时 %d", s.floor, s.cfg.StopTime)
	}
	s.moving = true
	s.arriveAt = s.time + delay
}

// NewSim 构造回放器；输入须已通过校验。
func NewSim(cfg Config, ps []*Passenger) *Sim {
	s := &Sim{cfg: cfg, all: ps, floor: cfg.InitialFloor}
	s.arrivalOrder = make([]*Passenger, len(ps))
	copy(s.arrivalOrder, ps)
	// 按 (到达时刻, 输入次序) 稳定排序
	for i := 1; i < len(s.arrivalOrder); i++ {
		for j := i; j > 0 && lessArrival(s.arrivalOrder[j], s.arrivalOrder[j-1]); j-- {
			s.arrivalOrder[j], s.arrivalOrder[j-1] = s.arrivalOrder[j-1], s.arrivalOrder[j]
		}
	}
	return s
}

func lessArrival(a, b *Passenger) bool {
	if a.Arrive != b.Arrive {
		return a.Arrive < b.Arrive
	}
	return a.seq < b.seq
}

// Run 推进回放直到全部乘客完成行程。
func (s *Sim) Run() {
	for {
		if s.done() {
			return
		}
		next := s.nextEventTime()
		s.time = next
		s.registerArrivals()
		if !s.moving && s.dir == 0 && len(s.waiting) > 0 {
			s.dispatch()
		}
		if s.moving && s.arriveAt == s.time {
			s.arriveFloor()
		}
	}
}

func (s *Sim) done() bool {
	if s.nextArrivalIdx < len(s.arrivalOrder) || len(s.waiting) > 0 || len(s.onboard) > 0 {
		return false
	}
	return !s.moving
}

// Summary 汇总一次回放的关键指标。
type Summary struct {
	FinishTime  int
	Passengers  int
	FullRejects int
	MaxLoad     int
	TotalWait   int
	MaxWait     int
}

func (s *Sim) Summary() Summary {
	sum := Summary{
		FinishTime:  s.time,
		Passengers:  len(s.all),
		FullRejects: s.fullRejects,
		MaxLoad:     s.maxLoad,
	}
	for _, p := range s.all {
		w := p.Wait()
		sum.TotalWait += w
		if w > sum.MaxWait {
			sum.MaxWait = w
		}
	}
	return sum
}

func (s *Sim) nextEventTime() int {
	best := -1
	if s.nextArrivalIdx < len(s.arrivalOrder) {
		best = s.arrivalOrder[s.nextArrivalIdx].Arrive
	}
	if s.moving && (best < 0 || s.arriveAt < best) {
		best = s.arriveAt
	}
	if best < 0 {
		// 仅等待者存在而电梯空闲：立即在当前时刻调度
		return s.time
	}
	if best < s.time {
		return s.time
	}
	return best
}

// registerArrivals 登记当前时刻到达的乘客（先于到站与开门处理）。
func (s *Sim) registerArrivals() {
	for s.nextArrivalIdx < len(s.arrivalOrder) && s.arrivalOrder[s.nextArrivalIdx].Arrive <= s.time {
		p := s.arrivalOrder[s.nextArrivalIdx]
		s.nextArrivalIdx++
		s.waiting = append(s.waiting, p)
		s.logf("乘客 %s 到达 %d 层，目的地 %d 层", p.ID, p.From, p.To)
	}
}

// dispatch 空闲时选择最早等待者，空载直达其起点。
func (s *Sim) dispatch() {
	p := s.waiting[0]
	s.pickup = p
	if p.From == s.floor {
		s.dir = p.dir()
		s.pickup = nil
		s.logf("电梯空闲于 %d 层，响应最早等待者 %s，转为%s", s.floor, p.ID, dirName(s.dir))
		s.serviceFloor()
		return
	}
	if p.From > s.floor {
		s.dir = 1
	} else {
		s.dir = -1
	}
	s.logf("电梯空闲于 %d 层，空载前往 %d 层接乘客 %s", s.floor, p.From, p.ID)
	s.moving = true
	s.arriveAt = s.time + s.cfg.TravelTime
}
