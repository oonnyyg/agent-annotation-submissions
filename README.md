# Gated Particle Tracker

二维粒子观测跟踪工具（仅 Python 标准库，要求 3.12+）。将逐帧、可能漏检
且彼此交叉的位置观测连接成可解释的轨迹，并输出预测与观测之间的对应
关系。

## 用法

```bash
python -m tracker.cli input.json [-o output.json]
python -m unittest discover -s tests   # 运行测试
```

输入 JSON：

```json
{
  "config": {
    "meas_var": 1.0, "accel_var": 0.1, "init_vel_var": 10.0,
    "gate_threshold": 9.0, "miss_penalty": 5.0, "new_penalty": 5.0,
    "max_misses": 3, "max_tracks": 8, "max_observations": 8
  },
  "frames": [
    {"time": 0.0, "observations": [{"id": 1, "x": 0.0, "y": 0.0}]},
    {"time": 1.0, "observations": []}
  ]
}
```

输出 JSON 包含：

- `frames`：逐帧每条活跃轨迹的位置 `(x, y)`、位置协方差（2x2）、
  连续漏检数 `misses`；`associations` 给出轨迹与观测编号的对应关系
  及匹配代价（新建轨迹代价为 `null`）；`new_track_ids` /
  `ended_track_ids` 给出本帧新建与结束的轨迹。
- `summary`：每条轨迹的起止帧与长度。

## 输入校验

- 帧时间必须严格递增；允许空帧（无观测）。
- 每帧观测编号唯一；坐标必须为有限数。
- `meas_var` 必须为正；`accel_var`、`init_vel_var` 非负；所有配置
  数值必须有限。
- 每帧观测数与同时活跃的轨迹数均不得超过 8（可用
  `max_observations` / `max_tracks` 调整）；超限即以非零退出码
  明确失败，绝不静默丢弃。

## 模型与噪声约定

- 状态为 `[x, vx, y, vy]`，两轴独立滤波，每轴为匀速（CV）模型。
- 时间间隔 `dt` 内的状态转移 `F = [[1, dt], [0, 1]]`；过程噪声为
  连续白噪声加速度模型：
  `Q = accel_var * [[dt^3/3, dt^2/2], [dt^2/2, dt]]`，两轴加速度
  噪声相互独立、方差同为 `accel_var`。
- 测量只含位置，测量噪声方差为 `meas_var`（两轴相同）。
- 预测与协方差均按真实时间差 `dt` 计算；新轨迹速度为零，位置方差
  取 `meas_var`，速度方差取 `init_vel_var`。

## 关联与门控

- 每个“轨迹–观测”候选对计算创新协方差 `S = H P H^T + R` 下的
  马氏距离平方；超过 `gate_threshold` 的关联被禁止。
- 其余关联通过匈牙利算法求全局一对一最小总代价指派（非贪心）：
  匹配代价为距离平方，每条未匹配轨迹加 `miss_penalty`，每个未匹配
  观测加 `new_penalty`。
- 匹配后执行卡尔曼更新；未匹配观测创建新轨迹；未匹配轨迹保留
  预测结果并累计漏检，连续漏检达到 `max_misses` 时结束。

## 确定性与并列规则

- 每帧观测先按编号排序（数值编号在前、按数值升序，字符串编号在
  后、按字典序），轨迹按轨迹编号升序处理，因此同帧观测重排不会
  改变结果。
- 指派代价并列时，优先选择轨迹编号最小、其次观测编号最小的
  方案（字典序最小指派向量）。
- 新轨迹按排序后的观测顺序依次分配递增的轨迹编号。
