# ising-ensemble-lab

纯 Python 标准库实现的二维 Ising 模型实验工具：Metropolis 采样、小晶格精确枚举对照、
CSV 样本与 JSON 汇总输出。

## 模型约定

- L×L 方晶格（3 ≤ L ≤ 64），周期边界，自旋 s_i ∈ {+1, -1}。
- 能量：每条最近邻键只计一次，`E = -J Σ_<ij> s_i s_j - h Σ_i s_i`，玻尔兹曼常数 k_B = 1。
- 耦合 J 与温度 T 必须为有限正数，外场 h 有限（可为零或负）。
- 初态：`"all_up"`（全 +1）或长度 L² 的 ±1 列表。

## 采样口径

- 一轮扫描（sweep）= L² 次随机选点尝试，选点独立均匀、**允许重复**。
- 每次尝试为单点 Metropolis 翻转：`ΔE ≤ 0` 必接受，否则以 `exp(-ΔE/T)` 概率接受。
- 每个温度点先执行 `thermalization_sweeps` 轮热化（不进入任何统计），
  之后每完成 `sample_every` 轮完整扫描记录一次样本，共 `n_samples` 次。
- 每个样本记录**总能量**与**有符号总磁化**。

## 随机序列与可复现性

- 每个温度点使用独立的随机序列：`random.Random(f"ising|{seed}|{temperature!r}")`。
- 因此每个温度都从**相同初态**独立运行；重排温度列表不会改变任一温度的结果。
- 相同 seed 完全可复现；配置拒绝重复温度。

## 统计定义（均为总体矩，不做无偏修正）

设样本数 n、自旋数 N = L²，样本总能量 E_k、有符号总磁化 M_k：

- 每自旋平均能量：`<E>/N`
- 每自旋平均绝对磁化：`<|M|>/N`
- 每自旋比热：`c = Var(E) / (N T²)`，其中 `Var(E) = <E²> - <E>²`
- 每自旋磁化率：`χ = Var(M) / (N T)`，其中 `Var(M) = <M²> - <M>²`（用有符号磁化）

## 精确枚举（L = 3 或 4）

枚举全部 2^(L²) 个状态，权重用平移能量计算以保证数值稳定：
`w = exp(-(E - E_min)/T)`，配分函数 `Z = Σw`，均值与方差按同一口径
（总体矩）计算精确值。JSON 汇总中并列给出 `sampled`、`exact` 与
`sampling_error = sampled - exact`。

注意：有限样本的采样值**不要求**精确等于枚举值；两者差异反映统计涨落与
热化/关联时间，正是本工具要展示的内容（例如低温下磁化率采样常被低估）。

## 使用方法

```bash
python -m ising_lab examples/config_l4.json --samples samples.csv --summary summary.json
```

配置示例见 `examples/config_l4.json`。温度条目可写为数字（用默认
`thermalization_sweeps=100, sample_every=1, n_samples=100`）或对象覆盖。

输出：

- CSV：每次样本一行（temperature, sample_index, sweep, energy, magnetization）。
- JSON：每温度点的采样统计、精确值（L ≤ 4 时）与采样误差。

## 测试

```bash
python -m unittest discover -s tests
```

覆盖：单点翻转能量差与重算一致、周期边界邻居与键计数、L=3 精确枚举与
直接枚举对照、高温极限、固定种子复现、温度顺序无关、热化不计入统计、
配置校验（尺寸范围、正温度、重复温度、非法自旋等）。
