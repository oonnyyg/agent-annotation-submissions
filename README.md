# budgeted-stock-policy

一次性销售活动的备货决策工具：在共享采购预算下，对多个商品求**全局最优**的整数订货组合，
并把每个商品的期望收益拆解为销售、残值回收、采购成本与缺货损失，帮助判断收益究竟来自
真实销售，还是来自承担缺货 / 滞销风险。

仅使用 Python 标准库（3.12+），不调用任何在线服务或优化库。

## 用法

```bash
python -m stock_policy INPUT.json [--json OUT.json] [--csv OUT.csv]
```

- 不指定 `--json` 时，最优备货 JSON 写到标准输出。
- 指定 `--csv` 时，额外输出逐商品、逐预算方案的收益 CSV。
- 输入非法时以非零码退出，且不会生成任何部分输出文件。

## 输入格式

```json
{
  "items": [
    {
      "id": "A",
      "price": 1000,
      "cost": 600,
      "salvage": 100,
      "shortage_penalty": 50,
      "demand": [
        {"quantity": 0, "probability": "0.2"},
        {"quantity": 1, "probability": "0.5"},
        {"quantity": 2, "probability": "0.3"}
      ]
    }
  ],
  "budgets": [0, 600, 1500]
}
```

约束（全部在求解前校验，任一违反即整体报错）：

- `price` / `cost` / `salvage` / `shortage_penalty` / `budgets` 均为整数分；
  `cost > 0`，`price >= cost`，`0 <= salvage <= cost`，`shortage_penalty >= 0`。
- `demand` 是非负整数需求量的离散分布；`probability` 必须是十进制**字符串**，
  相同 `quantity` 的条目自动合并，合并后概率之和必须严格等于 1。
- 商品 `id` 唯一；`budgets` 中每个方案独立求解，输入中的商品顺序不影响结果。

## 收益模型

对每个商品与每个 0 到需求上限的整数备货量 `q`，按完整需求分布（不以均值代替）计算：

```
期望销量   E[sales]    = Σ p_d · min(q, d)
期望剩余   E[leftover] = Σ p_d · max(q - d, 0)
期望缺货   E[shortage] = Σ p_d · max(d - q, 0)
期望收益 = price·E[sales] + salvage·E[leftover] - cost·q - shortage_penalty·E[shortage]
```

解读：若某商品期望收益主要来自 `price·E[sales]`，收益来自真实销售；若备货量高而
`E[leftover]` 大，说明在赌残值回收（滞销风险）；若 `E[shortage]` 大且缺货损失高，
说明少备货在承担缺货风险。概率与收益全程用 `decimal.Decimal` 精确计算，
不做提前四舍五入，因此最优组合不受舍入影响。

## 组合优化与平局规则

多商品共享一个预算，用动态规划（多重选择背包）枚举所有满足
`Σ cost_i · q_i <= budget` 的组合，取期望总收益最大者——**不是**按单件收益率贪心。
收益并列时依次：

1. 总采购成本（资金占用）更小者优先；
2. 按商品编号排序后的数量向量，字典序更小者优先。

## 输出

JSON 中每个预算方案给出 `total_cost`、`expected_profit` 及逐商品的
`quantity`、`expected_sales`、`expected_leftover`、`expected_shortage`、
`expected_profit`；小数以十进制字符串输出。CSV 每行对应一个（预算, 商品）组合。

## 问题规模限制

DP 状态数为 `O(预算 × 商品数)`，每步转移枚举该商品全部备货量，整体约为
`O(预算 × Σ(最大需求_i + 1))`。适用于预算在数百万分（数万元）以内、
单商品需求上限在数千以内的场景；更大的实例会因内存与时间随预算线性增长而不可行。

## 测试

```bash
python -m unittest discover -s tests -v
```

覆盖零预算、滞销残值、贪心失效（DP 优于单件收益率贪心）、平局规则、
输入顺序无关性、精确十进制概率及各类非法输入。
