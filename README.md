# two-stage-flash-balance

两级闪蒸物料平衡工具：追踪混合进料中各组分在最终产品（`L1`、`V2`、`L2`）
中的分配，并给出可核对的物料收支。仅使用 Python 标准库，不调用物性数据库。

## 假设

- 每级的气液平衡分配系数 `K_i = y_i / x_i` 由输入固定给定（必须为正数），
  不计算温度/压力关系，也不做能量平衡。
- 多股进料按各组分摩尔流量直接相加混合；不允许负流量；全部进料为零时拒绝运行。
- 第一级的液相直接成为产品 `L1`，气相进入第二级；第二级的气、液相分别成为
  产品 `V2`、`L2`。若第一级无气相，第二级输出空流，不会拿原始进料重算。

## 用法

```
python -m flash_balance config.json --streams streams.csv --summary summary.json
```

省略 `--streams` 时 CSV 写到标准输出；省略 `--summary` 且给了 `--streams`
时摘要 JSON 写到标准输出。配置非法或求解失败时以退出码 1 报错，且不写出
任何报告文件。

## 输入格式（JSON）

```json
{
  "feeds": [{"name": "F1", "flows": {"A": 60, "B": 40}}],
  "cases": [
    {"name": "base", "stages": [
      {"k_values": {"A": 2.0, "B": 0.5}},
      {"k_values": {"A": 2.0, "B": 0.5}}
    ]}
  ],
  "tolerance": 1e-12
}
```

- `feeds`：一股或多股进料，`flows` 为组分摩尔流量。
- `cases`：一组或多组分配系数工况，每个工况必须恰好两级；结果按工况名
  排序输出，与输入次序无关。
- `tolerance`：可选，Rachford–Rice 求解精度，默认 `1e-12`。

## 输出

- 物流 CSV：`case, stream, component, molar_flow, mole_fraction`。每股物流
  先给 `TOTAL` 行；空相只输出总量 0，组成留空（不强行归一化）。
- 摘要 JSON：每个工况包含混合进料、逐级状态（`phase` 为
  `liquid`/`vapor`/`two-phase`，或第二级 `skipped-empty-feed`）、气相比例、
  退化标记、迭代次数、两相平衡残差 `max|y_i - K_i x_i|`、逐级与全流程的
  各组分守恒残差、求解精度，以及三股产品的总量与组成。

## 求解方法

每级用 Rachford–Rice 方程求气相比例 psi：
`f(psi) = sum z_i (K_i - 1) / (1 + psi (K_i - 1)) = 0`，在 `(0, 1)` 内对分
求解。`f(0) <= 0` 判全液、`f(1) >= 0` 判全气，端点解归入对应单相；所有
有料组分 `K_i` 均为 1 时按全液处理并标记 `degenerate`。接近 1 的系数不会
造成不收敛（对分法与初值无关）。

## 测试

```
python -m unittest discover -s tests
```
