# Gray Enclosure Radiation Calculator

纯 Python 标准库实现的封闭腔体灰体漫反射辐射换热计算器。

## 用法

```sh
python -m enclosure_radiation config.json \
    --surfaces-csv surfaces.csv --exchange-json exchange.json
```

输入为 JSON 配置（见 `examples/three_surface.json`）：

- `surfaces`：几何与物性，`name`、`area`（m²，>0）、`emissivity`（(0, 1]）。
- `view_factors`：N×N 视角因子矩阵 `F[i][j]`（i → j）。允许凹面 `F[i][i] > 0`。
- `tolerances`：`row_sum` 与 `reciprocity` 容差（默认 1e-9）。
- `cases`：多个温度工况，共用同一几何。每个表面二选一：
  - `{"temperature_K": T}` 固定温度；
  - `{"adiabatic": true}` 净辐射热流为零（未控温绝热面）。

## 校验规则（不满足即拒绝，不做归一化修补）

- 矩阵元素非负、有限；每行和为 1（容差 `row_sum`）。
- 面积互易：`A_i·F_ij = A_j·F_ji`（容差 `reciprocity`）。
- 每个辐射连通分量（按 F 矩阵无向连通）至少有一个固定温度面，
  否则报 `UnderconstrainedError`，指出欠约束的分量成员。

## 物理模型

假设表面不透明、漫反射、灰体。对每个表面联立辐射度方程：

- 固定温度面：`J_i − (1−ε_i)·Σ_j F_ij·J_j = ε_i·σ·T_i⁴`
- 绝热面：`J_i − Σ_j F_ij·J_j = 0`（净热流为零，J_i = G_i）

其中 σ = 5.670374419e−8 W/(m²·K⁴)。反射再交换通过完整线性方程组隐式计入，
不做黑体近似，也不预填未知温度。解出 J 后：

- 辐照度 `G_i = Σ_j F_ij·J_j`
- 绝热面平衡温度 `T_i = (J_i/σ)^(1/4)`
- 净热流（向外为正）`q_i = A_i·(J_i − G_i)`
- 两两交换功率 `P_ij = A_i·F_ij·(J_i − J_j)`（满足 `P_ij = −P_ji`）

## 输出

- `surfaces.csv`：逐工况逐表面的温度、辐射度 J、辐照度 G、净热流密度、净功率。
- `exchange.json`：逐工况的两两交换功率、反对称性最大残差、
  逐面收支、全腔体能量残差（应接近 0，可用于精度核查）。

## 单位与精度

全部 SI 单位：m²、K、W、W/m²。线性方程组用带部分主元的高斯消元求解，
双精度下典型残差在 1e−9 W 量级；能量残差与反对称性残差均在输出中可查。
求解失败（校验错误、欠约束、奇异矩阵）时以非零退出码报错，
绝不输出伪装的零热流结果。

## 测试

```sh
python -m unittest discover -s tests
```

覆盖等温极限（净热流为零）、两无限大平行板解析解、
带绝热面的三表面腔体、多连通分量欠约束检测及全部输入校验拒绝路径。
