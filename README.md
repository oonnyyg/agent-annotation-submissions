# cash-cassette-dispense

自助现金设备的本地模拟后台（纯 Go 标准库，不连接银行系统）。在有限钞箱
库存下生成可执行的配钞方案，并管理取款请求从预留到确认出钞的完整过程。

## 规则

- **单位**：面额与金额统一使用整数最小货币单位；每种面额配置现有张数。
- **导入**：`load` 拒绝重复面额与负库存；`replenish` 只增加实物库存，
  不改变已有请求的预留方案。
- **配钞**：取款必须精确凑出金额。优先总张数最少；张数相同时按面额从高
  到低比较用量，优先多用较大面额。使用动态规划精确求解，不以贪心失败
  代替无解；区分「总余额不足」与「有余额但无法组合」两种失败。
- **预留**：创建请求成功立即预留对应钞票，其他请求只能使用未预留库存。
  请求编号重复且金额相同返回原结果（幂等），金额不同则拒绝；失败的配钞
  不残留预留。
- **确认/取消**：确认后扣除实物库存；取消只释放未确认请求。重复确认或
  取消不会再次扣减或归还；已确认请求不能取消；不模拟部分出钞。
- **持久化**：库存与请求保存为 JSON（默认 `atm-state.json`，可用
  `-state` 指定），正常重启后未完成请求保持预留，可继续确认或取消。
- **对账**：`status` 输出逐面额的实物 / 预留 / 可用张数，满足
  `实物 = 预留 + 可用`；`requests` 输出请求状态与配钞明细，预留合计与
  各 RESERVED 请求的配钞明细一致。

## 构建与测试

```sh
go build ./...
go test ./...
```

测试覆盖：非贪心配钞（如面额 {10,6} 凑 12 必须得到 6+6）、同张数大面额
优先、余额不足与无法组合的区分、请求交错占用库存、幂等创建/确认/取消、
补钞不影响已有预留、持久化重启恢复。

## 复现示例

```sh
go run . -state demo.json load 100:2,50:3,20:5
go run . -state demo.json withdraw w1 170   # 预留 100x1,50x1,20x1
go run . -state demo.json status            # 实物/预留/可用对账
go run . -state demo.json confirm w1        # 确认出钞，扣减实物
go run . -state demo.json requests          # 请求状态与配钞明细
```

非贪心场景复现（贪心取 10 后余 2 无解，精确解为 6+6）：

```sh
go run . -state demo2.json load 10:5,6:5
go run . -state demo2.json withdraw g1 12   # plan=6x2
```

## 命令一览

```
cash-atm [-state FILE] <command>
  load <denom:count,...>     导入钞箱（拒绝重复面额/负库存）
  replenish <denom> <count>  补钞（不影响已有预留）
  withdraw <id> <amount>     创建取款请求并预留
  confirm <id>               确认出钞（幂等）
  cancel <id>                取消未确认请求（幂等）
  status                     逐面额 实物/预留/可用
  requests                   请求状态与配钞明细
```

## 代码结构

- `dispense/solve.go`：精确配钞 DP（最少张数 + 大面额优先）。
- `dispense/atm.go`：库存、预留、请求生命周期。
- `dispense/store.go`：JSON 持久化（临时文件 + rename 原子写入）。
- `main.go`：命令行入口。
- `dispense/dispense_test.go`：自动化测试。
