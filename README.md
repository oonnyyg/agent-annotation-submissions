# midi-note-timeline

离线 MIDI 分析器（纯 Go 标准库，无第三方依赖）。把标准 MIDI 文件中的
多音轨事件还原为真实时间轴上的音符区间，输出 CSV 音符表与 JSON 异常
汇总。不播放、不合成任何音频。

## 用法

```sh
go run ./cmd/midinotes [-csv out.csv] [-json out.json] <file.mid>
```

默认输出 `<file.mid>.notes.csv` 与 `<file.mid>.anomalies.json`。

## 支持范围

- 标准 MIDI 文件 **0 型与 1 型**；拒绝 2 型。
- 仅支持 **PPQN**（每四分音符 tick 数）正时基；拒绝 SMPTE 时基。
- 完整解析轨道块与变长整数，支持**运行状态**（running status）。
- 不处理的元事件与系统独占事件（SysEx F0/F7）按声明长度跳过。
- 文件截断或数据越界时返回带**字节偏移**的 `*midi.ParseError`。

## 时间模型

- 所有轨道的事件按 **绝对 tick → 轨道原序号 → 轨内顺序** 稳定合并。
- 默认速度为每四分音符 500000 微秒（120 BPM）；所有轨道的速度事件
  （meta 0x51）共同生效，同一 tick 多个速度事件时**最后一个**决定后续
  速度；tick 到秒通过跨速度段**分段积分**得到。
- 时间精度：tick 为整数；秒数以 `float64` 输出（CSV 保留 9 位小数，
  即纳秒级显示精度），积分按每段 `delta_ticks * micros_per_quarter /
  division` 累加。

## 音符语义

- 音符按 **轨道 + 通道 + 音高** 配对；同一键的多次开启按 **FIFO** 关闭；
  力度为 0 的 note-on 视为 note-off。
- **延音踏板（CC64）** 按轨道和通道独立处理，控制值 ≥ 64 为踩下。踩下
  期间的 note-off 只记录抬键（`key_off_*`），声音延续到踏板松开
  （`sound_end_*`）；延音期间重新开启同音高不会吞掉之前的延音。
- 每个音符输出按键起止与声音终点三组时间（tick 与秒）。

## 异常

- `unmatched_note_off`：没有匹配 note-on 的关闭事件，仅记录异常。
- `unclosed_note`：文件结束仍未关闭的音符，截断到全文件最大结束 tick
  并标记 `unclosed=true`。
- 踏板延音到文件结束仍未释放的音符同样截到文件结束，标记
  `pedal_truncated=true`，**不**计为异常。

## 复音统计

按每轨道统计最大同时发声数：以左闭右开 `[start, end)` 的声音区间
（秒）做扫描线计算，零时长音符不计入。

## 输出

- CSV：`track, channel, pitch, velocity, key_on/key_off/sound_end` 的
  tick 与秒，以及 `unclosed`、`pedal_truncated` 标记。
- JSON：格式、时基、最大 tick、音符数、每轨最大复音、异常列表。

## 测试

```sh
go test ./...
```

覆盖解析（运行状态、截断定位、非法格式拒绝）、变速积分、同 tick 速度
覆盖、重叠同音 FIFO、踏板延音/截断/轨道通道隔离、半开区间复音统计。
