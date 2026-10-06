# Particle Settling Column

静水沉降柱分析工具，纯 Python 标准库实现（Python ≥ 3.12），按粒级计算
终端沉降速度，预测取样层平均浓度，并核对悬浮/沉积质量收支。

## 模型假设

- 静止流体中的球形颗粒，不考虑絮凝与颗粒间相互作用。
- 净重力与阻力平衡：`0.5 * rho_f * v^2 * A * Cd = (rho_p - rho_f) * g * V`，
  其中 `A = pi d^2 / 4`，`V = pi d^3 / 6`。
- 阻力系数 `Cd = 24/Re + 3/sqrt(Re) + 0.34`，`Re = rho_f v d / mu`；
  不直接套用低雷诺数 Stokes 公式，而是用带二分保护的牛顿迭代求唯一正根。
  未收敛时抛出 `ConvergenceError`，不生成任何预测。
- 初始时各粒级在 `[0, H]` 水深内均匀分布；忽略加速阶段，悬浮区域以终端
  速度整体向下平移，到达床面（`z = H`）的颗粒立即沉积。
- 任意非负时刻：悬浮质量 + 累计沉积质量 = 初始质量（逐粒级与总量均闭合）。
- 取样浓度按平移后悬浮区域 `[v t, H]` 与取样区间 `[z_top, z_bottom]` 的
  重叠长度计算区间平均质量浓度，而非仅判断区间中心。

## 单位

全部使用 SI 单位：米（m）、千克（kg）、秒（s）。深度从水面向下计量。
动力黏度 Pa·s，密度 kg/m³，重力加速度 m/s²，浓度 kg/m³。

## 输入

JSON 配置（见 `examples/config.json`）：

```json
{
  "column": {"depth": 2.0, "area": 0.5},
  "fluid": {"density": 1000.0, "viscosity": 0.001},
  "gravity": 9.81,
  "classes": [
    {"name": "coarse_sand", "diameter": 0.0002, "density": 2650.0, "mass": 2.0}
  ]
}
```

所有数值必须为正的有限值；颗粒密度必须大于流体密度。

取样请求 CSV（列：`time,z_top,z_bottom`）：区间必须位于水柱
`(0, depth)` 内且厚度为正，时间非负。

## 用法

```
python -m settling examples/config.json --samples examples/samples.csv --outdir out
```

输出（写入 `--outdir`，默认当前目录）：

- `velocities.csv`：各粒级终端速度、雷诺数、阻力系数、力平衡相对残差、迭代次数。
- `concentrations.csv`：每个取样时刻/区间/粒级的平均质量浓度（kg/m³）。
- `mass_balance.csv`：各取样时刻（及 `--times` 指定时刻）逐粒级与总计的
  初始、悬浮、沉积质量及闭合误差。

## 测试

```
python -m unittest discover -s tests
```

覆盖：细颗粒 Stokes 极限、高雷诺数粗颗粒、力平衡残差、未收敛报错、
部分清水层、粒级完全沉降、区间重叠（非中心判断）、区间校验与质量闭合。
