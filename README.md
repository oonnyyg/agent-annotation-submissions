# finite-allele-drift

有限单倍体种群的等位基因演化计算器。仅用 Python 标准库，通过完整转移矩阵
精确推进离散概率分布（不做随机模拟），观察选择与双向突变如何改变代际分布，
并区分"当前位于边界"与"历史上曾经触及边界"。

## 模型

- 固定大小 `N` 的**单倍体**种群，含目标（target）与普通（normal）两种类型。
- 每代**先选择、后突变**：
  1. 选择：目标类型相对适合度为 `1 + s`（必须为正），普通类型为 1。
     当前目标数为 `i` 时，繁殖概率
     `p_sel = i(1+s) / (i(1+s) + (N-i))`。
  2. 突变：目标以概率 `v` 转为普通，普通以概率 `u` 转为目标：
     `p = p_sel(1-v) + (1-p_sel)u`。
- 下一代目标数服从 `Binomial(N, p)`。`p = 0` 或 `p = 1` 时退化为确定转移，
  不会裁掉任何小概率状态，所有 `N+1` 个计数状态全程保留。

## 首次触边口径

- 边界指计数 `0` 或 `N`。**边界不是吸收态**（有突变时过程会离开边界）。
- 若初始计数就在边界，首次触边记在第 0 代（增量为 1）。
- 首次触边只记录一次：离开边界后再返回不会重复计数。
- `untouched_probability` 是到该代为止从未触及任一边界的概率；
  `first_hit_increment` 是该代首次触边的概率增量；
  `first_hit_cumulative` 是其累计值。

## 用法

```
python -m allele_drift config.json --out-dir out
```

输入 JSON 可以是单个方案对象、方案数组，或 `{"scenarios": [...]}`：

```json
{
  "scenarios": [
    {
      "name": "neutral_drift",
      "population_size": 20,
      "initial_count": 8,
      "generations": 30,
      "selection_coefficient": 0.0,
      "mutation_to_target": 0.0,
      "mutation_from_target": 0.0
    }
  ]
}
```

字段校验：`population_size >= 1`、`0 <= initial_count <= N`、
`generations >= 0`、`1 + s > 0`、两个突变概率都在 `[0, 1]`，
非法配置会被拒绝并报错。

## 输出

- `<name>.csv`：从第 0 代开始，每代每个计数的概率，以及该代的期望频率、
  期望 `2f(1-f)`、首次触边增量/累计、未触边概率。
- `summary.json`：每个方案的参数、每代汇总（不含完整分布）以及
  `max_probability_sum_error`。

## 数值误差

每代检查概率和是否为 1。所有代的最大偏差写入
`max_probability_sum_error`；若超过容差 `1e-9`
（`allele_drift.core.PROB_SUM_TOLERANCE`）则抛出 `ArithmeticError`。

## 测试

```
python -m unittest discover -s tests
```

覆盖中性期望守恒、单向突变、初始边界（第 0 代记录、离开不重复计数）、
二项行边界概率与非法配置拒绝。
