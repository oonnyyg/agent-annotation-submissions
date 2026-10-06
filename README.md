# 盘点工作台（stocktake-adjustment-desk）

纯 Go 标准库实现的商品库位库存与盘点工作台。所有数据以 JSONL 追加事件流
持久化在指定目录（`events.jsonl`），每次命令启动时重放事件还原状态，
因此进程重启后盘点状态、封存明细和调整记录全部保留。

运行：

```
go run ./cmd/stocktake <command> [flags]
go test ./...
```

所有数量均为非负整数；出库若导致当前库存为负会被整笔拒绝。命令输出为 JSON，
校验失败时错误打印到 stderr 并以非零码退出。

## 冻结时点假设（核心）

- `start` 选定一个库位并把**当时该库位的账面数量整体快照为基线**。
- 每条清点记录的数量都表示**这一冻结时点**的实盘数量，而不是录入时刻数量。
- 盘点期间出入库照常发生，只改变“当前库存”，**绝不改变冻结基线**。
- 因此封存差异 = 实盘（冻结时点） − 基线（冻结时点），与盘点期间的
  出入库流水完全独立；入账时把该差异**追加**到当前库存，而不是用实盘量
  直接替换当前值。这保证盘点期间发生的出入库不会被盘点结果覆盖。
- 同一库位不能同时属于两个未结束（open）盘点；盘点封存后该库位即解锁，
  可开启新的盘点（用于“正常重开”，历史盘点及其调整记录仍然保留）。

## 清点记录规则

- 每条记录有全局唯一编号 `--record`，含库位、商品、数量。
- 相同编号 + 完全相同内容（库位/商品/数量）可重复导入，按幂等处理，
  数量不重复累加；相同编号但内容不同则拒绝，避免覆盖既有依据。
- 不同编号**不会**按商品自动去重：同一商品的多条记录数量相加。
- 未封存的记录可 `revoke` 撤销，撤销后不再参与合计（撤销本身也可幂等重复）。
- 已封存/已入账的盘点不能再新增或撤销记录。

## 封存与入账

- 每个库位必须先 `complete` 明确确认清点完成，才能 `seal`。
- 封存时取“基线商品 ∪ 实盘商品”的并集：基线中有、但没有存活清点记录的
  商品按**实盘为 0**处理（漏盘归零）；实盘多出的商品基线按 0。
- 封存会冻结逐商品明细（baseline/counted/diff）与差异合计，之后不可修改。
- `post` 入账把全部差异加到当前库存。任何商品在入账后会变成负数时，
  **整次拒绝**，封存结果原样保留；等库存补足后可再次 `post` 重试。
- 入账成功后再次 `post` 不会二次调整，只返回原封存差异，
  输出中 `"already_posted": true`。

## 盘点状态与可用操作

| 状态 | 含义 | 可用操作 |
| --- | --- | --- |
| `open` | 已冻结基线，清点进行中 | `count`、`revoke`、`complete`、`seal`；该库位出入库不受限，但不能对该库位再开新盘点 |
| `sealed` | 已确认完成并冻结差异明细，尚未入账 | 仅 `post`（可反复重试）、`show`/`list`；不能改清点或封存明细 |
| `posted` | 差异已追加到当前库存 | 仅只读 `show`/`list`；重复 `post` 返回原结果；不能再改清点或明细 |

库存本身没有状态机：任何时候都可 `inbound`/`outbound`/`query`，
出库始终受“不得负库存”约束。

## 命令一览

| 命令 | 关键参数 | 说明 |
| --- | --- | --- |
| `inbound` | `--dir --loc --sku --qty [--memo]` | 入库，增加当前库存 |
| `outbound` | `--dir --loc --sku --qty [--memo]` | 出库，负库存则拒绝 |
| `query` | `--dir [--loc] [--sku]` | 查询当前库存（JSON 数组） |
| `start` | `--dir --id --loc` | 开盘点并冻结该库位账面基线 |
| `count` | `--dir --id --record --loc --sku --qty` | 导入一条清点记录 |
| `revoke` | `--dir --id --record` | 撤销未封存的清点记录 |
| `complete` | `--dir --id --loc` | 确认库位清点完成 |
| `seal` | `--dir --id` | 封存：计算并冻结逐商品差异 |
| `post` | `--dir --id` | 差异入账；拒绝后可重试，成功后幂等 |
| `show` | `--dir --id` | 查看单个盘点（基线/记录/封存明细/状态） |
| `list` | `--dir` | 列出全部盘点 |

## 典型场景：盘点中出库、漏盘归零、重试入账

```
go run ./cmd/stocktake inbound  --dir data --loc L1 --sku A --qty 10
go run ./cmd/stocktake inbound  --dir data --loc L1 --sku B --qty 5
go run ./cmd/stocktake start    --dir data --id ST1 --loc L1     # 基线 A:10 B:5
go run ./cmd/stocktake outbound --dir data --loc L1 --sku A --qty 4 --memo order-1
go run ./cmd/stocktake count    --dir data --id ST1 --record r1 --loc L1 --sku A --qty 10
                             # 只盘了 A；B 漏盘
go run ./cmd/stocktake complete --dir data --id ST1 --loc L1
go run ./cmd/stocktake seal     --dir data --id ST1
                             # 明细：A 10-10=0；B 0-5=-5（漏盘归零）
go run ./cmd/stocktake post     --dir data --id ST1
                             # 当前 A 保持 6（盘点中出了 4，差异为 0），
                             # 当前 B 5+(-5)=0
```

若某商品在封存后被大量出库，`post` 会因会变负而整体拒绝；之后执行一次
`inbound` 补足库存再 `post` 即可成功，封存明细在重试期间保持不变。

## 数据文件

- `DIR/events.jsonl`：每行一个带递增 `seq` 的事件（inbound/outbound/
  stocktake_started/count_recorded/count_revoked/location_completed/
  stocktake_sealed/stocktake_posted）。
- 调整依据可追溯：每条清点记录、撤销、封存明细和入账差异都是独立事件，
  `show` 可查看某笔盘点的完整依据链。
- 存储为单写者追加模型（CLI 每次调用处理一条命令），适合顺序操作；
  并发多写者不在本工具范围内。
