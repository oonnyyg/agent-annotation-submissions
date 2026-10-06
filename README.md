# closed-network-mva

纯 Python 标准库实现的闭式排队网络平均值分析（Mean Value Analysis, MVA）工具。
用于评估固定数量任务在服务站之间循环流转时的吞吐率与等待时间，便于比较
服务时间调整带来的收益。不使用离散事件仿真，也不依赖任何第三方包。

## 模型假设

- 单类任务：所有任务统计特征相同，网络中任务总数固定（闭式网络）。
- 站点类型：
  - `single`：单服务台，指数服务时间，FCFS，计入排队等待；
  - `infinite`：无限服务台（延迟站），驻留时间恒等于平均服务时间，不随人数增加。
- 平均服务时间必须为正数。
- 路由为马尔可夫转移矩阵：每行概率非负且和为 1，整个路由图必须强连通，
  保证访问比方程有唯一正解。
- 乘积形式网络假设：指数服务 + 单/无限服务台使 MVA 精确成立。

## 访问比归一化

访问比 v 满足平衡方程 `v = v · P`（P 为路由矩阵），并以参考站归一化：
`v[reference_station] = 1`。求解采用“惰性链”幂迭代
`v ← v·(P + I)/2`，与普通幂迭代有相同的不动点，但对周期性路由
（例如两站确定性地来回跳转）也能收敛，不会因振荡而误判无解。

## MVA 递推

从零人口 `N = 0`（所有站平均在站数为 0，单独在输出中给出）递推到目标人口：

- 单服务台：`R_i(n) = S_i · (1 + Q_i(n-1))`
- 无限服务台：`R_i(n) = S_i`
- 参考站吞吐率：`X(n) = n / Σ_i v_i · R_i(n)`
- 平均在站数：`Q_i(n) = X(n) · v_i · R_i(n)`
- 完整循环时间：`C(n) = n / X(n)`

输出前核对 `Σ_i Q_i = N`。负荷指标：

- 单服务台：`load = X·v_i·S_i` 为利用率，数学上不超过 1（工具内部带容差校验）；
- 无限服务台：`load` 为平均忙碌服务者数，只表示负荷大小，不设上限。

## 多方案比较

`scenarios` 中的每个方案有独立的人口与可选的 `service_time_overrides`。
每个方案都从零人口独立递推，不共享上一方案的队列状态。
第一个方案为基准，JSON 汇总中的 `relative_throughput` 为相对基准的吞吐比。

## 输入格式（JSON）

```json
{
  "reference_station": "cpu",
  "stations": [
    {"name": "cpu", "type": "single", "mean_service_time": 0.5},
    {"name": "think", "type": "infinite", "mean_service_time": 5.0}
  ],
  "routing": [[0.0, 1.0], [1.0, 0.0]],
  "scenarios": [
    {"name": "baseline", "population": 10},
    {"name": "faster-cpu", "population": 10,
     "service_time_overrides": {"cpu": 0.25}}
  ]
}
```

`routing[i][j]` 是任务离开站 i 后前往站 j 的概率，行顺序与 `stations` 一致。
配置错误（行和不为 1、图不强连通、未知站名等）会给出可定位的错误信息。

## 运行方法

```bash
python -m closed_network_mva examples/network.json --csv details.csv --json summary.json
```

- CSV：每个方案 × 每个站一行的性能明细（访问比、驻留时间、平均在站数、负荷等）。
- JSON：每个方案的吞吐率、循环时间、相对基准吞吐比、零人口状态及各站明细。

## 测试

```bash
python -m unittest discover -s tests
```
