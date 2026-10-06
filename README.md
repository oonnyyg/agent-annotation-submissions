# Refresh Family Ledger（本地模拟）

一个可离线验证的授权会话账本：模拟刷新令牌轮换（refresh token rotation），
并在旧令牌被重放时撤销整条令牌族。**这只是本地模拟，不是完整的 OAuth 服务**；
令牌是调用方提供的唯一模拟标识，不接入真实账号。

## 运行

```sh
go run ./cmd/tokens -dir state
```

从 stdin 按行读取 JSONL 操作，每个操作向 stdout 写一行结果。
状态持久化到 `-dir` 指定的本地目录（`ledger.json`）。

## 操作

- `create`：`{"op":"create","session":"s1","client":"c1","scopes":["read"],"token":"t1","expires_at":1000}`
  创建会话，绑定客户端与允许范围，初始令牌带绝对截止时间。
- `rotate`：`{"op":"rotate","token":"t1","client":"c1","new_token":"t2","scopes":["read"]}`
  作废旧令牌并建立唯一后继；`scopes` 省略时沿用旧范围，只能维持或缩小。
- `revoke`：`{"op":"revoke","token":"t1","client":"c1"}` 主动撤销整条令牌族。
- `advance`：`{"op":"advance","now":10}` 显式推进单调时钟（不允许回退）。
- `query`：`{"op":"query","session":"s1"}` 返回会话状态、撤销记录与历史。

## 状态机

单个令牌：`active -> consumed`（被轮换）、`active -> expired`（时钟到达
`expires_at`，含等于）、`active|consumed -> revoked`（族被撤销）。
会话状态：`active` / `expired`（普通失效）/ `revoked`（重放或主动撤销），
查询结果中通过 `revocation.reason`（`replay` 或 `explicit`）区分。

关键规则：

- 轮换成功：旧令牌置为 `consumed` 并记录父子链，后继沿用同一绝对截止时间。
- 重放检测：绑定客户端再次使用已 `consumed` 的旧令牌，无论已轮换多少代，
  整条令牌族（含当前头令牌）全部撤销，并记录触发令牌与客户端。
- 普通拒绝（未知令牌、客户端不符、扩大范围、复用已存在的令牌标识、
  令牌已失效）不改动账本，也不消耗当前令牌。
- 时钟只前进；到达截止时刻（`now >= expires_at`）令牌即失效。

## 持久化

- 成功应答在写入之后返回；写入采用临时文件 + fsync + rename，失败不留半次轮换。
- 存档带 SHA-256 校验和；损坏或被篡改的存档在加载时明确拒绝（`corrupt ledger archive`）。
- 正常重开后仍能识别早期令牌的重放（父子关系与撤销记录均已持久化）。

## 测试

```sh
go test ./...
```

覆盖：轮换父子链、范围缩小/扩大、标识复用、客户端不符、到期边界
（`expires_at - 1` 可轮换、`expires_at` 拒绝）、时钟回退、多代后继下的
重放族撤销、无关会话隔离、主动撤销覆盖后继、持久化往返、重开后重放检测、
损坏存档拒绝、写入失败不破坏旧存档。
