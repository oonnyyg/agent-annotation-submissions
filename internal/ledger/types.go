package ledger

// Date is an ISO calendar date (YYYY-MM-DD). String comparison is valid
// because the format is fixed-width and zero-padded.
type Date = string

// Batch is a grant of leave minutes valid over [Start, End).
type Batch struct {
	ID       string `json:"id"`
	Employee string `json:"employee"`
	Start    Date   `json:"start"`
	End      Date   `json:"end"`
	Minutes  int    `json:"minutes"`
	// Used is minutes currently frozen by approved requests.
	Used int `json:"used"`
	// ExpiredReturned is minutes returned by cancellations after the
	// batch had already expired; never usable again.
	ExpiredReturned int `json:"expiredReturned"`
}

// Remaining is the minutes not currently consumed by a request.
func (b *Batch) Remaining() int {
	return b.Minutes - b.Used - b.ExpiredReturned
}

// Effective reports whether the batch can be consumed on date d,
// using a left-closed, right-open validity range [Start, End).
func (b *Batch) Effective(d Date) bool {
	return b.Start <= d && d < b.End
}

// Allocation records how many minutes one request took from one batch.
type Allocation struct {
	BatchID string `json:"batchId"`
	Minutes int    `json:"minutes"`
}

// Request is a leave application. Its allocation plan is frozen at
// approval time and is the only source for cancellation refunds.
type Request struct {
	ID       string       `json:"id"`
	Employee string       `json:"employee"`
	Date     Date         `json:"date"`
	Minutes  int          `json:"minutes"`
	Allocs   []Allocation `json:"allocs"`
	Canceled bool         `json:"canceled"`
}

// State is the whole persisted ledger.
type State struct {
	LastDate Date                `json:"lastDate"`
	Batches  map[string]*Batch   `json:"batches"`
	Requests map[string]*Request `json:"requests"`
}

func NewState() *State {
	return &State{
		Batches:  map[string]*Batch{},
		Requests: map[string]*Request{},
	}
}
