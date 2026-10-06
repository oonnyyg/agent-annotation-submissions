# battery-pulse-response

电池脉冲负载仿真工具：单支路 Thevenin 等效电路，区分瞬时欧姆压降与缓慢 RC 极化，
并精确定位负载过程中首次触及截止条件的时刻。

## 运行

```
python -m battery_pulse examples/config.json --out out
```

输出（写入 `--out` 目录，每个方案独立从相同初态运行）：

- `<scenario>.csv`：状态轨迹，列为
  `time_s, side, current_a, soc, v_oc_v, v_ohm_v, v_pol_v, v_terminal_v`。
  电流跳变瞬间同时输出 `side=left`（跳变前）与 `side=right`（跳变后）两行。
- `report.json`：每个方案的 `stop_reason`
  （`completed` / `voltage_cutoff` / `soc_depleted`）、`stop_time_s`、
  `final_soc` 与 `net_discharge_ah`（净放电量）。

## 模型

端电压 `V(t) = OCV(soc) - I·R0 - Σ V_pol,k`：

- **OCV**：SOC 在 [0, 1] 上的递增网格分段线性插值，电压必须非降。
- **R0**：串联欧姆电阻（非负），产生换流瞬间的瞬时压降。
- **RC 支路**：若干并联极化支路（R、C 均为正），每段恒流内解析更新
  `V_pol(t+dt) = I·R + (V_pol(t) - I·R)·exp(-dt/τ)`，`τ = R·C`。
  零电流休息时极化按指数继续衰减，状态不冻结。
- **SOC**：库伦积分 `dsoc/dt = -I / (3600·C_Ah)`，放电电流为正。

## 截止判定

- 每次换流先检查右侧（跳变后）端电压，立即截止则停在该瞬间，放电量不增加。
- 段内用解析表达式对 `V(t) = V_min` 做二分求交，跨 OCV 插值节点也精确；
  因此停止时刻与 `sample_dt_s` 采样密度无关。
- SOC 降至 0 时停在边界（`soc_depleted`），不裁剪后继续计算。

## 单位与理想化假设

- 单位：容量 Ah，电流 A，电压 V，电阻 Ω，电容 F，时间 s。
- 恒温：所有参数不随温度变化；不考虑热效应。
- 单支路等效电路：无并联模组、无自放电、无容量衰减、无电流效率损失。
- OCV 仅依赖 SOC（无滞回）；充放电仅通过电流符号区分。

## 测试

```
python -m unittest discover -s tests
```

覆盖：休息期极化恢复、换流瞬间立即截止、段内电压穿越截止、SOC 耗尽、
采样密度不影响停止时刻、参数校验。
