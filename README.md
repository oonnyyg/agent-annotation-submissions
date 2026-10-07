# bernoulli-sequential-audit

纯标准库的伯努利序贯分析工具（SPRT）。逐条回放成功/失败观测序列的判断过程，
并对固定样本上限下的停止概率做精确递推（非随机模拟）。

## 输入

方案 JSON（见 `examples/plan.json`）：

- `p0` / `p1` / `true_p`：均须严格位于 (0, 1)，且 `p1 > p0`（备择成功率高于原假设）。
- `lower` / `upper`：对数似然比下界（必须为负）与上界（必须为正）。
- `max_observations`：样本上限，1–200。

观测 CSV（见 `examples/observations.csv`）：列 `series_id,position,outcome`。
同一 `series_id` 内 `position` 必须唯一，`outcome` 只能为 0 或 1。
任何非法输入都会整次拒绝（退出码 2，不写输出文件）。

## 似然比方向

每条观测的增量为备择相对原假设的对数似然比：

- 成功（1）：`log(p1 / p0)`，为正，把累积量推向上界；
- 失败（0）：`log((1 - p1) / (1 - p0))`，为负，把累积量推向下界。

累积量 `>= upper` 判备择（`accept_alternative`），`<= lower` 判原假设
（`accept_null`），等号即触发。只认首次越界：停止后剩余观测不再影响结论，
报告记录停止位置与未使用观测数。

## 截断含义

到达 `max_observations` 时先判边界，仍在区间内才标 `undecided`
（原因 `max_observations_reached`）。数据提前耗尽同样不强行给结论
（原因 `data_exhausted`），报告给出已使用数量与最终似然比。

精确分析以 `true_p` 为真实成功率，对“尚未停止”路径的累积似然比分布逐步递推：
越界概率质量被移入对应边界并不再传播，因此每一步都满足
`P(停止于下界) + P(停止于上界) + P(仍在运行) = 1`（概率守恒）。
输出每步两类停止概率、上限处未定概率与截断期望样本数 `E[N] = Σ P(N >= k)`。

## 复现命令

```powershell
python -m seqaudit.cli --plan examples/plan.json --observations examples/observations.csv `
    --details-csv examples/details.csv --summary-json examples/summary.json
python -m unittest discover -s tests
```

输出：`--details-csv` 为逐条过程表（每个序列每步的增量、累积似然比与状态），
`--summary-json` 为各序列结论与精确分析报告。多个序列在同一方案下独立运行。
