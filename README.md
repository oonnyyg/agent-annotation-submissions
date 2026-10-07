# gapped-allan-stability

纯 Python 标准库实现的**带缺口重叠 Allan 偏差（overlapping Allan deviation）**分析工具，
用于频率稳定性实验。采样缺口被切分为独立连续段：不插值、不跨段拼接平均窗口。

## 量纲约定

- 时间列：秒（s），必须严格递增且落在统一采样网格 `t_k = t_0 + k * tau0` 上。
- 频率列：无量纲相对频率（fractional frequency, y）。
- Allan 偏差输出：无量纲；平均时长 `tau = m * tau0` 以秒报告。

## 输入

CSV 数据（默认列名 `time,frac_freq`，可在配置中改）：

```csv
time,frac_freq
0.0,1.2e-9
1.0,1.1e-9
```

JSON 配置：

```json
{
  "tau0": 1.0,
  "m_values": [1, 2, 4, 8, 16],
  "detrend": false,
  "time_column": "time",
  "value_column": "frac_freq"
}
```

- `tau0`：基础采样间隔（秒，正数）。
- `m_values`：正整数平均长度列表。
- `detrend`：默认 `false`。为 `true` 时先用**全部观测的实际时间**做普通最小二乘
  线性拟合并减去趋势（缺口前后的时间距离不被压缩），再重复分析；JSON 报告同时
  保留原始结果（`raw`）、去趋势结果（`detrended_results`）与拟合系数（`trend_fit`）。

## 用法

```sh
python -m gapped_allan.cli --data data.csv --config config.json \
    --out-csv report.csv --out-json report.json
```

或安装后使用入口命令 `gapped-allan`（见 `pyproject.toml` 的 `[project.scripts]`）。

输出 CSV 按平均时长排序，列为 `m,tau_s,allan_dev,allan_var,pairs,status`。
某尺度数据不足时 `status` 为 `insufficient_data`，偏差/方差字段为空（JSON 中为
`null`），绝不输出零或非有限数字。配置或网格非法时以非零码退出，且**不会覆盖**
已有输出文件（原子写入）。

## 算法

对每个平均长度 `m`，在每段内逐点滑动取两个相邻、各含 `m` 点的均值，累计均值差
平方 `sum(d^2)` 与有效差分数 `N`。多段按差分数量汇总合并（不直接平均各段偏差）：

```
AVAR(m) = sum(d^2) / (2 N)
ADEV(m) = sqrt(AVAR(m))
```

## 验证方法

```sh
python -m unittest discover -s tests -v
```

测试覆盖：

- **恒定频偏**：常数序列各尺度 ADEV 为 0；交替 ±1 序列 AVAR(1)=2 与理论一致。
- **线性漂移**：`y = a*t` 时 `AVAR(m) = (a*m*tau0)^2 / 2`；OLS 去趋势后 ADEV 归零，
  且拟合使用真实时间戳（缺口不被压缩）。
- **分段合并**：两段数据按有效差分数量加权汇总，验证不产生跨缺口窗口对。
- **CLI**：端到端报告、非法配置/离网数据不覆盖已有输出、数据不足状态。
