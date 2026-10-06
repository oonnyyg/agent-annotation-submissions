# compound-gear-kinematics

复合齿轮传动核算工具：从连接关系建立转速约束，联立求解各轴转速，并在转速
唯一确定时核对给定外力矩下的功率收支。仅依赖 Python 标准库。

## 用法

```bash
python -m gear_kinematics input.json -o outdir
```

输出 `speeds.csv`（每个工况每轴一行，单位 rpm，欠约束轴留空）与
`diagnostics.json`（逐工况状态、约束残差、矛盾来源、自由度、功率收支）。
示例见 `examples/sample.json`。

## 输入格式

- `axes`：轴名列表。所有轴共用统一正方向，转速为带符号值。
- `connections`：连接列表，每条带 `id` 与 `type`：
  - `rigid`：`{"axes": [a, b]}`，同轴刚性连接，约束 `wa - wb = 0`。
  - `external_mesh`：`gear_a`/`gear_b` 各含 `axis` 与 `teeth`（正整数），
    约束 `wa*za + wb*zb = 0`（转向相反）。
  - `internal_mesh`：同上但 `gear_a` 为内齿圈（齿数须大于小齿轮），
    约束 `wa*za - wb*zb = 0`（转向相同）。
  - `planetary`：`sun`、`ring`（各含 `axis`、`teeth`）、`carrier`（轴名）、
    `planet_teeth`。校验齿数为正整数且几何关系
    `z_ring == z_sun + 2*z_planet` 成立；约束为 Willis 方程
    `(ws - wc)*zs + (wr - wc)*zr = 0`。
- `cases`：工况列表，互不影响。每项含：
  - `inputs`：部分轴的给定转速 `{"axis": a, "rpm": v}` 或固定
    `{"axis": a, "fixed": true}`（即 0 rpm）。
  - `torques`：各轴外力矩（N·m，与转速同号约定），未给出的轴按 0 处理。

## 符号约定与单位

- 转速输入/输出均为转每分钟（rpm），带符号，正号表示统一正方向。
- 功率按 `P = T * w` 计算，其中 `w = rpm * 2π/60`（rad/s），输出瓦特（W），
  带符号；`power.total_W` 为全部轴的收支总和，`balanced` 表示总和在
  1e-6 W 以内。不求内部反力矩，不模拟效率损失。
- 数值容差：线性求解与残差判定使用 `1e-9`（见 `solver.TOL`）。

## 诊断语义

- `unique`：全部轴转速唯一确定，输出转速、逐约束残差与功率收支。
- `underdetermined`：报告剩余自由度 `dof` 与自由轴 `free_axes`；已确定轴
  仍给出真实转速，自由轴留空，绝不擅自置零；不输出功率。
- `conflicting`：报告一组不可约的矛盾约束来源 `conflict_sources`
  （连接 `id` 或 `工况名/input[轴]`）；重复但一致的约束允许存在。

求解基于高斯消元（RREF），连接记录顺序不影响物理解，也不会把整个轮系
简化为单一传动比。

## 测试

```bash
python -m unittest discover -s tests -v
```
