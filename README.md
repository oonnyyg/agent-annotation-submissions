# Bernoulli Sequential Audit

纯标准库的序贯概率比检验（SPRT）工具：对逐条到达的 0/1 观测做回放判定，
并对给定的真实成功率精确递推停止概率，不做随机模拟。

## 输入

- 方案 JSON：`p0`（原假设成功率）、`p1`（备择成功率，必须大于 `p0`）、
  `lower`（负的下界）、`upper`（正的上界）、`max_observations`（1–200）、
  `true_p`（用于精确分析的真实成功率）。所有概率严格位于 (0, 1)。
- 观测 CSV：列 `sequence_id,position,value`。`value` 只能为 0 或 1，
  同一序列内 `position` 必须唯一；任何非法行都会整次拒绝运行。

## 判定规则

对每条观测累积对数似然比：成功加 `log(p1/p0)`（正值），失败加
`log((1-p1)/(1-p0))`（负值）。因此累积值上升支持备择、下降支持原假设。

- 累积值 `>= upper`：判 `alternative`；`<= lower`：判 `null`。
- 只认首次越界，之后的观测标记为 `unused`，不再影响结论。
- 到达 `max_observations` 时先判边界，仍在区间内才记为 `undecided`，
  停止原因为 `max_observations_reached`。
- 数据提前结束且未越界时同样为 `undecided`，原因为 `data_exhausted`，
  不强行给结论；输出已使用数量与最终似然比。

## 精确停止概率

`seqaudit.exact` 在 `true_p` 下对未停止路径做精确递推：第 n 步的累积
似然比只取决于成功次数 k，因此状态为 (n, k)。越界概率质量被吸收，
已停止路径不再继续传播，总概率守恒（各步停止概率 + 上限未定概率 = 1）。
输出每一步两类停止概率、上限未定概率与截断期望样本数
`E[min(T, max_observations)]`。

## 复现命令

```powershell
python -m seqaudit.cli --plan examples/plan.json `
  --observations examples/observations.csv --out-dir out
python -m unittest discover -s tests
```

输出文件：`out/steps.csv`（逐条过程表）、
`out/stopping_probabilities.csv`（每步停止概率）、
`out/summary.json`（各序列结论与精确分析汇总）。
