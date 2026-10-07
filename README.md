# 现金钞箱配钞模拟器（cash-cassette-dispense）

一个纯 Go 标准库实现的自助现金设备本地模拟后台。在有限钞箱库存下生成
可执行的配钞方案，管理取款请求从预留到确认出钞的完整生命周期，并将
库存与请求持久化到本地 JSON 文件。不连接任何银行系统。

## 规则

### 金额与面额

- 所有金额、面额一律使用整数最小货币单位（如“分”），无浮点。
- 每种面额配置现有张数；`init` 导入时拒绝重复面额与负库存。
- `add` 补钞只增加实物库存，不改变任何已有请求的预留方案。

### 配钞算法

- 取款必须**精确**凑出金额。
- 目标：总张数最少；张数相同时，从最大面额向下比较，优先多用大面额。
- 实现为分阶段有界背包 DP（`plan.go`），不是贪心；贪心失败但存在解时
  仍能给出方案（例如库存 `{4:1, 3:2, 1:5}` 取 6，贪心给 4+1+1 三张，
  本系统给 3+3 两张）。
- 失败原因区分两种：
  - `ErrInsufficientBalance`：可用总余额不足；
  - `ErrCannotCompose`：余额够但面额组合不出该金额。

### 请求生命周期

```
withdraw ──> RESERVED ──confirm──> CONFIRMED（扣减实物库存）
                │
                └──cancel──> CANCELLED（释放预留）
```

- `withdraw` 成功即按方案预留钞票，其他请求只能使用未预留库存。
- 请求编号幂等：同 id 同金额返回原结果；同 id 不同金额拒绝
  （`ErrAmountMismatch`）。配钞失败不残留任何预留。
- `confirm` 扣减实物库存；重复 confirm 是幂等空操作，不会重复扣减。
- `cancel` 只释放未确认（RESERVED）请求；重复 cancel 是幂等空操作；
  已确认请求不能取消。不模拟部分出钞。

### 持久化与对账

- 每次变更后状态原子写入 `--state` 指定的 JSON 文件（先写临时文件再
  rename），正常重启后未完成请求可继续 confirm/cancel。
- `status` 输出逐面额的实物（physical）、预留（reserved）、可用
  （available）张数，以及每个请求的状态与配钞明细。三者满足
  `available = physical - reserved`，且 reserved 等于所有 RESERVED
  请求方案之和，可对账。

## 使用

```sh
go build -o cashsim .

# 初始化钞箱：面额:张数，逗号分隔
./cashsim --state cash_state.json init 100:2,50:1,20:3,10:5

# 取款（创建请求并预留）
./cashsim --state cash_state.json withdraw w1 130

# 确认出钞 / 取消
./cashsim --state cash_state.json confirm w1
./cashsim --state cash_state.json cancel w2

# 补钞（不影响已有预留）
./cashsim --state cash_state.json add 50:10

# 查看库存与请求对账
./cashsim --state cash_state.json status
```

`--state` 默认为当前目录的 `cash_state.json`。

## 测试

```sh
go test ./...
```

覆盖：非贪心配钞（DP 优于贪心）、同张数大面额优先、库存张数约束、
余额不足与无法组合的区分、请求交错共享库存、幂等取款/确认/取消、
取消后库存释放再分配、补钞不改变预留、持久化重启后继续处理、
实物/预留/可用三方对账。

## 文件结构

- `plan.go` — 配钞 DP 与平局规则
- `store.go` — 库存、请求状态机、错误分类
- `persist.go` — JSON 持久化与 `denom:count` 解析
- `main.go` — CLI 入口
- `plan_test.go` / `persist_test.go` — 自动化测试
