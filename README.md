# quadratic-pipe-network

稳态二次阻力管网水力计算程序，仅依赖 Python 标准库（>= 3.12）。
不考虑泵、阀门与瞬态水击。

## 模型

- 节点分两类：固定水头边界（`head` 给定）与自由节点（`demand` 给定净取水量，
  正值表示取水，负值表示注入）。
- 管段有声明的正方向（`from` -> `to`）与正阻力系数 `resistance`。
  沿正方向的水头差满足 `H_from - H_to = r * q * |q|`。
  `q < 0` 表示实际流向与声明方向相反，程序不会把反向或零流量强行改为正数。
- 允许回路和平行支路；拒绝自环，拒绝缺少固定水头参照的连通分量。
- 量纲自洽即可：若流量取 m³/s、水头取 m，则阻力系数量纲为 s²/m⁵；
  程序本身不假设具体单位。

## 求解

联立全部未知水头（自由节点）与全部管段流量，用阻尼 Newton 法求解：

- 节点方程：自由节点流入 - 流出 - 净取水量 = 0
- 管段方程：`H_from - H_to - r * q * |q| = 0`

收敛需同时满足 `max|节点残差| <= --tol-node` 且 `max|管段残差| <= --tol-pipe`，
并在 `--max-iter` 次迭代内完成。奇异雅可比、非有限步长、线搜索失败或
超出迭代上限都会报告原因与当前残差，且该工况不输出水头/流量结果，
不会把最后一次迭代包装成有效解。

## 输入格式

网络文件（见 `examples/network.json`）：

```json
{
  "nodes": [{"id": "tank", "head": 100.0}, {"id": "j1", "demand": 0.0}],
  "pipes": [{"id": "p1", "from": "tank", "to": "j1", "resistance": 0.5}]
}
```

工况文件（见 `examples/scenarios.json`）：`demands` 覆盖网络文件中的默认值，
同一网络可批量计算多个独立负荷方案，方案顺序不影响各自结果。

```json
{"scenarios": [{"name": "peak", "demands": {"j1": 8.0}}]}
```

## 运行与复现

```bash
python -m pipenet examples/network.json examples/scenarios.json \
    --pipes-out out/pipes.csv --nodes-out out/nodes.json \
    --max-iter 50 --tol-node 1e-8 --tol-pipe 1e-8
```

- `pipes.csv`：每个成功工况的管段流量、实际方向（forward/reverse/zero）与水头损失。
- `nodes.json`：每个工况的收敛状态、残差、节点水头、各固定水头边界的净供水量、
  总供水量与总取水量的平衡误差。失败工况只有状态与残差，退出码为 1。

## 测试

```bash
python -m unittest discover -s tests -v
```

覆盖树形网络解析解、闭环分流、注入导致的反向流动、校验拒绝（自环、
无固定水头分量、负阻力）、超限不冒充成功，以及批量工况的顺序无关性。
