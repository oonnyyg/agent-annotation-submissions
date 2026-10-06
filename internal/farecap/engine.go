package farecap

import (
	"fmt"
	"sort"
	"time"
)

// timeLayout 是乘车记录使用的本地时间格式（固定 UTC 偏移，无夏令时）。
const timeLayout = "2006-01-02T15:04:05"

// Ride 表示一条乘车记录。
type Ride struct {
	ID     string `json:"id"`
	Card   string `json:"card"`
	Board  string `json:"board"`
	Alight string `json:"alight"`
	From   int    `json:"from"`
	To     int    `json:"to"`
}

// VoidRecord 表示一条撤销指令及其结果。
type VoidRecord struct {
	ID     string `json:"id"`
	Card   string `json:"card"`
	Target string `json:"target"`
}

// Charge 是一次实际扣费的流水。
type Charge struct {
	RideID      string `json:"rideId"`
	AmountCents int    `json:"amountCents"`
	ServiceDay  string `json:"serviceDay"`
	ServiceWeek string `json:"serviceWeek"`
}

// CardState 保存单卡的记录与重算结果。
type CardState struct {
	Rides   map[string]*RideEntry `json:"rides"`
	Voids   []VoidRecord          `json:"voids"`
	Charges []Charge              `json:"charges"`
}

// RideEntry 记录一条乘车及其撤销标记。
type RideEntry struct {
	Ride   Ride `json:"ride"`
	Voided bool `json:"voided"`
}

// State 是可持久化的整体状态。
type State struct {
	Cards map[string]*CardState `json:"cards"`
}

// NewState 创建空状态。
func NewState() *State {
	return &State{Cards: map[string]*CardState{}}
}

// Engine 组合配置与状态。
type Engine struct {
	cfg *Config
	loc *time.Location
	st  *State
}

// NewEngine 创建引擎。
func NewEngine(cfg *Config, st *State) *Engine {
	if st == nil {
		st = NewState()
	}
	loc := time.FixedZone("cfg", cfg.UTCOffsetMinutes*60)
	return &Engine{cfg: cfg, loc: loc, st: st}
}

// State 返回内部状态（用于持久化）。
func (e *Engine) State() *State { return e.st }

func (e *Engine) parseTime(s string) (time.Time, error) {
	t, err := time.ParseInLocation(timeLayout, s, e.loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("非法时间 %q（期望 %s）", s, timeLayout)
	}
	return t, nil
}

// serviceDayKey 返回 t 所属服务日（本地 04:00 起算）的日期串。
func (e *Engine) serviceDayKey(t time.Time) string {
	return t.Add(-4 * time.Hour).In(e.loc).Format("2006-01-02")
}

// serviceWeekKey 返回 t 所属服务周（周一 04:00 起算）的周一日期串。
func (e *Engine) serviceWeekKey(t time.Time) string {
	a := t.Add(-4 * time.Hour).In(e.loc)
	midnight := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, e.loc)
	back := (int(midnight.Weekday()) + 6) % 7 // 周一为 0
	return midnight.AddDate(0, 0, -back).Format("2006-01-02")
}

// OpResult 是单条输入指令的处理结果。
type OpResult struct {
	Status      string `json:"status"` // ok / duplicate / rejected
	Reason      string `json:"reason,omitempty"`
	Charged     int    `json:"chargedCents,omitempty"`
	CardCharged int    `json:"cardTotalCents,omitempty"`
}

func (e *Engine) card(cardID string) *CardState {
	cs, ok := e.st.Cards[cardID]
	if !ok {
		cs = &CardState{Rides: map[string]*RideEntry{}}
		e.st.Cards[cardID] = cs
	}
	return cs
}

// AddRide 导入一条乘车记录；同编号同内容幂等，冲突拒绝。
func (e *Engine) AddRide(r Ride) OpResult {
	if r.ID == "" || r.Card == "" {
		return OpResult{Status: "rejected", Reason: "缺少记录编号或卡号"}
	}
	if r.From <= 0 || r.To <= 0 {
		return OpResult{Status: "rejected", Reason: "区号必须为正整数"}
	}
	board, err := e.parseTime(r.Board)
	if err != nil {
		return OpResult{Status: "rejected", Reason: err.Error()}
	}
	alight, err := e.parseTime(r.Alight)
	if err != nil {
		return OpResult{Status: "rejected", Reason: err.Error()}
	}
	if alight.Before(board) {
		return OpResult{Status: "rejected", Reason: "下车时间早于上车时间"}
	}
	cs := e.card(r.Card)
	if old, ok := cs.Rides[r.ID]; ok {
		if old.Ride == r {
			return OpResult{Status: "duplicate", CardCharged: totalCharged(cs)}
		}
		return OpResult{Status: "rejected", Reason: "记录编号冲突，内容不一致"}
	}
	// 同卡有效行程重叠检查。
	for _, ent := range cs.Rides {
		if ent.Voided {
			continue
		}
		ob, _ := e.parseTime(ent.Ride.Board)
		oa, _ := e.parseTime(ent.Ride.Alight)
		if board.Before(oa) && ob.Before(alight) {
			return OpResult{Status: "rejected", Reason: fmt.Sprintf("与记录 %s 行程重叠", ent.Ride.ID)}
		}
	}
	cs.Rides[r.ID] = &RideEntry{Ride: r}
	if err := e.recompute(cs); err != nil {
		delete(cs.Rides, r.ID)
		_ = e.recompute(cs)
		return OpResult{Status: "rejected", Reason: err.Error()}
	}
	charged := 0
	for _, ch := range cs.Charges {
		if ch.RideID == r.ID {
			charged = ch.AmountCents
		}
	}
	return OpResult{Status: "ok", Charged: charged, CardCharged: totalCharged(cs)}
}

