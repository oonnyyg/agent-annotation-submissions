# spin-echo-sequence

本地自旋回波模拟工具：用可复现的脉冲序列观察失谐（频偏）与弛豫如何共同影响磁化信号。
仅使用 Python 标准库，不连接任何硬件设备。

## 运行方式

```sh
python -m spin_echo examples/spin_echo.json --csv signal.csv --summary summary.json
```

- 位置参数：JSON 配置文件路径。
- `--csv`：各采样时刻的分组磁化与总信号 CSV（默认 `signal.csv`）。
- `--summary`：JSON 摘要（默认 `summary.json`），含采样窗内峰值及最早出现时间。
- 配置非法时以非零状态退出，错误信息指出具体位置（如 `spins[1].t2`）。

运行测试：

```sh
python -m unittest discover -s tests
```

## 坐标与物理约定

- 旋转坐标系中取右手系：+z 沿主磁场，横向平面为 x-y。
- 自由演化：横向磁化以角频率 `2*pi*offset_hz` 绕 **+z 轴右手旋转**
  （正频偏使 +x 转向 +y），并按 `exp(-t/T2)` 指数衰减；
  纵向分量按 `exp(-t/T1)` 向 `equilibrium_mz` 恢复。
  演化使用闭式解，与输出采样间隔无关。
- 射频脉冲：瞬时（不消耗时间），绕横向平面内相位轴
  `u = (cos phase, sin phase, 0)` 按右手定则旋转 `flip` 角度。
  同一时刻的多个脉冲按配置中的输入顺序依次执行；
  该时刻的采样在全部脉冲之后记录。
- 总复信号：各组横向分量按归一化权重（`weight / sum(weights)`）**先相加**，
  再取幅值 `hypot(mx, my)` 与相位 `atan2(my, mx)`；幅值为零时相位输出为空。
- 采样只是记录，不改变演化；重复或乱序的采样时刻会被去重排序，结果不变。

## 配置格式

```json
{
  "spins": [
    {
      "weight": 1.0,
      "initial": [0.0, 1.0, 0.0],
      "offset_hz": 5.0,
      "t1": "inf",
      "t2": 0.8,
      "equilibrium_mz": 1.0
    }
  ],
  "pulses": [
    {"time": 0.1, "phase_deg": 0.0, "flip_deg": 180.0}
  ],
  "samples": {"start": 0.0, "stop": 0.4, "step": 0.001},
  "peak_window": [0.15, 0.25]
}
```

- `spins[].weight`：非负；所有组权重之和必须为正。
- `spins[].initial`：`[mx, my, mz]` 初始磁化（默认 `[0, 1, 0]`）。
- `spins[].offset_hz`：频偏，单位赫兹（默认 0）。
- `spins[].t1` / `spins[].t2`：弛豫时间，单位秒，必须为正；
  字符串 `"inf"` / `"infinity"` 表示无限（无弛豫）。
- `spins[].equilibrium_mz`：纵向平衡磁化（默认 1）。
- `pulses[]`：`time`（秒，>= 0）、`phase_deg`、`flip_deg`（度，默认 0 / 180）。
- `samples`：显式数组 `[t0, t1, ...]`，或 `{"start", "stop", "step"}`（秒）。
- `peak_window`：可选 `[t_start, t_end]`，限定摘要中峰值搜索的采样窗；
  缺省为全部采样。峰值并列时取最早时刻。

## 输出

CSV 每行一个采样时刻：`time_s`，随后每组 `group{i}_mx/my/mz`，
最后 `total_magnitude` 与 `total_phase_rad`（零幅值时相位列为空）。

JSON 摘要包含自旋组数、总权重、采样数、脉冲数、峰值窗，
以及窗内峰值 `peak.time_s` / `peak.magnitude`。
