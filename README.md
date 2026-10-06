# two-stage-flash-balance

两级闪蒸物料平衡工具：追踪混合进料中各组分在两级分离后的去向，
输出每股物流的流量与组成，并给出可核对的物料收支。
仅使用 Python 标准库，不调用任何物性数据库。

## 运行

```
python -m flash_balance config.json --outdir out
```

测试：`python -m unittest discover -s tests`

## 输入（JSON）

```json
{
  "feeds": [{"methane": 30.0, "ethane": 20.0}, {"methane": 10.0, "propane": 40.0}],
  "cases": [
    {"name": "base",
     "stage1_k": {"methane": 4.0, "ethane": 1.5, "propane": 0.6},
     "stage2_k": {"methane": 5.0, "ethane": 2.0, "propane": 0.8}}
  ]
}
```

- `feeds`：一股或多股进料，数值为各组分摩尔流量。多股进料按组分相加混合；
  负值或非有限值被拒绝；全部进料为零时报错且不产生任何输出。
- `cases`：一组或多组工况，每组给出两级各自的平衡分配系数 `K_i = y_i / x_i`。
  所有 K 值必须为正数。工况可任意排列，输出按工况名排序，结果与次序无关。

## 模型假设

- K 值由输入固定给定，不计算温度-压力关系，不做能量平衡。
- 每级用 Rachford–Rice 方程求气相比例 ψ：端点解（RR(0) ≤ 0 或 RR(1) ≥ 0）
  分别归入全液、全气单相；否则在 (0, 1) 内对分求解（严格单调，必然收敛）。
- 所有有料组分的 K 均为 1 时相分裂不可确定，按全液处理并在摘要中标记
  `degenerate: true`。
- 第一级液相直接为产品 `stage1_liquid`；气相进入第二级。第二级的液、气相分别
  为产品 `stage2_liquid`、`stage2_vapor`。第一级无气相时第二级输出空流，
  不会拿原始进料重新计算。

## 输出

写入 `--outdir`（默认当前目录）：

- `streams_<工况名>.csv`：每股产品物流一行，含 `total_flow`、各组分摩尔流量
  `n_<组分>` 与摩尔分数 `x_<组分>`。空相（总流量为零）的组成留空，不强行归一化。
- `summary.json`：各工况求解摘要，包括每级相态、气相比例、迭代次数、
  Rachford–Rice 残差（`rr_residual` / `equilibrium_residual`）、逐组分
  物料收支（`balance`：feed / product / residual）及最大相对收支残差
  `max_balance_residual`，以及警告信息。

任何非法配置或求解失败都会以非零退出码终止，且不写出任何报告文件，
避免产生伪完整结果。
