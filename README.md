# mailbox-sequence-store

一个只用 Go 标准库实现的本地邮箱管理工具。它面向离线客户端：从标准输入
读取 JSONL 命令，向标准输出逐行返回 JSONL 结果，状态持久化到本地文件。
不实现任何网络邮件协议，也不做 MIME 解析——正文按不透明字符串存储。

## 启动方式

```sh
go build -o mbox .
./mbox -state /path/to/state.json
```

- 状态文件路径优先级：`-state` 标志 > 环境变量 `MAILBOX_STATE` > 当前目录
  `mailbox-state.json`。
- 每条命令一行 JSON，每个结果一行 JSON。正常重启后内容与编号完全延续。
- 状态通过“临时文件 + 重命名”原子写入；失败的命令不会留下部分变更。

## 两种编号的区别

- **UID**：邮箱内永久编号，从 1 递增，删除后**不复用**。每个邮箱有独立的
  UID 计数器。消息被删除、清除（expunge）或邮箱重排后，UID 始终不变。
- **序号（seq）**：消息当前在邮箱中的位置，从 1 开始连续排列。expunge
  移除消息后，剩余消息会重新连续编号，因此序号会随删除而变化。

所有查询结果同时返回 `seq` 和 `uid`，离线客户端应使用 UID 做长期引用。

## 命令

### create — 创建邮箱
```json
{"cmd":"create","mailbox":"inbox"}
```

### append — 追加消息
```json
{"cmd":"append","mailbox":"inbox","body":"hello"}
=> {"ok":true,"uid":1,"seq":1}
```

### list / fetch — 查询消息
`list` 返回元数据，`fetch` 额外返回正文。两者都**不会改变已读状态**。
不带 `select` 时覆盖全部消息。
```json
{"cmd":"fetch","mailbox":"inbox","select":{"by":"uid","items":["1","3:5"]}}
=> {"ok":true,"messages":[{"seq":1,"uid":1,"seen":false,"deleted":false,"body":"hello"}, ...]}
```

### store — 设置标记
支持 `seen`（已读）与 `deleted`（待删除）。标记删除**不会**立即移除消息。
```json
{"cmd":"store","mailbox":"inbox","select":{"by":"seq","items":["2:4"]},"seen":true,"deleted":false}
```

### expunge — 清除待删除消息
移除所有带 `deleted` 标记的消息，返回其**旧序号**与 UID；剩余消息重新
连续编号。
```json
{"cmd":"expunge","mailbox":"inbox"}
=> {"ok":true,"removed":[{"old_seq":2,"uid":2}]}
```

### copy — 复制到另一个邮箱
按**源序号顺序**复制选中消息，正文与标记保留，目标邮箱分配新 UID 并返回
对应关系。源消息不变；目标邮箱不存在时整条命令无效（不产生任何变更）。
```json
{"cmd":"copy","mailbox":"inbox","to":"archive","select":{"by":"uid","items":["1:3"]}}
=> {"ok":true,"copied":[{"src_uid":1,"dst_uid":1},{"src_uid":2,"dst_uid":2}]}
```

### stats / mailboxes — 统计与邮箱列表
```json
{"cmd":"stats","mailbox":"inbox"}
=> {"ok":true,"stats":{"messages":3,"unseen":2,"deleted":1,"next_uid":4}}
{"cmd":"mailboxes"}
=> {"ok":true,"mailboxes":["archive","inbox"]}
```

## 选集（select）语义

- `by` 为 `"uid"` 或 `"seq"`；`items` 为编号或闭区间 `"a:b"` 的列表。
- 重叠选集自动去重；不存在的编号静默忽略。
- 倒置区间（如 `"5:2"`）、0、负数、非数字、空选集都会被**明确拒绝**，
  命令返回 `{"ok":false,"error":"..."}` 且不产生任何变更。
- 每条命令在执行任何修改前先解析出完整选集，执行期间不会因序号移动
  而选错消息。

## 测试

```sh
go test ./...
```
