package farecap

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Config 描述费率规则。金额一律为整数分。
type Config struct {
	// UTCOffsetMinutes 固定 UTC 偏移（分钟），不涉夏令时。
	UTCOffsetMinutes int `json:"utcOffsetMinutes"`
	// TransferWindowMinutes 换乘时限（分钟），窗口自换乘组首趟上车起算。
	TransferWindowMinutes int `json:"transferWindowMinutes"`
	// Fares 按覆盖区间宽度（起终区号最大减最小再加一）索引的票价表（分）。
	Fares map[string]int `json:"fares"`
	// DailyCapCents 服务日封顶（分），非负。
	DailyCapCents int `json:"dailyCapCents"`
	// WeeklyCapCents 服务周封顶（分），非负。
	WeeklyCapCents int `json:"weeklyCapCents"`
}

// LoadConfig 从 r 读取并校验配置。
func LoadConfig(r io.Reader) (*Config, error) {
	var c Config
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if c.UTCOffsetMinutes < -12*60 || c.UTCOffsetMinutes > 14*60 {
		return nil, fmt.Errorf("utcOffsetMinutes 超出合法范围: %d", c.UTCOffsetMinutes)
	}
	if c.TransferWindowMinutes <= 0 {
		return nil, fmt.Errorf("transferWindowMinutes 必须为正: %d", c.TransferWindowMinutes)
	}
	if c.DailyCapCents < 0 || c.WeeklyCapCents < 0 {
		return nil, fmt.Errorf("日/周上限不能为负: %d/%d", c.DailyCapCents, c.WeeklyCapCents)
	}
	if len(c.Fares) == 0 {
		return nil, fmt.Errorf("票价表不能为空")
	}
	type kv struct {
		width int
		fare  int
	}
	var rows []kv
	for k, v := range c.Fares {
		var w int
		if _, err := fmt.Sscanf(k, "%d", &w); err != nil || w <= 0 {
			return nil, fmt.Errorf("票价表键必须为正整数宽度: %q", k)
		}
		if v < 0 {
			return nil, fmt.Errorf("票价不能为负: 宽度 %d = %d", w, v)
		}
		rows = append(rows, kv{w, v})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].width < rows[j].width })
	for i := 1; i < len(rows); i++ {
		if rows[i].fare < rows[i-1].fare {
			return nil, fmt.Errorf("票价表必须单调非降: 宽度 %d(%d) 低于宽度 %d(%d)",
				rows[i].width, rows[i].fare, rows[i-1].width, rows[i-1].fare)
		}
	}
	return &c, nil
}

// FareFor 查询覆盖宽度 width 的票价；缺失返回错误。
func (c *Config) FareFor(width int) (int, error) {
	if width <= 0 {
		return 0, fmt.Errorf("非法区间宽度: %d", width)
	}
	key := fmt.Sprintf("%d", width)
	if f, ok := c.Fares[key]; ok {
		return f, nil
	}
	return 0, fmt.Errorf("缺少宽度 %d 的票价", width)
}
