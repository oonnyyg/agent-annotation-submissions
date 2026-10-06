# invoicematch — 采购三方核销工具

本地命令行工具，核对**订单（PO）— 收货（GR）— 供应商发票（INV）**三方数量，
只把已收货且被有效发票占用的数量计入应付。纯 Go 标准库实现。

## 运行

```sh
go run ./cmd/invoicematch -dir <状态目录> < ops.jsonl
```

- 从 stdin 逐行读取 JSONL 操作，每行输出一个 JSON 响应到 stdout。
- 状态持久化在 `<状态目录>/ops.jsonl`（追加日志），重开程序自动重放恢复。
- 数量为正整数；金额为整数分（不含税、不含汇率）。

示例输入见 `examples/demo.jsonl`。

## 操作

所有操作都带 `op` 与 `id` 字段。响应为 `{"ok":bool,"id":...,"error":...,"result":...}`。

### create_order — 创建订单
```json
{"op":"create_order","id":"PO1","supplier":"SUP1","lines":{"1":{"sku":"WIDGET","qty":100,"unit_price":250}}}
```
订单行确定供应商、商品（sku）、订购数量和单价（分）。

### receive — 收货
```json
{"op":"receive","id":"R1","order_id":"PO1","line":1,"qty":50,"date":"2026-01-15"}
```
生成一个收货批次。同一订单行累计净收货（收货−退货）不得超过订购量。

### return — 退货
```json
{"op":"return","id":"RET1","receipt_id":"R1","qty":5}
```
引用原收货批次，只能退**尚未被有效发票占用**的数量。

### invoice — 发票核销
```json
{"op":"invoice","id":"INV1","supplier":"SUP1","lines":[{"order_id":"PO1","line":1,"qty":60,"unit_price":250}]}
```
- 一张发票可含多个订单行，但必须属于同一供应商，且单价与订单一致。
- 每个订单行按**收货日期升序、再按批次号升序**分配可开票数量，允许跨批次与部分开票。
- 任一行非法（供应商不符、单价不符、数量不足等）则**整票拒绝**，不改变任何余额。
- 成功时冻结分摊明细（`allocations`）与应付金额 `amount`（分）。之后补录更早日期的收货不会改变已冻结的旧分摊。

### void_invoice — 冲销发票
```json
{"op":"void_invoice","id":"V1","invoice_id":"INV1"}
```
按原分摊明细释放占用，且只释放一次（重复冲销报错）。发票记录保留为历史，仍可查询。

### balance — 查询订单行余额
```json
{"op":"balance","id":"q1","order_id":"PO1","line":1}
```
返回：

| 字段 | 含义 |
|---|---|
| `ordered` | 订购数量 |
| `received_net` | 净收货（收货−退货） |
| `unreceived` | 未收货数量 |
| `received_uninvoiced` | 已收未被有效发票占用 |
| `invoiced_valid` | 有效已开票数量 |
| `payable_cents` | 应付金额（分）= `invoiced_valid × unit_price` |

### invoice_detail — 查询发票分摊明细
```json
{"op":"invoice_detail","id":"q2","invoice_id":"INV1"}
```
返回冻结的分摊明细、金额与 `voided` 状态（冲销后仍可查）。

## 幂等

同一 `id` 且内容完全相同的重复提交返回**原结果**（包括失败结果）；
同一 `id` 内容不同则拒绝。查询操作（`balance`、`invoice_detail`）只读，不缓存、不持久化。

## 守恒关系

对任意订单行，任意时刻满足：

```
ordered == unreceived + received_uninvoiced + invoiced_valid
received_net == received_uninvoiced + invoiced_valid
```

即：未收货、已收未开票、有效已开票三段互不重叠且恰好覆盖订购量；
未到货数量永远不会进入 `invoiced_valid` 与应付金额。

## 测试

```sh
go test ./...
```

覆盖跨批次核销、补录较早收货不改变旧分摊、冲销后再开票、退货受发票占用限制、
超量收货拒绝、整票原子拒绝、幂等重放与重启持久化。
