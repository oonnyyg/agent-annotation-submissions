package main

import "time"

// Trip 表示一个必须执行的固定班次。
type Trip struct {
	ID      string
	From    string
	To      string
	Start   time.Time // UTC
	End     time.Time // UTC
	Index   int       // 在输入中的稳定序号（按读入顺序）
}

// Duration 返回班次持续时间，输入校验保证其严格为正。
func (t Trip) Duration() time.Duration {
	return t.End.Sub(t.Start)
}

// DeadheadLeg 描述两站之间的有向空驶。
type DeadheadLeg struct {
	From     string
	To       string
	Duration time.Duration
	Distance int // 整数里程，>=0
}

// Input 是校验通过后的规划输入。
type Input struct {
	Trips         []Trip
	Deadhead      map[[2]string]DeadheadLeg
	MinPrep       time.Duration // 统一最短整备时长
}