// AddVoid 撤销一条乘车记录并完整重算该卡扣费。
func (e *Engine) AddVoid(v VoidRecord) OpResult {
	if v.ID == "" || v.Card == "" || v.Target == "" {
		return OpResult{Status: "rejected", Reason: "撤销指令缺少字段"}
	}
	cs := e.card(v.Card)
	for _, old := range cs.Voids {
		if old.ID == v.ID {
			if old == v {
				return OpResult{Status: "duplicate", CardCharged: totalCharged(cs)}
			}
			return OpResult{Status: "rejected", Reason: "撤销编号冲突，内容不一致"}
		}
	}
	ent, ok := cs.Rides[v.Target]
	if !ok {
		return OpResult{Status: "rejected", Reason: "目标记录不存在"}
	}
	if ent.Voided {
		return OpResult{Status: "rejected", Reason: "目标记录已撤销"}
	}
	ent.Voided = true
	cs.Voids = append(cs.Voids, v)
	if err := e.recompute(cs); err != nil {
		ent.Voided = false
		cs.Voids = cs.Voids[:len(cs.Voids)-1]
		_ = e.recompute(cs)
		return OpResult{Status: "rejected", Reason: err.Error()}
	}
	return OpResult{Status: "ok", CardCharged: totalCharged(cs)}
}

func totalCharged(cs *CardState) int {
	sum := 0
	for _, ch := range cs.Charges {
		sum += ch.AmountCents
	}
	return sum
}

type parsedRide struct {
	ride          Ride
	board, alight time.Time
}

// recompute 按该卡有效记录完整重算扣费流水。
func (e *Engine) recompute(cs *CardState) error {
	var rides []parsedRide
	for _, ent := range cs.Rides {
		if ent.Voided {
			continue
		}
		b, err := e.parseTime(ent.Ride.Board)
		if err != nil {
			return err
		}
		a, err := e.parseTime(ent.Ride.Alight)
		if err != nil {
			return err
		}
		rides = append(rides, parsedRide{ent.Ride, b, a})
	}
	sort.Slice(rides, func(i, j int) bool {
		if !rides[i].board.Equal(rides[j].board) {
			return rides[i].board.Before(rides[j].board)
		}
		return rides[i].ride.ID < rides[j].ride.ID
	})

	window := time.Duration(e.cfg.TransferWindowMinutes) * time.Minute
	var charges []Charge
	dayTotals := map[string]int{}
	weekTotals := map[string]int{}

	var groupDay string
	var groupStart time.Time
	var groupMin, groupMax, groupFare int
	inGroup := false

	for _, pr := range rides {
		day := e.serviceDayKey(pr.board)
		week := e.serviceWeekKey(pr.board)
		lo, hi := pr.ride.From, pr.ride.To
		if lo > hi {
			lo, hi = hi, lo
		}
		// 跨服务日必开新组；窗口自组首趟上车起算，不随换乘延长。
		if !inGroup || day != groupDay || !pr.board.Before(groupStart.Add(window)) {
			inGroup = true
			groupDay = day
			groupStart = pr.board
			groupMin, groupMax = lo, hi
			groupFare = 0
		} else {
			if lo < groupMin {
				groupMin = lo
			}
			if hi > groupMax {
				groupMax = hi
			}
		}
		fare, err := e.cfg.FareFor(groupMax - groupMin + 1)
		if err != nil {
			return err
		}
		delta := fare - groupFare
		groupFare = fare
		// 受当前日与周剩余额度共同限制。
		remaining := e.cfg.DailyCapCents - dayTotals[day]
		if w := e.cfg.WeeklyCapCents - weekTotals[week]; w < remaining {
			remaining = w
		}
		charge := delta
		if charge > remaining {
			charge = remaining
		}
		if charge < 0 {
			charge = 0
		}
		dayTotals[day] += charge
		weekTotals[week] += charge
		charges = append(charges, Charge{
			RideID:      pr.ride.ID,
			AmountCents: charge,
			ServiceDay:  day,
			ServiceWeek: week,
		})
	}
	cs.Charges = charges
	return nil
}

// Query 返回指定卡的扣费流水与总额。
func (e *Engine) Query(cardID string) ([]Charge, int) {
	cs, ok := e.st.Cards[cardID]
	if !ok {
		return nil, 0
	}
	return cs.Charges, totalCharged(cs)
}
