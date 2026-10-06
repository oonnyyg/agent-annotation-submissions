package payroll

type Tier struct {
	UpTo      int `json:"up_to"`
	UnitPrice int `json:"unit_price"`
}

type SchemeVersion struct {
	SchemeID string `json:"scheme_id"`
	Version  int    `json:"version"`
	Tiers    []Tier `json:"tiers"`
}

type MemberWeight struct {
	EmpID  string `json:"emp_id"`
	Weight int    `json:"weight"`
}

type Entry struct {
	Kind          string `json:"kind"`
	BatchID       string `json:"batch_id"`
	EmpID         string `json:"emp_id"`
	Amount        int    `json:"amount"`
	Month         string `json:"month"`
	SchemeID      string `json:"scheme_id"`
	SchemeVersion int    `json:"scheme_version"`
	Quantity      int    `json:"quantity"`
}

type Batch struct {
	BatchID       string         `json:"batch_id"`
	SchemeID      string         `json:"scheme_id"`
	SchemeVersion int            `json:"scheme_version"`
	Month         string         `json:"month"`
	Quantity      int            `json:"quantity"`
	Members       []MemberWeight `json:"members"`
	Total         int            `json:"total"`
	Fingerprint   string         `json:"fingerprint"`
	Voided        bool           `json:"voided"`
	VoidMonth     string         `json:"void_month,omitempty"`
}

type State struct {
	Schemes      map[string][]SchemeVersion `json:"schemes"`
	Batches      map[string]*Batch          `json:"batches"`
	Entries      []Entry                    `json:"entries"`
	ClosedMonths map[string]bool            `json:"closed_months"`
}

func NewState() *State {
	return &State{
		Schemes:      map[string][]SchemeVersion{},
		Batches:      map[string]*Batch{},
		Entries:      []Entry{},
		ClosedMonths: map[string]bool{},
	}
}
