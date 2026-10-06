# Layered Refraction Arrivals

首波走时分析器：对水平层状模型（速度随深度严格增加，末层为无限半空间）
计算地表直达波与各界面临界折射波，找出每个检波点的最早到达路径。
全部单位为米（m）和秒（s），仅依赖 Python 标准库。

## 用法

```bash
python -m refraction input.json --csv paths.csv --json analysis.json
```

输入 JSON（`range` 与 `observed` 可选）：

```json
{
  "model": {"layers": [{"velocity": 500, "thickness": 100},
                       {"velocity": 2000}]},
  "offsets": [0, 60, 258.2, 500],
  "range": [0, 1000],
  "observed": [[60, 0.224], [500, 0.637]]
}
```

- `model.layers`：自上而下排列；除末层半空间外每层必须给出正的有限
  `thickness`；速度必须为正且严格递增，否则拒绝计算。
- `offsets`：检波点炮检距（≥ 0，支持零距离）。
- `range`：连续距离区间 `[x_min, x_max]`，用于首波分支分析。
- `observed`：实测首波 `[offset, time]`，仅做残差对比，不做反演。

输出：

- CSV：每个测点 × 每条路径 × 每个分段的明细（层号、角色
  down/headwave/up/direct、长度、水平位移、分段走时），并标记
  `reachable` 与 `is_first`。
- JSON：每个测点的首波到时与全部并列路径（含分段走时之和校验）、
  分支区间与切换点、始终不是首波的界面、残差/RMS 报告。

## 物理模型

直达波：`t = x / v1`。

界面 `i` 的临界折射波（首波沿第 `i+1` 层顶部传播）：

```
sin(θ_j) = v_j / v_{i+1}
t(x) = x / v_{i+1} + Σ_{j≤i} 2 h_j cos(θ_j) / v_j
```

### 临界距离 ≠ 首波交叉距离

- **临界距离** `x_crit = Σ 2 h_j tan(θ_j)`：折射波存在的最小出露距离。
  小于该距离时首波尚未出露，路径被标记为不可达（CSV 中
  `reachable=no`），绝不把直线走时公式外推成有效路径。
- **首波交叉距离**：两条有效路径走时相等的炮检距，即首波分支切换点。
  它由有效路径直线的解析交点求得，通常大于临界距离，且与输入测点
  无关——即使没有任何检波点落在切换点附近，分支边界依然精确。

某些界面虽可传播（`x_crit` 有限），其折射波却在任何距离都不是首波
（隐蔽层，例如被更深层高速界面压制）。分支分析会在
`never_first_interfaces` 中列出这些界面。

### 数值容差

并列首波与可达性判定使用相对容差 `TIME_TOLERANCE = 1e-9`
（按 `max(1, |t|)` 缩放）。走时在该容差内相等的路径全部保留在
`tied_paths` 中，例如交叉距离处直达波与折射波并列。

## 测试

```bash
python -m unittest discover -s tests
```

测试包含解析解校验：临界距离、截距时间、交叉距离的闭式解，
隐蔽层场景，分段走时与总走时一致性，以及 CLI 端到端流程。
