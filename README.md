# quadratic-pipe-network

小型供水管网稳态水力计算程序，仅使用 Python 标准库。从 JSON 配置读取管网与
用水工况，联立求解未知节点水头与管段流量，批量比较不同工况，并以 CSV / JSON
输出结果。不考虑泵、阀门与瞬态水击。

## 模型与量纲

- 节点分两类：固定水头边界节点（水库/水塔）与给定净取水量的需求节点。
  需求为正表示取水，为负表示注入。
- 管段有指定正方向（`from` → `to`）与正阻力系数 `r`。沿正方向满足
  `h_from - h_to = r · q · |q|`；`q < 0` 表示实际流向与正方向相反，
  求解器不会把反向或零流量强行改成正数。
- 量纲自洽即可：水头用米（或任意长度单位），流量用 L/s（或任意流量单位），
  阻力系数量纲为 `水头 / 流量²`。
- 允许回路和平行支路；拒绝自环管段，以及没有固定水头参照的连通分量
  （加载时报错退出）。

## 求解与收敛判断

未知量为全部需求节点水头与全部管段流量，用 Newton 法联立求解（解析雅可比，
高斯消元）。每次迭代计算两类残差（均取最大绝对值）：

- 节点残差：各需求节点的流量守恒不平衡量（流量单位）；
- 管段残差：各管段水头关系的偏差（水头单位）。

只有两者同时不超过各自容限（`--node-tolerance`、`--pipe-tolerance`，
默认 `1e-9`）才判定收敛。达到 `--max-iterations`（默认 50）、雅可比奇异或
出现非有限值时判定失败：报告原因与最终残差，且不输出任何水头/流量结果，
失败工况不会冒充成功。

## 输入格式

管网文件（见 `examples/network.json`）：

```json
{
  "fixed_heads": { "R1": 100.0 },
  "pipes": [
    { "name": "main-a", "from": "R1", "to": "J1", "resistance": 1.5 }
  ]
}
```

工况文件（见 `examples/scenarios.json`）：`scenarios` 数组，每项含 `name`
与 `demands`（节点 → 净取水量，缺省节点为零）。各工况独立求解，方案顺序
不影响任一方案的结果。

## 运行与复现

```sh
python -m pipenet.cli examples/network.json examples/scenarios.json \
    --pipes-csv out/pipes.csv --nodes-json out/nodes.json
```

输出：

- 管段 CSV：`scenario, pipe, flow, converged`；
- 节点 JSON：每个工况的收敛状态、迭代数、两类残差、全部节点水头、
  各边界节点的净供水量 `boundary_supply`，以及总量平衡校验
  （`total_supply`、`total_demand`、`balance_error`）。

任一工况失败时进程退出码为 1，输入非法时为 2。

运行测试：

```sh
python -m unittest discover -s tests -v
```

测试覆盖树形网络、闭环分流、反向流动、双容差收敛判断、超限失败报告、
自环/无固定水头分量校验，以及批量工况的顺序无关性。
