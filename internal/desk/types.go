package desk

// MoveEvent 是一次入库或出库。
type MoveEvent struct {
	Loc string `json:"loc"`
	SKU string `json:"sku"`
	Qty int    `json:"qty"`
}

// Take 的生命周期状态。
const (
	TakeOpen   = "open"
	TakeSealed = "sealed"
	TakePosted = "posted"
)

// CountRecord 是一条实盘清点记录，(库位, 编号) 唯一。
type CountRecord struct {
	RecID string `json:"rec_id"`
	Loc   string `json:"loc"`
	SKU   string `json:"sku"`
	Qty   int    `json:"qty"`
	Void  bool   `json:"void,omitempty"`
}

// DiffLine 是封存时冻结的一行差异明细（实盘 - 基线）。
type DiffLine struct {
	SKU      string `json:"sku"` // 形如 "loc/sku"
	Baseline int    `json:"baseline"`
	Counted  int    `json:"counted"`
	Diff     int    `json:"diff"`
}

// Take 是一次盘点。
type Take struct {
	ID        string                    `json:"id"`
	Locations []string                  `json:"locations"`
	Status    string                    `json:"status"`
	Baseline  map[string]map[string]int `json:"baseline"` // loc -> sku -> 冻结时点账面数量
	Records   map[string]*CountRecord   `json:"records"`  // "loc/recID" -> 记录
	Completed map[string]bool           `json:"completed"`
	Diffs     []DiffLine                `json:"diffs,omitempty"`
	PostSeq   int                       `json:"post_seq,omitempty"` // 入账事件序号，追溯依据
}

func (t *Take) hasLoc(loc string) bool {
	for _, l := range t.Locations {
		if l == loc {
			return true
		}
	}
	return false
}

// State 是重放事件得到的全部内存状态。
type State struct {
	Stock map[string]map[string]int `json:"stock"` // loc -> sku -> 当前库存
	Takes map[string]*Take          `json:"takes"`
}

func NewState() *State {
	return &State{Stock: map[string]map[string]int{}, Takes: map[string]*Take{}}
}

// StockRow 是库存查询的一行输出。
type StockRow struct {
	Loc string `json:"loc"`
	SKU string `json:"sku"`
	Qty int    `json:"qty"`
}
