# Refresh Family Ledger

一个可离线验证的授权会话账本，模拟刷新令牌轮换（refresh token
rotation）与重放检测。**这只是本地模拟，不交付完整 OAuth 服务**：
令牌使用调用方提供的唯一模拟标识，不接入真实账号。

## 运行

```sh
go run ./cmd/tokens -dir <状态目录>
```

从 stdin 按 JSONL 接收操作，逐行向 stdout 返回结果。状态持久化到
`-dir` 指定的本地目录（默认 `token-ledger-data`）。

### 操作

| op | 字段 | 说明 |
| --- | --- | --- |
| `create` | `session_id, client_id, token_id, scopes, expires_at` | 创建授权会话与初始令牌 |
| `rotate` | `token_id, client_id, new_token_id, scopes?` | 轮换：作废旧令牌并建立唯一后继 |
| `revoke` | `session_id` | 主动撤销整条令牌族 |
| `advance` | `seconds` | 推进显式单调时钟 |
| `status` | `session_id` | 会话状态（区分 `replay` / `explicit` 撤销） |
| `history` | `session_id` | 事件历史 |
| `chain` | `session_id` | 令牌父子链（根在前） |

示例：

```jsonl
{"op":"create","session_id":"s1","client_id":"app","token_id":"t1","scopes":["read","write"],"expires_at":100}
{"op":"rotate","token_id":"t1","client_id":"app","new_token_id":"t2","scopes":["read"]}
{"op":"rotate","token_id":"t1","client_id":"app","new_token_id":"t3"}
{"op":"status","session_id":"s1"}
```

第三次操作是对已消费令牌 `t1` 的重放，整条令牌族被撤销，
`status` 返回 `"revoked":true,"revoke_reason":"replay"`。

## 状态机

令牌状态：

- **ACTIVE**：初始或轮换产生的新令牌。
- **CONSUMED**：被成功轮换消耗；同时创建唯一后继（原子完成）。
- **EXPIRED**：显式单调时钟到达绝对截止时间（`now >= expires_at`），
  截止时间沿令牌族继承，不随轮换延长。
- **REVOKED**（会话级，覆盖整条令牌族）：
  - 绑定客户端重放任何已成功消费的旧令牌（无论已存在多少代后继）；
  - 对会话主动 `revoke`。

范围规则：轮换时 `scopes` 只能维持或缩小；省略则沿用旧令牌范围。

## 边界与保证

- **普通拒绝不改账本**：未知令牌、客户端不符、令牌过期、扩大范围、
  新标识重复等都不会消耗当前令牌，也不产生事件。
- **重放判定顺序**：先校验客户端（不符即普通拒绝），再判定会话已撤销，
  然后才识别已消费令牌的重放并撤销整族、记录触发来源（令牌与客户端）。
- **无关会话隔离**：一个会话的重放撤销不影响其他会话。
- **持久化**：令牌父子关系与撤销记录写入带 SHA-256 校验和的存档，
  先写临时文件再原子改名；成功应答发生在保存之后，写入失败会回滚
  内存变更，不留下半次轮换。正常重开后仍能识别早期令牌的重放；
  损坏或被篡改的存档明确拒绝加载（`corrupt ledger archive`）。

## 测试

```sh
go test ./...
```

覆盖：轮换消耗与父子链、各类拒绝的零副作用、截止时刻边界
（`now == expires_at` 即失效）、多代后继后的重放撤销、错误客户端
重放不触发撤销、会话隔离、主动撤销覆盖后继、存档往返、重开后重放
检测、损坏存档拒绝加载、失败写入不留半次轮换。
