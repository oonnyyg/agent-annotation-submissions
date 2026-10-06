package midi

// tempoPoint 记录某个 tick 生效的新速度。
type tempoPoint struct {
	tick  uint64
	tempo uint32 // 微秒/四分音符
}

// Timeline 将绝对 tick 换算为秒，跨速度段积分。
type Timeline struct {
	division uint16
	points   []tempoPoint // 按 tick 升序，首项必为 tick 0
}

// NewTimeline 从全部事件的速度元事件构建时间轴。
// 同 tick 多个速度事件时最后一个为后续速度。
func NewTimeline(f *File) *Timeline {
	tl := &Timeline{division: f.Division}
	tl.points = append(tl.points, tempoPoint{tick: 0, tempo: DefaultTempo})
	for _, ev := range f.Events {
		if ev.Status != EvMeta || ev.Meta != MetaTempo {
			continue
		}
		last := &tl.points[len(tl.points)-1]
		if last.tick == ev.AbsTick {
			last.tempo = ev.Tempo // 同 tick 后者覆盖
		} else {
			tl.points = append(tl.points, tempoPoint{tick: ev.AbsTick, tempo: ev.Tempo})
		}
	}
	return tl
}

// Seconds 返回绝对 tick 对应的秒数（从 0 起积分）。
func (tl *Timeline) Seconds(tick uint64) float64 {
	var sec float64
	prev := tl.points[0]
	for i := 1; i < len(tl.points); i++ {
		p := tl.points[i]
		if p.tick >= tick {
			break
		}
		sec += float64(p.tick-prev.tick) * float64(prev.tempo) / 1e6 / float64(tl.division)
		prev = p
	}
	sec += float64(tick-prev.tick) * float64(prev.tempo) / 1e6 / float64(tl.division)
	return sec
}

// MaxTick 返回全文件最大事件 tick。
func (f *File) MaxTick() uint64 {
	var max uint64
	for _, ev := range f.Events {
		if ev.AbsTick > max {
			max = ev.AbsTick
		}
	}
	return max
}
