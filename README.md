# aquifer-pumping-response

多井抽水响应计算器：基于理想承压含水层的 Theis 模型，预测观测点降深时序，
比较停泵后的恢复过程，并对用户给定的有限参数候选计算加权平方误差排序。
仅使用 Python 标准库，不依赖外部水文库。

## 模型假设

- 均质、各向同性、无限延展的承压含水层，导水系数 `T`（m²/s）与贮水系数 `S`（-）为常数且必须为正。
- 每口井流量分段恒定（m³/s）：正值抽水、负值回灌；首个时刻之前流量为零。
- 每个流量增量按 Theis 井函数产生响应：
  `s = ΔQ / (4πT) · W(u)`，`u = r²S / (4T·Δt)`，`W(u)` 为指数积分 E1(u)。
- 所有井、所有流量增量在时间和空间上线性叠加。切换瞬间新增量贡献为零；
  停泵作为负增量继续参与计算，停泵前的历史响应不会被清空，因此能正确
  模拟恢复过程。
- 全部量使用统一的米-秒单位。

## 井函数数值方法

`W(u) = E1(u)` 对极小与极大的无量纲时间参数采用两种互补算法：

- `u ≤ 1`：幂级数（A&S 5.1.11）。
- `u > 1`：Lentz 连分式（A&S 5.1.22）。

精度由 `well_function.tolerance` 与 `well_function.max_terms` 控制。
求值是 `(u, tolerance, max_terms)` 的纯函数：重复查询或增加采样点不会
改变已有时刻的结果。未收敛时抛出带失败原因的 `WellFunctionError`。

## 用法

```
python -m aquifer_response job.json -o outdir
```

输出 `drawdown.csv`（每观测点总降深及每口井贡献）与 `report.json`
（模型参数、各井贡献汇总、残差、候选排序、井函数收敛统计）。
计算失败时以非零码退出，且不会覆盖已有输出文件（原子写入）。

## 输入 JSON

```json
{
  "aquifer": {"transmissivity": 0.001, "storativity": 0.0002},
  "wells": [
    {"name": "PW1", "x": 0, "y": 0,
     "schedule": [{"time": 0, "rate": 0.02}, {"time": 86400, "rate": 0.0}]}
  ],
  "observation_points": [{"name": "OW1", "x": 30, "y": 0}],
  "times": [3600, 43200, 86400],
  "well_function": {"tolerance": 1e-14, "max_terms": 10000},
  "observations": [
    {"point": "OW1", "time": 43200, "drawdown": 1.0, "weight": 1.0}
  ],
  "candidates": [
    {"name": "base", "transmissivity": 0.001, "storativity": 0.0002}
  ]
}
```

- `schedule` 时间必须非递减；观测点不得与任何井重合。
- `observations` 可选，`weight` 必须为正。
- `candidates` 可选（提供时必须同时给出 `observations`）：逐一计算加权
  平方误差并排序，同分保持输入顺序，不做连续优化。

完整示例见 `examples/job.json`。

## 测试

```
python -m unittest discover -s tests
```

覆盖：井函数基准值与极端参数、单井 Theis 基准、多井叠加、抽停恢复、
采样密度不变性、候选排序与同分稳定性、失败时不覆盖已有报告。
