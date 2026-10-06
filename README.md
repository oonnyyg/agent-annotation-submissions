# Ising Ensemble Lab

纯 Python 标准库实现的二维 Ising 模型实验工具：单点 Metropolis 采样、
小晶格精确枚举核查、CSV 样本与 JSON 汇总输出。玻尔兹曼常数取 1。

## 运行

```bash
python -m ising examples/config.json --samples-csv samples.csv --summary-json summary.json
python -m unittest discover -s tests
```

## 配置（JSON）

- `size`：方形晶格边长，3 到 64 的整数，周期边界。
- `coupling`：耦合强度 J，有限且为正。
- `temperatures`：温度列表，每个有限且为正，不允许重复。
- `field`：外场 h，有限，可为零（默认 0）。
- `equilibration_sweeps` / `sampling_sweeps` / `sample_interval`：正整数，
  分别为热化扫描数、采样扫描数、采样间隔（默认 1000 / 10000 / 10）。
- `seed`：整数随机种子。
- `initial_spins`：可选，长度为 size² 的 ±1 列表；缺省为全 +1 初态。

## 模型与算法定义

- 总能量 `E = -J * Σ_<ij> s_i s_j - h * Σ s_i`，每条最近邻键只计一次。
- 一轮扫描（sweep）= size² 次随机选点尝试，允许重复选点；
  每次尝试按 Metropolis 准则翻转单点：`ΔE ≤ 0` 必接受，否则以
  `exp(-ΔE / T)` 概率接受。
- 热化扫描的数据不进入统计；采样阶段每完成 `sample_interval` 轮完整
  扫描记录一次样本（总能量与有符号总磁化）。

## 随机序列与可复现性

每个温度使用独立的随机流，种子为 `"ising:<seed>:<rank>"`，其中 `rank`
是该温度在排序后温度列表中的名次。因此固定种子可完全复现，且重排
配置中的温度列表不会改变任何温度的结果；每个温度都从相同初态独立运行。

## 统计定义（全部为总体矩，不做无偏修正）

- 每自旋平均能量 `<E>/N`，每自旋平均绝对磁化 `<|M|>/N`。
- 每自旋比热 `C/N = Var(E) / (N T²)`，其中 `Var(E) = <E²> - <E>²`。
- 每自旋磁化率 `χ/N = Var(M) / (N T)`，使用**有符号**磁化的方差。

## 精确枚举核查

边长为 3 或 4 时，枚举全部 2^N 个自旋状态，用数值稳定的玻尔兹曼权重
`exp(-β(E - E_min))` 计算同口径精确均值与响应量，并在 JSON 汇总中并列
采样误差（`sampling_error`）。有限样本只是统计涨落意义下接近精确值，
不应以“必须吻合精确值”作为程序正确性的判据。

## 输出

- CSV：每行一个样本（`temperature, sweep, energy, magnetization`）。
- JSON：晶格与调度参数、每个温度的采样汇总、（小晶格时）精确值与采样误差。
