# 计件结算工具 (piecepay)

面向加工班组的计件结算命令行工具。按 JSONL 从标准输入接收操作，逐行向标准输出返回 JSON，
全部状态保存在指定目录的 `state.json` 中，重开后查询与幂等判定结果一致。

## 运行方式

```sh
go run ./cmd/piecepay -dir <状态目录>
```

标准输入每行一个 JSON 操作（空行忽略），每个操作输出一行 JSON：
成功时 `"ok":true`，失败时 `"ok":false` 并带 `error` 说明。
失败的操作不会修改任何状态。

## 操作一览

### register_scheme — 登记/更新计价方案

```json
{"op":"register_scheme","scheme_id":"s1","tiers":[{"up_to":100,"rate_cents":10},{"up_to":200,"rate_cents":15},{"rate_cents":20}]}
```

- `tiers` 为累进阶梯，`up_to` 为该档累计件数上限（整数件），最后一档可省略表示上不封顶；
  `rate_cents` 为该档单价（整数分/件）。
- 同一 `scheme_id` 再次登记即追加一个新版本，返回 `scheme_version`。
  已登记批次永远保留当时的版本快照，方案更新不影响历史分配。

### register_batch — 登记生产批次

```json
{"op":"register_batch","batch_id":"b1","scheme_id":"s1","month":"2026-09","qty":101,"members":{"a01":1,"a02":1,"a03":1}}
```

- 批次总额按合格整数件数 `qty` 独立套用累进阶梯：每一档只对落在该档区间内的件数计价，
  不会把最后一档价格乘全部件数。
- `members` 为成员编号到正整数权重的映射。分配规则：先按权重取各人份额的整数部分，
  剩余分按余数降序逐个分完，余数相同按员工编号字典序；个人金额之和恒等于批次总额。
- `batch_id` 唯一：相同内容重提返回原结果（`"duplicate":true`），内容不同则拒绝；
  登记失败不会留下部分成员收入，也不会占用编号。
- 目标月份已封账时拒绝登记。

### void_batch — 整批撤销

```json
{"op":"void_batch","batch_id":"b1","month":"2026-10"}
```

- 冲销记入指定的未封账月份 `month`（可与原批次月份不同，用于跨月撤销）。
- 逐人冲销金额为登记时份额快照取负，不按当前权重或价格重算。
- 同一批次只能撤销一次。

### close_month — 月份封账

```json
{"op":"close_month","month":"2026-09"}
```

封账后该月不能新增批次，也不能作为冲销入账月份；重复封账会被拒绝。

### query_month — 查询月度结算

```json
{"op":"query_month","month":"2026-10"}
```

返回 `report`：

- `members`：每人当期 `added`（新增）、`reversed`（冲销，负数）与 `net`（净额），单位分；
- `added` / `reversed` / `net`：全班组汇总；
- `entries`：当期每笔流水，`kind` 为 `add` 或 `reversal`，并携带
  `batch_id`、`scheme_id`、`scheme_version`、`batch_month`，可追查原批次和计价版本。

## 自定义业务规则

本工具内置的结算口径如下，如需调整请修改对应位置：

- 阶梯计价与校验：`cmd/piecepay/calc.go` 的 `priceQty` / `validateTiers`；
- 权重分币规则（整数部分 + 余数降序 + 编号字典序）：`cmd/piecepay/calc.go` 的 `splitShares`；
- 幂等判定口径（方案、月份、件数、成员权重完全一致才算同一请求）：
  `cmd/piecepay/ops.go` 的 `batchFingerprint`；
- 撤销口径（快照取负、不重算、不重复）：`cmd/piecepay/ops.go` 的 `opVoidBatch`。

## 测试

```sh
go test ./...
```

覆盖阶梯边界（含"最后一档不乘全部件数"）、分币余数与平局次序、
方案版本隔离、幂等重提、封账限制、跨月撤销与重开一致性。
