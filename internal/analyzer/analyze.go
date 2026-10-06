package analyzer

// Region 报告一个等价单元区域及其命中的规则。
type Region struct {
	Desc  string         // 区域边界描述
	Input map[string]any // 区域内最小输入（可直接复现）
	Rules []string       // 命中该区域的规则 id（按声明顺序）
}

type Report struct {
	Complete   bool // 单元数超限时不完整
	TotalCells int
	CellLimit  int
	Uncovered  []Region // 无任何规则命中
	Overlaps   []Region // 多条规则命中（unique 策略下即冲突）
	Shadowed   []ShadowedRule
	Witnesses  []Witness // first 策略下仍可生效规则的见证
}

type ShadowedRule struct {
	RuleID   string   `json:"rule_id"`
	Coverers []string `json:"coverers"` // 联合遮蔽它的前面规则
}

type Witness struct {
	RuleID string         `json:"rule_id"`
	Input  map[string]any `json:"input"`
}

// Analyze 按规则边界划分等价单元做精确静态分析。
func Analyze(s *Spec) *Report {
	rep := &Report{CellLimit: s.CellLimit}
	total, limit, ok := TotalCells(s)
	rep.TotalCells = total
	rep.CellLimit = limit
	if !ok {
		rep.Complete = false
		return rep
	}
	rep.Complete = true

	cells := EnumCells(s)
	// first 策略：每条规则是否有“最早命中”的单元
	winner := make([]bool, len(s.Rules))
	witnessDone := make([]bool, len(s.Rules))
	// 记录每条规则被哪些前面的规则覆盖（用于遮蔽归因）
	coverers := make([]map[int]bool, len(s.Rules))
	for i := range coverers {
		coverers[i] = map[int]bool{}
	}

	for _, c := range cells {
		matched := MatchCell(s, c)
		ids := make([]string, len(matched))
		for i, ri := range matched {
			ids[i] = s.Rules[ri].ID
		}
		switch {
		case len(matched) == 0:
			rep.Uncovered = append(rep.Uncovered, Region{Desc: s.Describe(c), Input: s.Representative(c)})
		case len(matched) > 1:
			rep.Overlaps = append(rep.Overlaps, Region{Desc: s.Describe(c), Input: s.Representative(c), Rules: ids})
		}
		if s.Strategy == StrategyFirst && len(matched) > 0 {
			w := matched[0]
			winner[w] = true
			if !witnessDone[w] {
				// 单元按最小输入顺序枚举，首个见证即最小输入
				rep.Witnesses = append(rep.Witnesses, Witness{RuleID: s.Rules[w].ID, Input: s.Representative(c)})
				witnessDone[w] = true
			}
			for _, ri := range matched[1:] {
				coverers[ri][w] = true
			}
		}
	}

	if s.Strategy == StrategyFirst {
		for i := range s.Rules {
			if winner[i] {
				continue
			}
			// 规则从未成为最早命中：要么不可达（空条件已在校验拒绝，
			// 故其区域非空），要么被前面规则联合完全遮蔽。
			names := []string{}
			for j := 0; j < i; j++ {
				if coverers[i][j] {
					names = append(names, s.Rules[j].ID)
				}
			}
			rep.Shadowed = append(rep.Shadowed, ShadowedRule{RuleID: s.Rules[i].ID, Coverers: names})
		}
	}
	return rep
}
