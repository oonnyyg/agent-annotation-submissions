# spin-echo-sequence

本地自旋回波模拟工具：用可复现的脉冲序列观察失谐（频偏）与弛豫如何
共同影响磁化信号。仅使用 Python 标准库，不连接任何设备。

## 运行方式

```sh
python -m spin_echo examples/spin_echo.json --csv out.csv --summary out.json
```

- 位置参数：JSON 配置文件路径。
- `--csv`：CSV 输出路径（默认 `spin_echo_output.csv`）。
- `--summary`：JSON 摘要输出路径（默认 `spin_echo_summary.json`）。
- 配置非法时以非零状态退出，错误信息包含定位路径
  （如 `config.spins[0].weight`）。

运行测试：

```sh
python -m unittest discover -s tests
```

## 坐标约定

- 右手坐标系，z 轴沿主磁场方向。
- 自由演化：横向磁化以角频率 `2*pi*offset_hz` 绕 +z 轴作右手旋转
  （正频偏使 +x 转向 +y），横向分量按 `exp(-t/T2)` 衰减；纵向分量按
  `Mz_eq + (Mz - Mz_eq) * exp(-t/T1)` 向平衡值恢复。
- 脉冲：瞬时、不消耗时间，绕横平面内的相位轴
  `(cos(phase), sin(phase), 0)` 按翻转角 `flip` 作右手旋转。
- 同一时刻的多个脉冲按配置中的输入顺序依次执行；该时刻的采样发生在
  全部脉冲之后。
- 所有演化均为闭式解，不按采样间隔逐步推进，因此采样网格、重复采样
  与查询顺序都不会改变演化结果。

## 配置格式

```json
{
  "spins": [
    {"name": "fast", "weight": 1.0, "m0": [0.0, 0.0, 1.0],
     "offset_hz": 40.0, "t1": 1.5, "t2": 0.4, "mz_eq": 1.0}
  ],
  "pulses": [
    {"time": 0.0, "phase": 0.0, "flip": 1.5707963267948966}
  ],
  "samples": [0.0, 0.01, 0.02],
  "window": [0.0, 0.05]
}
```

- `spins[].weight`：非负权重，总和必须为正；信号按归一化权重合成。
- `spins[].m0`：初始磁化 `[Mx, My, Mz]`。
- `spins[].offset_hz`：固定频偏，单位赫兹（可为负）。
- `spins[].t1` / `spins[].t2`：纵向 / 横向弛豫时间，单位秒，必须为正；
  用 `null` 表示无穷大（无弛豫）。
- `spins[].mz_eq`：纵向平衡磁化，缺省为 0。
- `pulses[].time` / `samples[]`：时刻，单位秒，非负。
- `pulses[].phase` / `pulses[].flip`：相位轴与翻转角，单位弧度；
  `phase` 缺省为 0。
- `window`：峰值报告窗口 `[start, end]`（秒），缺省为全部采样。

时间一律以秒输入，频偏以赫兹输入，角度以弧度输入。NaN、负权重、
非正弛豫时间等非法数值会被定位并拒绝。

## 输出

CSV 每个采样时刻先列出各组的 `(Mx, My, Mz)`，随后一行 `<total>`
给出总复信号的幅值与相位（弧度）。总信号由归一化权重下的横向分量
先复数相加再取幅值和相位；幅值为零时相位输出为空。

JSON 摘要报告指定窗口内的峰值幅值及其最早出现时间，以及采样数、
组数和脉冲数。
