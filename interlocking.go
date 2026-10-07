package main

import "fmt"

type routeState struct {
	route   *Route
	train   string
	nextIdx int
}

type Interlocking struct {
	cfg           *Config
	occupied      map[string]string
	sectionLocks  map[string]string
	switchPos     map[string]Position
	switchHolders map[string]map[string]bool
	routes        map[string]*routeState
	trainRoute    map[string]string
}

func NewInterlocking(cfg *Config) *Interlocking {
	il := &Interlocking{
		cfg:           cfg,
		occupied:      map[string]string{},
		sectionLocks:  map[string]string{},
		switchPos:     map[string]Position{},
		switchHolders: map[string]map[string]bool{},
		routes:        map[string]*routeState{},
		trainRoute:    map[string]string{},
	}
	for w, p := range cfg.Switches {
		il.switchPos[w] = p
	}
	return il
}

func (il *Interlocking) findRoute(id string) (*Route, error) {
	for i := range il.cfg.Routes {
		if il.cfg.Routes[i].ID == id {
			return &il.cfg.Routes[i], nil
		}
	}
	return nil, fmt.Errorf("进路 %q 不存在", id)
}

func (il *Interlocking) RequestRoute(id string) error {
	rt, err := il.findRoute(id)
	if err != nil {
		return err
	}
	if _, ok := il.routes[id]; ok {
		return fmt.Errorf("进路 %q 已锁闭", id)
	}
	for _, s := range rt.Sections {
		if holder, locked := il.sectionLocks[s]; locked {
			return fmt.Errorf("区段 %q 已被进路 %q 锁闭", s, holder)
		}
	}
	for w, want := range rt.Switches {
		if holders := il.switchHolders[w]; len(holders) > 0 && il.switchPos[w] != want {
			return fmt.Errorf("道岔 %q 已被持有在 %s，与要求的 %s 冲突", w, il.switchPos[w], want)
		}
	}
	for _, s := range rt.Sections {
		il.sectionLocks[s] = id
	}
	for w, want := range rt.Switches {
		il.switchPos[w] = want
		if il.switchHolders[w] == nil {
			il.switchHolders[w] = map[string]bool{}
		}
		il.switchHolders[w][id] = true
	}
	il.routes[id] = &routeState{route: rt, nextIdx: 0}
	return nil
}

func (il *Interlocking) release(id string) {
	rs := il.routes[id]
	for _, s := range rs.route.Sections {
		delete(il.sectionLocks, s)
	}
	for w := range rs.route.Switches {
		delete(il.switchHolders[w], id)
		if len(il.switchHolders[w]) == 0 {
			delete(il.switchHolders, w)
		}
	}
	delete(il.routes, id)
}

func (il *Interlocking) CancelRoute(id string) error {
	rs, ok := il.routes[id]
	if !ok {
		return fmt.Errorf("进路 %q 未锁闭，无法取消", id)
	}
	if rs.train != "" {
		return fmt.Errorf("列车 %q 已进入进路 %q，行车中禁止取消", rs.train, id)
	}
	il.release(id)
	return nil
}

func (il *Interlocking) Enter(train, routeID string) error {
	rs, ok := il.routes[routeID]
	if !ok {
		return fmt.Errorf("进路 %q 未锁闭，列车 %q 不能进入", routeID, train)
	}
	if rs.train != "" {
		return fmt.Errorf("进路 %q 已有列车 %q", routeID, rs.train)
	}
	if other, busy := il.trainRoute[train]; busy {
		return fmt.Errorf("列车 %q 已在进路 %q 上，不能同时进入进路 %q", train, other, routeID)
	}
	rs.train = train
	rs.nextIdx = 0
	il.trainRoute[train] = routeID
	il.occupied[rs.route.Sections[0]] = train
	return nil
}

func (il *Interlocking) Advance(train string) error {
	routeID, ok := il.trainRoute[train]
	if !ok {
		return fmt.Errorf("列车 %q 不在任何进路上，无法推进", train)
	}
	rs := il.routes[routeID]
	cur := rs.route.Sections[rs.nextIdx]
	delete(il.occupied, cur)
	rs.nextIdx++
	if rs.nextIdx >= len(rs.route.Sections) {
		delete(il.trainRoute, train)
		il.release(routeID)
		return nil
	}
	il.occupied[rs.route.Sections[rs.nextIdx]] = train
	return nil
}

func (il *Interlocking) SetSwitch(sw string, pos Position) error {
	if _, ok := il.cfg.Switches[sw]; !ok {
		return fmt.Errorf("道岔 %q 不存在", sw)
	}
	if !pos.valid() {
		return fmt.Errorf("道岔位置非法: %q", pos)
	}
	if holders := il.switchHolders[sw]; len(holders) > 0 && il.switchPos[sw] != pos {
		return fmt.Errorf("道岔 %q 仍被进路持有在 %s，不能手动转到 %s", sw, il.switchPos[sw], pos)
	}
	il.switchPos[sw] = pos
	return nil
}

type SectionSnap struct {
	Section  string `json:"section"`
	Occupied string `json:"occupied_by,omitempty"`
	LockedBy string `json:"locked_by,omitempty"`
}

type SwitchSnap struct {
	Switch   string   `json:"switch"`
	Position Position `json:"position"`
	Holders  []string `json:"holders,omitempty"`
}

type Snapshot struct {
	Sections []SectionSnap `json:"sections"`
	Switches []SwitchSnap  `json:"switches"`
}

func (il *Interlocking) Snapshot() Snapshot {
	snap := Snapshot{}
	for _, s := range il.cfg.Sections {
		snap.Sections = append(snap.Sections, SectionSnap{
			Section:  s,
			Occupied: il.occupied[s],
			LockedBy: il.sectionLocks[s],
		})
	}
	for _, w := range sortedKeys(il.switchPos) {
		ss := SwitchSnap{Switch: w, Position: il.switchPos[w]}
		for h := range il.switchHolders[w] {
			ss.Holders = append(ss.Holders, h)
		}
		sortStrings(ss.Holders)
		snap.Switches = append(snap.Switches, ss)
	}
	return snap
}
