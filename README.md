# periodic-pair-distribution

粒子轨迹结构分析工具：在周期边界条件下统计不同组分的径向分布函数
（RDF）与累计配位数（CN）。仅依赖 Python 标准库，通过逐帧归一化
避免盒子体积变化带来的归一化偏差。

## 用法

```
python -m pair_distribution CONFIG.json
```

### 配置（JSON）

```json
{
  "trajectory": "traj.jsonl",
  "pairs": [["A", "B"], {"a": "A", "b": "A"}],
  "r_max": 5.0,
  "n_bins": 100,
  "output_csv": "pair_distribution.csv",
  "summary_json": "summary.json"
}
```

- `pairs`：种类对。`["A", "B"]` 以 A 为中心统计 B；`["A", "A"]`
  为同种统计，排除自身并按**有序对**计数（即双向双计数）。
- `r_max`：最大半径，不得超过**任一帧**最短盒边的一半，否则报错。
- `n_bins`：径向区间数，等宽划分 `[0, r_max)`。

### 轨迹（JSONL，每行一帧）

```json
{"box": [10.0, 10.0, 10.0],
 "particles": [{"id": 1, "type": "A", "position": [0.1, 5.0, 5.0]}, ...]}
```

- 盒子为正交盒，`box` 三个边长必须为正，否则拒绝该帧。
- 粒子 `id` 与 `type` 集合在所有帧中必须一致，否则报错。
- 坐标允许落在主盒外；距离一律使用最小镜像约定。

## 归一化约定

径向区间左闭右开 `[lo, hi)`，`r == r_max` 不计入。

对每一帧、每个区间 k：

```
g_k = count_k / (N_center * rho_target * V_shell(k))
```

- `V_shell(k) = 4π/3 * (hi³ - lo³)`，精确球壳体积。
- 异种对：`rho_target = N_b / V_frame`。
- 同种对：`rho_target = (N_a - 1) / V_frame`（中心以外的粒子数）。
- 配位数：`CN_k = cumulative_count_k / N_center`，即每个中心在
  半径内的平均邻居数。

所有量**逐帧计算**，再对帧做等权平均；不会先把不同体积帧的计数
混合再归一化，因此帧顺序变化不影响平均结果。任一种类缺失或同种
粒子不足两个时，该帧分布为空（g 为 0）且配位数为 0。

## 单位

长度单位与输入坐标/盒长一致；`g(r)` 无量纲；配位数为粒子数。
CSV 中 `r_lo`、`r_hi` 使用该长度单位。

## 输出

- **CSV**：每对种类输出 `frame = mean` 的汇总行（`count` 为各帧
  原始计数之和，`g_r`/`coordination` 为等权平均），以及每帧
  （`frame = 0, 1, ...`）的原始计数与逐帧 g、CN。
- **摘要 JSON**：帧数、粒子数、种类列表、`r_max` 上限校验值、
  归一化约定说明及每对种类的总计数与空帧信息。

## 测试

```
python -m unittest discover -s tests
```

覆盖跨周期边界距离、变盒长轨迹的等权平均、同种有序对双计数、
直方图左闭右开边界与 `r_max` 排除、缺失种类/单粒子空分布、
非正盒长与粒子不一致的拒绝等。
