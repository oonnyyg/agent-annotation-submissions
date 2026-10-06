# invoicematch — 本地三方核销工具

在采购订单（Order）、实际收货（Receipt）与供应商发票（Invoice）之间做三方匹配，
让数量差异可追溯，并且**只有已收货且被有效发票占用的数量才计入应付**。

数量为正整数；金额以整数分计算，不含税与汇率。仅使用 Go 标准库。

## 运行

```sh
go run ./cmd/invoicematch -dir <数据目录>
```

从 stdin 读取 JSONL（每行一个操作），向 stdout 输出 JSONL（每行一个结果）。
成功的变更操作会追加到 `<数据目录>/ops.jsonl`；重开程序时自动重放恢复状态。

## 输入操作

| op | 字段 | 说明 |
|---|---|---|
| `create_order` | `order_id, line_id, supplier, item, qty, unit_price` | 订单行：供应商、商品、订购数量、单价（分） |
| `receive` | `receipt_id, order_id, line_id, date, qty` | 收货批次；累计净收货不得超过订购量 |
| `return` | `return_id, receipt_id, qty` | 引用原收货批次退货；只能退尚未被发票占用的数量 |
| `invoice` | `invoice_id, lines:[{order_id,line_id,qty,unit_price}]` | 供应商发票，可含多行 |
| `void_invoice` | `invoice_id` | 冲销整张发票 |
| `query_balance` | `order_id, line_id` | 查询订单行余额 |
| `query_invoice` | `invoice_id` | 查询发票分摊明细（含已冲销的历史） |

## 输出

每个操作输出一行：

```json
{"ok":true,"op":"invoice","id":"INV1","invoice":{...}}
{"ok":false,"op":"return","id":"RET1","error":"..."}
```

- `invoice` / `void_invoice` / `query_invoice` 成功时带 `invoice`：
  `id, supplier, lines, allocs（按收货批次的冻结分摊）, amount（应付分）, voided`。
- `query_balance` 成功时带 `balance`：
  - `ordered` 订购数量
  - `received` 净收货（收货 − 退货）
  - `invoiced` 有效已开票数量（不含已冲销）
  - `uninvoiced` 已收未开票 = `received − invoiced`
  - `unreceived` 未收货 = `ordered − received`
  - `payable` 应付金额（分）= `invoiced × 单价`

## 业务规则

- **收货**：累计净收货（收货 − 退货）不得超过订单行订购量。
- **退货**：只能退原批次中 `qty − returned − invoiced` 的可用数量；
  已被有效发票占用的数量不可退。
- **发票**：所有行必须属于同一供应商，且单价与订单行一致，否则整票拒绝。
  每行按（收货日期，批次编号）顺序分配可开票数量，允许跨批次与部分开票；
  任一行数量不足或非法时整票拒绝，不改变任何余额。
- **核销冻结**：发票成功时冻结分摊明细与应付金额；之后补录更早日期的收货
  不会改变旧发票的分摊（只影响新发票的分配顺序）。
- **冲销**：按原分摊明细释放占用，只释放一次；发票记录保留，仍可查询历史。

## 幂等与持久化

- 同类操作中编号相同且内容完全一致的重复提交返回原始结果（不产生副作用）；
  编号相同但内容不同的提交被拒绝。
- 失败的操作不写入日志、不改变余额，因此收货补齐后可用同一编号重试发票。
- 所有成功变更追加到 `ops.jsonl`，重启后状态完整保留（含幂等记录与冲销历史）。

## 守恒关系

对任意订单行始终成立：

```
ordered   = unreceived + received
received  = uninvoiced + invoiced        （invoiced 仅统计有效发票）
payable   = invoiced × unit_price
每批次:   qty = returned + invoiced(批次占用) + available
```

即：未收货数量永不进入应付；已收未开票数量只体现在 `uninvoiced`，不计入 `payable`。

## 测试

```sh
go test ./...
```

覆盖跨批核销、部分开票、整票拒绝原子性、退货受限、冲销后释放一次与再开票、
补录较早收货不改变旧分摊、幂等重放与重启持久化。`testdata/smoke.jsonl` 为端到端示例。
