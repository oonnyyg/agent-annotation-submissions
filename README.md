# detector-coincidence-replay

双通道探测器回放工具：分析死时间漏掉哪些脉冲，以及校时后真正能配成多少次符合事件。仅依赖 Python 标准库。

## 用法

```bash
python -m coincidence_replay --pulses pulses.csv --config config.json --out-dir replay_out
```

输出三个文件到 `--out-dir`：

- `pulse_status.csv`：逐脉冲状态（`accepted` / `rejected` 及原因）
- `coincidence_pairs.csv`：符合对（含每对校时后时间差 `delta_ns`）与未配对脉冲及原因
- `summary.json`：各通道输入/接收计数、忙时间、活时间与计数率，以及符合对数

重复回放同一输入，输出完全一致。

## 输入格式

脉冲 CSV（时间统一为整数纳秒，编号全局唯一）：

```csv
pulse_id,channel,time_ns
a1,A,100
b1,B,105
```

配置 JSON：

```json
{
  "acquisition": {"start_ns": 0, "end_ns": 1000000},
  "coincidence_window_ns": 20,
  "channels": {
    "A": {"dead_time_ns": 100, "model": "nonparalyzable", "time_offset_ns": 0},
    "B": {"dead_time_ns": 50, "model": "extendable", "time_offset_ns": -5}
  }
}
```

## 规则说明

- 采集区间为左闭右开 `[start_ns, end_ns)`；区间外事件或重复编号直接报错。
- 通道内脉冲按 (时间, 编号字典序) 排序后回放；各通道独立配置非负死时间。
- 死区为半开区间 `[触发时刻, 触发时刻+死时间)`，恰好到结束时刻的脉冲可接收。
  - `nonparalyzable`（不可延长）：被拒脉冲不改变死区。
  - `extendable`（可延长）：每个到达脉冲都把死区结束延至自身时间加死时间。
- 忙时间 = 死区**并集**与采集区间的交集长度，不是每次死时间的简单相加；
  活时间 = 总时长 − 忙时间。输入计数率以总时长为分母，接收计数率以活时间为分母；
  活时间为零时接收计数率为 `null`。
- 接收后再加各通道校时偏移 `time_offset_ns`；校时后落在采集区间外的脉冲不参与符合。
- 符合配对是**贪心依次比较**，不是最近邻搜索：比较两路最早未用脉冲，
  时间差绝对值不超过窗口即配对，否则丢弃较早者；每个脉冲只能用一次。
  未配对原因：`no_partner_in_window`（窗口内无对象被丢弃）、
  `channel_exhausted`（对方通道已无可配脉冲）。

## 测试

```bash
python -m unittest discover -s tests
```
