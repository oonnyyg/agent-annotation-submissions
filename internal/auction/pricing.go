package auction

import "sort"

// PriceStats 是某个候选价格上的累计买卖意愿量。
type PriceStats struct {
	Price     int64 `json:"price"`
	BuyVol    int64 `json:"buy_vol"`
	SellVol   int64 `json:"sell_vol"`
	Tradable  int64 `json:"tradable"`
	Imbalance int64 `json:"imbalance"`
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// CandidateStats 枚举候选价格（仅取当前有效订单出现过的价格），
// 并累计每个价格上愿意成交的买量与卖量。
func CandidateStats(orders []*Order) []PriceStats {
	priceSet := make(map[int64]struct{})
	for _, o := range orders {
		priceSet[o.Price] = struct{}{}
	}
	prices := make([]int64, 0, len(priceSet))
	for p := range priceSet {
		prices = append(prices, p)
	}
	sort.Slice(prices, func(i, j int) bool { return prices[i] < prices[j] })

	stats := make([]PriceStats, 0, len(prices))
	for _, p := range prices {
		var buyVol, sellVol int64
		for _, o := range orders {
			switch o.Side {
			case Buy:
				if o.Price >= p {
					buyVol += o.Remaining()
				}
			case Sell:
				if o.Price <= p {
					sellVol += o.Remaining()
				}
			}
		}
		stats = append(stats, PriceStats{
			Price:     p,
			BuyVol:    buyVol,
			SellVol:   sellVol,
			Tradable:  min(buyVol, sellVol),
			Imbalance: abs(buyVol - sellVol),
		})
	}
	return stats
}

// ChoosePrice 按规则选择成交价：
// 先最大化可成交量，再最小化失衡量，再取距离 ref 最近的价格，
// 仍同分取较低价。最大可成交量为零时返回 (0, false)。
func ChoosePrice(stats []PriceStats, ref int64) (int64, bool) {
	var best *PriceStats
	for i := range stats {
		s := &stats[i]
		if s.Tradable <= 0 {
			continue
		}
		if best == nil || better(s, best, ref) {
			best = s
		}
	}
	if best == nil {
		return 0, false
	}
	return best.Price, true
}

func better(c, b *PriceStats, ref int64) bool {
	if c.Tradable != b.Tradable {
		return c.Tradable > b.Tradable
	}
	if c.Imbalance != b.Imbalance {
		return c.Imbalance < b.Imbalance
	}
	dc, db := abs(c.Price-ref), abs(b.Price-ref)
	if dc != db {
		return dc < db
	}
	return c.Price < b.Price
}
