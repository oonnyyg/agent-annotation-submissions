# closed-network-mva

纯 Python 标准库实现的闭式排队网络平均值分析（Mean Value Analysis, MVA）工具。
评估固定数量任务在服务站之间循环流转时的吞吐与等待，用于比较服务时间调整
方案的收益，不依赖离散事件仿真。

## 模型假设

- 单类任务：所有任务统计特征相同，网络中任务总数恒定（闭式网络）。
- 站点类型：
  - `single`：单服务台，指数服务时间，按 MVA 递推计入排队等待，
    驻留时间 `R_i(n) = S_i * (1 + Q_i(n-1))`。
  - `infinite`：无限服务台（延迟站），无排队，`R_i(n) = S_i` 与人数无关。
- 路由为马尔可夫转移：转移概率每行和为 1，路由图必须强连通。
- 负荷口径：单服务台报告利用率 `U_i = X * v_i * S_i`（不得超过 1）；
  无限服务台报告忙碌服务者均数 `X * v_i * S_i`（可大于 1，不设上限）。

## 访问比归一化

访问比 `v_i` 表示任务每访问参考站一次、平均访问站点 `i` 的次数。
通过直接求解路由平衡线性方程组 `v = v P` 并固定 `v_ref = 1` 得到，
而非幂迭代，因此周期性路由（如确定性两站循环）也能精确求解，
不会因迭代振荡而误判无解。路由图不强连通、行和不为 1 等错误配置
会在报错信息中给出可定位的原因（如 `routing['a']: probabilities sum to ...`）。

## MVA 递推

从零人口开始递推：人口 0 单独给出（吞吐为 0、各站在站数为 0、
驻留时间等于服务时间）；之后对每个 `n = 1..N`：

```
R_i(n) = S_i * (1 + Q_i(n-1))   (single)   R_i(n) = S_i   (infinite)
C(n)   = sum_i v_i * R_i(n)                 # 参考站完整循环时间
X(n)   = n / C(n)                           # 参考站吞吐率
Q_i(n) = X(n) * v_i * R_i(n)                # 各站平均在站数
```

每个方案独立从零人口递推，不共享上一方案的队列状态。输出前自动核对
`sum_i Q_i = N` 以及单服务台利用率不超过 1。

## 输入格式（JSON）

```json
{
  "stations": [
    {"name": "cpu",   "type": "single",   "mean_service_time": 0.5},
    {"name": "think", "type": "infinite", "mean_service_time": 5.0}
  ],
  "routing": {"cpu": {"think": 1.0}, "think": {"cpu": 1.0}},
  "reference_station": "cpu",
  "scenarios": [
    {"name": "baseline", "population": 10},
    {"name": "faster-cpu", "population": 10,
     "service_time_overrides": {"cpu": 0.25}}
  ]
}
```

- `scenarios` 为人口 / 服务时间方案列表；第一个方案为基准，
  其余方案在 JSON 汇总中给出相对基准的吞吐变化
  `throughput_change_vs_baseline`。
- `service_time_overrides` 仅对该方案生效。

完整示例见 `examples/network.json`。

## 运行方法

```sh
python -m closed_mva.cli examples/network.json --csv out/detail.csv --json out/summary.json
```

- CSV 明细：每个（方案 × 人口级别 × 站点）一行，含访问比、驻留时间、
  平均在站数、负荷及参考站吞吐率 / 循环时间。
- JSON 汇总：每个方案在目标人口下的吞吐率、循环时间、相对基准变化、
  各站指标以及在站数总和校验值。

## 测试

```sh
python -m unittest discover -s tests
```
