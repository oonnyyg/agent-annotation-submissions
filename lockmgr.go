package main

import (
	"fmt"
	"sort"
)

// LockMode is either shared (S) or exclusive (X).
type LockMode string

const (
	Shared    LockMode = "S"
	Exclusive LockMode = "X"
)

// TxState describes the lifecycle of a transaction.
type TxState string

const (
	TxActive   TxState = "active"
	TxWaiting  TxState = "waiting"
	TxFinished TxState = "finished"
)

type lockRequest struct {
	Tx   int      `json:"tx"`
	Mode LockMode `json:"mode"`
}

type resourceLock struct {
	holders map[int]LockMode
	queue   []lockRequest
}

// Grant describes a lock granted to a transaction, with the reason.
type Grant struct {
	Tx       int    `json:"tx"`
	Resource string `json:"resource"`
	Mode     string `json:"mode"`
	Reason   string `json:"reason"`
}

// LockManager is a simplified, single-node lock manager used for
// failure-drill simulations. It is not a real database component.
type LockManager struct {
	resources map[string]*resourceLock
	txState   map[int]TxState
	txStart   map[int]int
	nextTx    int
	seq       int
}

func NewLockManager() *LockManager {
	return &LockManager{
		resources: make(map[string]*resourceLock),
		txState:   make(map[int]TxState),
		txStart:   make(map[int]int),
	}
}

// Begin allocates the next transaction id. Ids are never reused.
func (m *LockManager) Begin() int {
	m.seq++
	m.nextTx++
	m.txState[m.nextTx] = TxActive
	m.txStart[m.nextTx] = m.seq
	return m.nextTx
}

func (m *LockManager) resource(name string) *resourceLock {
	r, ok := m.resources[name]
	if !ok {
		r = &resourceLock{holders: make(map[int]LockMode)}
		m.resources[name] = r
	}
	return r
}

// compatible reports whether mode can coexist with locks held by others.
func (r *resourceLock) compatible(tx int, mode LockMode) bool {
	for h, hm := range r.holders {
		if h == tx {
			continue
		}
		if mode == Exclusive || hm == Exclusive {
			return false
		}
	}
	return true
}

// RequestLock attempts to acquire a lock. It returns the grant if the lock
// is granted immediately, or nil if the transaction is now waiting.
func (m *LockManager) RequestLock(tx int, resource string, mode LockMode) (*Grant, error) {
	st, ok := m.txState[tx]
	if !ok || st == TxFinished {
		return nil, fmt.Errorf("transaction %d does not exist or has finished", tx)
	}
	if st == TxWaiting {
		return nil, fmt.Errorf("transaction %d is waiting; only status or rollback allowed", tx)
	}
	if mode != Shared && mode != Exclusive {
		return nil, fmt.Errorf("invalid lock mode %q", mode)
	}
	r := m.resource(resource)
	held, has := r.holders[tx]
	if has {
		if held == Exclusive || held == mode {
			return &Grant{Tx: tx, Resource: resource, Mode: string(mode), Reason: "already-held"}, nil
		}
		// Upgrade S -> X: keep the S lock, wait without counting self as blocker.
		if len(r.queue) == 0 && r.compatible(tx, Exclusive) {
			r.holders[tx] = Exclusive
			return &Grant{Tx: tx, Resource: resource, Mode: string(Exclusive), Reason: "upgrade-immediate"}, nil
		}
		r.queue = append(r.queue, lockRequest{Tx: tx, Mode: Exclusive})
		m.txState[tx] = TxWaiting
		return nil, nil
	}
	if len(r.queue) == 0 && r.compatible(tx, mode) {
		r.holders[tx] = mode
		return &Grant{Tx: tx, Resource: resource, Mode: string(mode), Reason: "granted-immediately"}, nil
	}
	r.queue = append(r.queue, lockRequest{Tx: tx, Mode: mode})
	m.txState[tx] = TxWaiting
	return nil, nil
}

// releaseAll drops every lock and queued request of tx, then wakes queues.
func (m *LockManager) releaseAll(tx int) []Grant {
	var granted []Grant
	names := make([]string, 0, len(m.resources))
	for name := range m.resources {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		r := m.resources[name]
		delete(r.holders, tx)
		q := r.queue[:0]
		for _, req := range r.queue {
			if req.Tx != tx {
				q = append(q, req)
			}
		}
		r.queue = q
		granted = append(granted, m.wake(name, r)...)
	}
	return granted
}

// wake grants queued requests from the head while possible. Consecutive
// shared requests may be granted together; the scan stops at the first
// request that cannot be granted (no queue jumping).
func (m *LockManager) wake(name string, r *resourceLock) []Grant {
	var granted []Grant
	for len(r.queue) > 0 {
		req := r.queue[0]
		if !r.compatible(req.Tx, req.Mode) {
			break
		}
		r.queue = r.queue[1:]
		r.holders[req.Tx] = req.Mode
		m.txState[req.Tx] = TxActive
		granted = append(granted, Grant{Tx: req.Tx, Resource: name, Mode: string(req.Mode), Reason: "granted-after-wait"})
	}
	return granted
}

// Finish commits or rolls back a transaction, releasing all of its locks.
func (m *LockManager) Finish(tx int) ([]Grant, error) {
	st, ok := m.txState[tx]
	if !ok || st == TxFinished {
		return nil, fmt.Errorf("transaction %d does not exist or has finished", tx)
	}
	m.txState[tx] = TxFinished
	delete(m.txStart, tx)
	return m.releaseAll(tx), nil
}

