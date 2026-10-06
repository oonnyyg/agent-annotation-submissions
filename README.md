# coincidence_replay — 双通道探测器回放工具

用 Python 标准库实现，读取脉冲 CSV 与 JSON 采集配置，回放死时间行为并做两路符合配对，输出逐脉冲状态、符合对以及计数与活时间汇总。

## 用法

```sh
python -m coincidence_replay pulses.csv config.json -o replay_out
```

输出目录包含：

- `pulses.csv`：逐脉冲状态（是否接收、校时后时间、配对对象、未配对/拒收原因）
- `pairs.csv`：每个符合对的双方事件编号、校时后时间与时间差 `diff_ns`
- `summary.json`：各通道输入/接收计数、忙时间、活时间、计数率与符合对总数

重复回放同一输入，输出逐字节一致（确定性排序，无随机因素）。

## 输入格式

脉冲 CSV 表头为 `channel,event_id,time_ns`。时间统一为整数纳秒；事件编号全局唯一，重复编号或落在采集区间（左闭右开 `[start_ns, end_ns)`）之外的事件会报错。每个通道内按 `(time_ns, event_id)` 字典序排序后回放。

配置 JSON 示例：

```json
{
  "acquisition": {"start_ns": 0, "end_ns": 1000000},
  "window_ns": 20,
  "channels": {
    "A": {"dead_time_ns": 100, "model": "nonparalyzable", "time_offset_ns": 0},
    "B": {"dead_time_ns": 50, "model": "extendable", "time_offset_ns": -5}
  }
}
```

各通道独立配置非负死时间与校时偏移；`model` 为 `nonparalyzable`（不可延长）或 `extendable`（可延长）。

## 死时间模型

- 空闲时到达的脉冲被记录（接收）并触发死区 `[t, t + dead_time_ns)`。
- 不可延长：死区内被拒的脉冲不改变死区结束时间。
- 可延长：每个到达脉冲（包括被拒的）都把死区结束延至自身时间加死时间。
- 恰好到达死区结束时刻（`t == dead_until`）的脉冲可接收。

忙时间按**死区并集**与采集区间的交集计算，而不是把每次死时间简单相加；活时间 = 总时长 − 忙时间。`summary.json` 中输入计数率以总时长为分母，接收计数率以活时间为分母；活时间为零时接收率为 `null`。

## 符合配对

接收脉冲先加各通道校时偏移，落在采集区间外的不再参与符合。随后**依次比较两路最早未用脉冲**：时间差绝对值不超过窗口 `window_ns`（含边界）即配对，否则丢弃较早者；每个脉冲只能用一次。

注意这是贪心顺序配对，**不是最近邻搜索**：即使某个脉冲存在时间上更近的对方脉冲，只要队首比较落在窗口内就先配对。未配对脉冲在 `pulses.csv` 的 `note` 列标记 `unmatched_no_partner`；其他原因包括 `rejected_dead_time`（死区拒收）与 `out_of_range_after_offset`（校时后出界）。

## 测试

```sh
python -m unittest discover -s tests -v
```

覆盖同刻脉冲、延长死区、校时边界、死区并集、零活时间与重复回放一致性。
