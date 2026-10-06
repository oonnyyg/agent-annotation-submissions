# welch-cross-spectrum

双路同步信号分析工具：基于 Welch 平均的互谱分析，找出两路信号的共同频率成分及相位关系，
并诊断缺测窗口对结果的影响。仅使用 Python 标准库（Python 3.12+）。

## 用法

```
python -m cross_spectrum CONFIG.json
```

配置示例：

```json
{
  "csv_path": "data.csv",
  "sample_rate": 256.0,
  "segment_length": 128,
  "overlap": 64,
  "nfft": 128,
  "window": "hann",
  "spectrum_output": "spectrum.csv",
  "diagnostics_output": "diagnostics.json"
}
```

可选键：`index_column`（默认 `index`）、`channel1_column`（默认 `ch1`）、
`channel2_column`（默认 `ch2`）。

### 参数约束

- `sample_rate` 必须为正数（Hz）。
- `segment_length` 为整数，3 到 512 之间。
- `overlap` 为非负整数且小于 `segment_length`。
- `nfft` 为整数，不得短于 `segment_length`。
- `window` 为 `hann`（周期 Hann 窗）或 `rectangular`。

### 输入 CSV

记录按等间隔样本序号（`index` 列）定位，采样间隔为 `1/sample_rate`。
缺测可以表现为：某序号整行缺失，或某通道单元格为空/非数值。

## 算法说明

- 段窗从首个样本序号开始，按固定步长 `segment_length - overlap` 滑动；末尾不足整段的部分丢弃。
- 任一通道在段内存在缺测，整段跳过，不拼接缺口两侧的数据；跳过原因记入诊断。
- 每段两路分别去均值后乘窗；零填充只发生在乘窗之后（补至 `nfft`）。
- 归一化因子为 `sample_rate * sum(window^2)`，得到单边功率谱密度。
- 先对各段的自谱与复互谱分别做 Welch 平均，再由平均后的谱计算相干，绝不平均每段相干。
- 互谱定义为 `conj(FFT(ch1)) * FFT(ch2)`。
- 单边谱除直流及存在的奈奎斯特频点（`nfft` 为偶数时）外，幅值翻倍。
- 没有任何有效段时明确报错失败。

## 输出

### spectrum.csv

列：`frequency_hz, psd_ch1, psd_ch2, cross_real, cross_imag, coherence, phase_rad`。

- **频谱单位**：PSD 与互谱幅值的单位为 `信号单位²/Hz`（单边）。
- **相位约定**：`phase_rad = arg(conj(X1) * X2)`，单位为弧度。
  正值表示 ch2 超前 ch1；负值表示 ch2 滞后。
- 自谱乘积为零时 `coherence` 为空；互谱为零时 `phase_rad` 为空——不伪造零相位或满相干。

### diagnostics.json

- `valid_segments`：有效段数。
- `skipped_segments`：每个被跳过段的起始序号与原因（整行缺失或某通道缺测）。
- `energy_check`：每通道的谱积分（`sum(PSD) * df`）与分段加窗平均能量
  （`mean(sum((x - mean) * w)^2) / sum(w^2)`）的对比及相对误差，按 Parseval 定理两者应一致。

## 缺测窗口的影响

缺测不插值、不拼接：含缺测的段被整体剔除，等效于减少 Welch 平均的段数。
其后果是谱估计方差增大（平均次数减少）、有效分析时长缩短，但不会在缺口处
引入拼接瞬态或虚假频率成分；若缺测集中在某些频段对应的时间段，相位与相干
估计仍无偏，只是不确定性上升。诊断文件中的跳过清单可用于评估缺测的分布。

## 测试

```
python -m unittest discover -s tests
```

覆盖：同频已知相移的相位/相干恢复、独立噪声的低相干、缺测段跳过（含整行缺失）、
全部段无效时失败、配置校验与端到端 CLI。
