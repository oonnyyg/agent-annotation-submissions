# transit-fare-capping

本地公共交通计价工具：把换乘优惠与日/周封顶串联，撤销乘车记录后按有效记录完整重算扣费。仅使用 Go 标准库，金额一律为整数分。

## 运行

```sh
go run ./cmd/farecap -config config.json -state state.json -input input.jsonl
```

- `-config`：费率配置 JSON（默认 `config.json`）。
- `-state`：状态文件（默认 `state.json`），存在则加载，退出前保存；重开后结果一致。
- `-input`：JSONL 输入（缺省读标准输入）。每行一条指令，每行输出一个 JSON 结果。

## 配置（可自定义规则）

```json
{
  "utcOffsetMinutes": 480,
  "transferWindowMinutes": 90,
  "fares": {"1": 100, "2": 150, "3": 200, "4": 250},
  "dailyCapCents": 400,
  "weeklyCapCents": 1000
}
```

- `utcOffsetMinutes`：固定 UTC 偏移（分钟），不引入夏令时。
- `transferWindowMinutes`：换乘时限（分钟）。
- `fares`：按覆盖区间宽度（起终区号最大减最小再加一）索引的票价表（分），必须单调非降；缺少所需票价时拒绝操作。
- `dailyCapCents` / `weeklyCapCents`：非负的服务日/服务周封顶（分）。

规则要点：

- 服务日从本地时间 04:00 开始；服务周从周一 04:00 开始。
- 同服务日内，后续上车距当前换乘组首趟上车严格小于换乘时限时并入该组；窗口不随每次换乘延长，跨服务日必开新组。
- 组票价按组内全部起终区号的最大减最小再加一查询；每组增量为新旧组票价之差，再受当前日与周剩余额度共同限制，实际扣费同时累计到两级封顶。
- 记录按上车时间再按记录编号排序；同卡行程重叠拒绝；相同编号同内容重提幂等，内容冲突拒绝且不改旧记录。
- 撤销后按该卡有效记录完整重算，保留撤销历史，不直接退回旧扣费。

## 输入指令（JSONL）

```jsonl
{"type":"ride","id":"r1","card":"c1","board":"2026-10-05T08:00:00","alight":"2026-10-05T08:20:00","from":1,"to":1}
{"type":"void","id":"v1","card":"c1","target":"r1"}
{"type":"query","card":"c1"}
```

- `ride`：乘车记录，含卡号、上下车时间（`YYYY-MM-DDTHH:MM:SS`，配置偏移的本地时间）和起终区号（连续正整数）。
- `void`：撤销指定编号的乘车记录。
- `query`：查询某卡的扣费流水与累计扣费。

## 输出

每行一个 JSON：`status` 为 `ok` / `duplicate` / `rejected`（附 `reason`）；`ride` 返回本次 `chargedCents` 与卡累计 `cardTotalCents`；`query` 返回完整 `charges` 流水（含服务日/服务周）。

## 测试

```sh
go test ./...
```