// WaitEdge is a wait-for edge: From waits for To.
type WaitEdge struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// WaitForEdges builds the wait-for graph: edges to conflicting holders and
// to transactions queued ahead. No edge points to the waiter itself.
func (m *LockManager) WaitForEdges() []WaitEdge {
	edgeSet := make(map[WaitEdge]bool)
	for _, r := range m.resources {
		for i, req := range r.queue {
			for h, hm := range r.holders {
				if h != req.Tx && (req.Mode == Exclusive || hm == Exclusive) {
					edgeSet[WaitEdge{From: req.Tx, To: h}] = true
				}
			}
			for j := 0; j < i; j++ {
				if r.queue[j].Tx != req.Tx {
					edgeSet[WaitEdge{From: req.Tx, To: r.queue[j].Tx}] = true
				}
			}
		}
	}
	edges := make([]WaitEdge, 0, len(edgeSet))
	for e := range edgeSet {
		edges = append(edges, e)
	}
	sort.Slice(edges, func(a, b int) bool {
		if edges[a].From != edges[b].From {
			return edges[a].From < edges[b].From
		}
		return edges[a].To < edges[b].To
	})
	return edges
}

// DeadlockReport captures the wait-for edges before resolution, the cycle
// members, the chosen victim, and locks granted after the abort.
type DeadlockReport struct {
	EdgesBefore []WaitEdge `json:"edges_before"`
	Cycle       []int      `json:"cycle"`
	Victim      int        `json:"victim"`
	Granted     []Grant    `json:"granted"`
}

// ResolveDeadlocks repeatedly finds cycles, aborts the latest-started
// transaction in each cycle, and wakes queues until no cycle remains.
func (m *LockManager) ResolveDeadlocks() []DeadlockReport {
	var reports []DeadlockReport
	for {
		cycle := m.findCycleTxs()
		if len(cycle) == 0 {
			return reports
		}
		edges := m.WaitForEdges()
		victim := cycle[0]
		for _, tx := range cycle[1:] {
			if m.txStart[tx] > m.txStart[victim] {
				victim = tx
			}
		}
		m.txState[victim] = TxFinished
		delete(m.txStart, victim)
		granted := m.releaseAll(victim)
		reports = append(reports, DeadlockReport{
			EdgesBefore: edges,
			Cycle:       cycle,
			Victim:      victim,
			Granted:     granted,
		})
	}
}

// findCycleTxs returns all transactions that participate in any cycle of
// the wait-for graph, or nil if the graph is acyclic. Nodes are peeled
// iteratively while they have in-degree or out-degree zero; the survivors
// are exactly the nodes lying on cycles.
func (m *LockManager) findCycleTxs() []int {
	adj := make(map[int][]int)
	nodes := make(map[int]bool)
	for _, e := range m.WaitForEdges() {
		adj[e.From] = append(adj[e.From], e.To)
		nodes[e.From] = true
		nodes[e.To] = true
	}
	indeg := make(map[int]int)
	outdeg := make(map[int]int)
	for n := range nodes {
		indeg[n] = 0
		outdeg[n] = 0
	}
	for f, tos := range adj {
		outdeg[f] = len(tos)
		for _, t := range tos {
			indeg[t]++
		}
	}
	alive := make(map[int]bool)
	for n := range nodes {
		alive[n] = true
	}
	changed := true
	for changed {
		changed = false
		for n := range nodes {
			if !alive[n] {
				continue
			}
			if indeg[n] == 0 || outdeg[n] == 0 {
				alive[n] = false
				changed = true
				for _, t := range adj[n] {
					indeg[t]--
				}
				for f, tos := range adj {
					if !alive[f] {
						continue
					}
					for _, t := range tos {
						if t == n {
							outdeg[f]--
						}
					}
				}
			}
		}
	}
	var cyc []int
	for n, a := range alive {
		if a {
			cyc = append(cyc, n)
		}
	}
	sort.Ints(cyc)
	return cyc
}

// Snapshot is a JSON-serializable view of the manager state. Finished
// transactions appear neither as holders nor as waiters.
type Snapshot struct {
	Transactions map[string]TxState             `json:"transactions"`
	Holders      map[string]map[string]LockMode `json:"holders"`
	Queues       map[string][]lockRequest       `json:"queues"`
	WaitFor      []WaitEdge                     `json:"wait_for"`
}

// Snapshot returns the current state with deterministic, JSON-friendly keys.
func (m *LockManager) Snapshot() Snapshot {
	s := Snapshot{
		Transactions: make(map[string]TxState),
		Holders:      make(map[string]map[string]LockMode),
		Queues:       make(map[string][]lockRequest),
		WaitFor:      m.WaitForEdges(),
	}
	for tx, st := range m.txState {
		if st != TxFinished {
			s.Transactions[fmt.Sprintf("%d", tx)] = st
		}
	}
	for name, r := range m.resources {
		if len(r.holders) > 0 {
			h := make(map[string]LockMode, len(r.holders))
			for tx, mode := range r.holders {
				h[fmt.Sprintf("%d", tx)] = mode
			}
			s.Holders[name] = h
		}
		if len(r.queue) > 0 {
			q := make([]lockRequest, len(r.queue))
			copy(q, r.queue)
			s.Queues[name] = q
		}
	}
	return s
}
