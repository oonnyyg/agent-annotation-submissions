# plc-scan-replay

离线顺序控制回放器：用**确定的扫描周期**解释输入变化如何影响线圈、
定时器和计数器。它不连接任何真实设备，也不读取真实时钟——每次扫描的
时长全部来自输入记录。

> 本工具实现的是题面定义的扫描子集：布尔触点逻辑、普通线圈、
> 非保持接通延时器（TON）和向上计数器（CTU），并非完整的 PLC 指令集。

## 运行

```sh
go run ./cmd/scanlogic -program examples/program.json -records examples/scans.jsonl
```

输出为 JSONL，每行一次扫描，包含：固定的输入快照、扫描时长、每条语句
执行前后的差异（`changes`），以及该扫描结束时的完整状态。

## 程序格式（program JSON）

```json
{
  "inputs":   ["I_start"],
  "coils":    ["Q_run"],
  "timers":   [{"name": "T1", "preset_ms": 30}],
  "counters": [{"name": "C1", "preset": 3}],
  "statements": [
    {"type": "coil", "target": "Q_run", "condition": {"op": "contact", "name": "I_start"}},
    {"type": "ton",  "timer": "T1",     "condition": {"op": "contact", "name": "Q_run"}},
    {"type": "ctu",  "counter": "C1",
     "condition": {"op": "done", "name": "T1"},
     "reset":     {"op": "contact", "name": "I_start"}}
  ]
}
```

- 条件（`condition` / `reset`）是布尔表达式树：
  - `{"op":"contact","name":"X"}`：触点，引用已声明的输入或线圈；
  - `{"op":"done","name":"F"}`：功能块完成标志，引用已声明的定时器或计数器；
  - `{"op":"and","args":[...]}`、`{"op":"or","args":[...]}`：与 / 或（至少一个参数）；
  - `{"op":"not","args":[x]}`：非（恰好一个参数）。
- 语句按数组顺序组成程序，类型为 `coil`（写普通线圈）、`ton`（调用定时器）、
  `ctu`（调用计数器，必须带 `reset`）。
- 预置值（`preset_ms` / `preset`）必须为正整数。

### 静态校验（回放前拒绝非法程序）

- 所有被引用的名称必须已声明；
- 预置值必须为正整数；
- 同一个可写状态（某个线圈 / 定时器 / 计数器）每次扫描只能被**一条**语句写入，
  即整个程序中同一目标只允许出现一次；
- `ctu` 必须提供 `reset`，其它语句不允许带 `reset`。

## 扫描记录（records JSONL）

每行一条记录，必须给出**完整**的输入集合和正的整数扫描时长（毫秒）：

```json
{"scan": 1, "duration_ms": 10, "inputs": {"I_start": true}}
```

缺少已声明的输入、出现未声明的输入、或 `duration_ms` 非正整数，都会被拒绝。

## 扫描语义

每次扫描：

1. 固定本条记录的外部输入快照；
2. 按语句顺序逐条执行——**不是**一次性并行计算全部语句：
   - 前面语句写入的内部状态对后面的语句**立即可见**；
   - 前面的语句读取"后面才会被更新"的状态时，取到的仍是**旧值**；
3. 输出每条语句执行前后的差异和扫描末状态。

初始状态：所有布尔量为假，计时与计数为零。

### TON（非保持接通延时器）

- 条件为真：累计值加上本扫描时长；达到预置值即置 `done`，累计值**封顶**在预置值；
- 条件为假：累计值立即清零，`done` 复位。

### CTU（向上计数器）

- 只在条件**假到真的边沿**加一，达到预置值后封顶并置 `done`；
- `reset` 为真时清零并复位 `done`，**复位优先于计数**；
- 即使复位为真，边沿记忆也会更新——因此松开复位而条件仍为真时**不会**多计一次。

## 测试

```sh
go test ./...
```

覆盖同扫描依赖（前写后读 / 前读后写）、自保持回路、TON 封顶与清零、
CTU 边沿计数、复位优先级与复位释放边界、非法程序与非法记录的拒绝。

## 目录结构

- `cmd/scanlogic/main.go`：命令行入口；
- `internal/scanlogic/`：程序模型与校验（`program.go`、`validate.go`）、
  扫描执行引擎（`run.go`）及测试；
- `examples/`：示例程序（自保持 + 延时 + 计数）与扫描记录。
