# 串联杆件循环塑性分析工具

纯 Python 标准库实现的一维小应变弹塑性分析：给定端部位移历程，求解串联杆各段的
应力、应变、塑性变形与支座反力，区分真实残余变形与求解误差。不依赖任何有限元
或数值计算库。

## 模型假设

- 小应变一维杆，左端固定（`u_0 = 0`），右端按规定位移历程逐步加载。
- 不考虑惯性、温度效应；初始状态无应力、无塑性应变。
- 材料为线性各向同性硬化：屈服函数 `f = |σ| − (σ_y0 + H·α)`，拉压对称，
  其中 `α` 为累计塑性应变（单调不减），塑性应变 `ε_p` 保留符号。
- 更新采用弹性预测 + 返回映射（闭合形式），塑性一致切线模量为 `E·H/(E+H)`。

## 输入

模型 JSON（`examples/model.json`）：

```json
{
  "name": "two-segment-demo",
  "material": {"elastic_modulus": 200000.0, "yield_strength": 250.0, "hardening_modulus": 2000.0},
  "segments": [
    {"length": 1.0, "area": 100.0},
    {"length": 2.0, "area": 200.0, "material": {"elastic_modulus": 200000.0, "yield_strength": 300.0, "hardening_modulus": 2000.0}}
  ]
}
```

- 顶层 `material` 为默认材料，段内 `material` 可覆盖；两者至少提供一个。
- `elastic_modulus`、`yield_strength`、`hardening_modulus`、`length`、`area`
  必须为正的有限数；非有限或非法输入会被拒绝并报错退出。

位移历程 CSV（`examples/displacements.csv`）：单列无表头，或带表头且包含
`displacement` 列；每行一个按序施加的端部位移，必须为有限数值。

## 单位

程序不内置单位制，输入自洽即可。例如长度用 m、力用 N 时，弹性模量与屈服强度
用 Pa；长度用 mm、力用 N 时，模量用 MPa（N/mm²）。输出与输入单位一致。

## 求解与收敛判据

- 未知量为内部节点位移；残差为相邻段轴力之差 `R_j = N_{j-1} − N_j`。
- 每步用一致切线 Newton 迭代（带残差下降回溯线搜索）求解：
  当 `max|R_j| ≤ tol · max(1, max|N_i|)` 时判收敛，默认 `tol = 1e-10`。
- 所有试算都从上一收敛步的材料历史出发，迭代中不修改历史；只有整步收敛
  后才统一提交塑性应变与累计塑性应变，因此不会在迭代中重复累加塑性。
- 达到迭代上限（默认 50）时该步标记为 `converged = false`，结果字段为
  `null`（CSV 中为空，绝不补零），保留此前已收敛结果并停止分析。
- 轴力不平衡残差 `force_imbalance` 与位移兼容残差 `compatibility_residual`
  逐步输出；后者恒为机器精度量级，可用于区分真实残余塑性变形与求解误差。

## 用法

```bash
python -m series_bar examples/model.json examples/displacements.csv --out-dir output
```

选项：`--tol` 相对残差容差，`--max-iter` 每步最大迭代次数，`--out-dir` 输出目录。

退出码：`0` 全部收敛；`1` 存在未收敛步（已保留此前结果）；`2` 输入非法。

## 输出

- `steps.csv`：逐步规定位移、收敛标志、迭代数、支座反力 `reaction_force`
  （固定端对杆的作用力，等于轴力取负）、轴力、各节点位移 `u_0..u_N`、
  轴力不平衡与位移兼容残差。
- `segments.csv`：逐步各段总应变、应力、轴力、有符号塑性应变、累计塑性
  应变与屈服标志。
- `diagnostics.json`：容差与迭代上限、每步收敛诊断、失败步编号以及最后
  收敛步的完整塑性状态。

## 测试

```bash
python -m unittest discover -s tests
```

覆盖纯弹性（变截面轴力相等、伸长按柔度分配）、先后屈服、卸载与反向加载
（塑性应变有符号、累计量单调不减）、残余变形、失败步处理与非法输入拒绝。
