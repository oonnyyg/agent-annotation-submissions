package main

import (
	"time"
)

// Connection 表示一次可行接续。
type Connection struct {
	From, To int // 班次下标（Problem.Trips 中）
	Dist     int64
	Travel   time.Duration
	Wait     time.Duration // 整备+空驶之后的空等时间
}

// feasibleConnections 枚举所有满足时刻条件的接续对 (i, j)。
// 条件：arrive_i + turnaround + travel(to_i, from_j) <= depart_j。
func feasibleConnections(p *Problem) []Connection {
	var conns []Connection
	n := len(p.Trips)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			a, b := p.Trips[i], p.Trips[j]
			travel, dist, ok := p.deadhead(a.To, b.From)
			if !ok {
				continue
			}
			ready := a.Arrive.Add(p.Turnaround).Add(travel)
			if ready.After(b.Depart) {
				continue
			}
			conns = append(conns, Connection{
				From:   i,
				To:     j,
				Dist:   dist,
				Travel: travel,
				Wait:   b.Depart.Sub(ready),
			})
		}
	}
	return conns
}

// 最小费用最大流（SPFA 最短路增广，n<=120 规模足够快且实现确定）。
// 二部匹配模型：0=源，1..n=左点（作为前班），n+1..2n=右点（作为后班），2n+1=汇。
// 边：S->L_i (cap1,cost0)，R_i->T (cap1,cost0)，
// L_i->R_j (cap1,cost=dist) 对每个可行接续 (i,j)。
// 最大流 = 最多接续数，最少车辆数 = n - 最大流；
// 在最大流（车辆最少）前提下费用最小 = 空驶里程最小。

type mcmfEdge struct {
	to, rev int
	cap     int
	cost    int64
	connIdx int // 仅接续边有效，否则 -1
}

type mcmf struct {
	g [][]mcmfEdge
}

func (f *mcmf) addEdge(from, to, cap int, cost int64, connIdx int) {
	f.g[from] = append(f.g[from], mcmfEdge{to: to, rev: len(f.g[to]), cap: cap, cost: cost, connIdx: connIdx})
	f.g[to] = append(f.g[to], mcmfEdge{to: from, rev: len(f.g[from]) - 1, cap: 0, cost: -cost, connIdx: -1})
}

// minCostMaxFlow 返回 (最大流, 最小费用) 及每条接续边上的流量。
func minCostMaxFlow(nNodes, s, t int, edges [][5]int64, connCount int) (int, int64, []int) {
	f := &mcmf{g: make([][]mcmfEdge, nNodes)}
	for _, e := range edges {
		f.addEdge(int(e[0]), int(e[1]), int(e[2]), e[3], int(e[4]))
	}
	flow := 0
	var cost int64
	connFlow := make([]int, connCount)
	for {
		const inf = int64(1) << 62
		dist := make([]int64, nNodes)
		inQ := make([]bool, nNodes)
		prevV := make([]int, nNodes)
		prevE := make([]int, nNodes)
		for i := range dist {
			dist[i] = inf
		}
		dist[s] = 0
		queue := []int{s}
		inQ[s] = true
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			inQ[v] = false
			for ei, e := range f.g[v] {
				if e.cap > 0 && dist[v]+e.cost < dist[e.to] {
					dist[e.to] = dist[v] + e.cost
					prevV[e.to] = v
					prevE[e.to] = ei
					if !inQ[e.to] {
						inQ[e.to] = true
						queue = append(queue, e.to)
					}
				}
			}
		}
		if dist[t] == inf {
			break
		}
		// 沿最短路增广 1 单位（所有容量为 1 的瓶颈）。
		for v := t; v != s; v = prevV[v] {
			ei := prevE[v]
			f.g[prevV[v]][ei].cap--
			rev := f.g[prevV[v]][ei].rev
			f.g[v][rev].cap++
		}
		flow++
		cost += dist[t]
	}
	// 汇总接续边流量（正向边容量从 1 变 0 即被选中）。
	for v := range f.g {
		for _, e := range f.g[v] {
			if e.connIdx >= 0 && e.cap == 0 {
				connFlow[e.connIdx] = 1
			}
		}
	}
	return flow, cost, connFlow
}

// Solution 是求解结果。
type Solution struct {
	Vehicles  int
	TotalDist int64
	Chains    [][]int               // 每条链为班次下标序列
	Conns     map[[2]int]Connection // (from,to) -> 接续信息
}

// solve 全局求解：先最少车辆，再最小空驶里程。
func solve(p *Problem) *Solution {
	n := len(p.Trips)
	conns := feasibleConnections(p)
	s, t := 0, 2*n+1
	var edges [][5]int64
	for i := 0; i < n; i++ {
		edges = append(edges, [5]int64{int64(s), int64(1 + i), 1, 0, -1})     // S->L_i
		edges = append(edges, [5]int64{int64(n + 1 + i), int64(t), 1, 0, -1}) // R_i->T
	}
	for ci, c := range conns {
		edges = append(edges, [5]int64{int64(1 + c.From), int64(n + 1 + c.To), 1, c.Dist, int64(ci)})
	}
	flow, cost, connFlow := minCostMaxFlow(2*n+2, s, t, edges, len(conns))

	// 重建链：next[i] = j 表示班次 i 之后接班次 j。
	next := make([]int, n)
	hasPrev := make([]bool, n)
	for i := range next {
		next[i] = -1
	}
	connMap := map[[2]int]Connection{}
	for ci, c := range conns {
		if connFlow[ci] == 1 {
			next[c.From] = c.To
			hasPrev[c.To] = true
			connMap[[2]int{c.From, c.To}] = c
		}
	}
	var chains [][]int
	for i := 0; i < n; i++ {
		if hasPrev[i] {
			continue
		}
		var chain []int
		for v := i; v != -1; v = next[v] {
			chain = append(chain, v)
		}
		chains = append(chains, chain)
	}
	return &Solution{
		Vehicles:  n - flow,
		TotalDist: cost,
		Chains:    chains,
		Conns:     connMap,
	}
}
