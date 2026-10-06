# finite-allele-drift

有限单倍体种群的等位基因演化精确计算器。仅依赖 Python 标准库，
通过完整转移矩阵推进离散概率分布，不做随机模拟，也不裁剪小概率状态。

## 模型

- 固定大小 `N` 的单倍体种群，个体分为目标类型与普通类型。
- 每代先选择后突变：目标类型相对适合度为 `1 + s`（必须为正），
  普通类型为 1。按适合度加权得到繁殖概率
  `p_sel = i(1+s) / (i(1+s) + (N-i))`，再应用双向突变：
  `p = p_sel(1-v) + (1-p_sel)u`，其中 `u` 为普通转目标概率，
  `v` 为目标转普通概率。
- 下一代目标个数服从 `Binomial(N, p)`，用 `(N+1) x (N+1)` 的完整
  转移矩阵精确推进分布；`p` 为 0 或 1 时退化为确定性行。
- 带突变时边界 0 和 N **不是**吸收态，分布可自由离开边界。

## 首次触边口径

- 另维护一份仅含内部状态（1..N-1）的“尚未触边”分布。
- 初始就在边界时，首次触边记在第 0 代（增量为 1）。
- 之后每代从内部流向边界的概率质量即该代首次触边增量；
  一旦触边即永久移出该分布，离开后再返回不会重复计数。
- 汇总中的 `boundary_hit_cumulative` 为累计首次触边概率，
  `prob_untouched_boundary` 为至今仍位于内部、从未触边的概率。

## 用法

```
python -m allele_drift config.json [--outdir OUT]
```

输入 JSON 支持多个独立方案：

```json
{
  "scenarios": [
    {
      "name": "neutral",
      "population_size": 20,
      "initial_count": 10,
      "generations": 60,
      "selection_coefficient": 0.0,
      "mutation_to_target": 0.0,
      "mutation_to_ordinary": 0.0
    }
  ]
}
```

非法配置（非正适合度、越界突变率、初始计数越界等）会被拒绝并给出错误信息。

## 输出

- 每个方案一个 `<name>_generations.csv`：从第 0 代起，每代每个计数的
  概率，以及该代的期望频率、`E[2f(1-f)]`、概率和（用于数值校验）、
  未触边概率、首次触边增量与累计值。
- 一个 `summary.json`：各方案参数、末代统计、逐代首次触边增量，
  以及 `max_probability_sum_error`（所有代中 |概率和 - 1| 的最大值，
  用于明确数值误差，纯浮点舍入量级）。

## 测试

```
python -m unittest discover -s tests
```
