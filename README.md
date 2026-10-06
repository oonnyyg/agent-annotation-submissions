# mailbox-sequence-store

本地邮箱管理工具（仅 Go 标准库）。面向离线客户端：消息删除、复制之后，
仍能通过稳定的 UID 找回原来的邮件。不实现任何网络邮件协议，也不做 MIME
解析——消息正文按不透明字符串存储。

## 启动方式

```sh
go build -o mailboxd .
./mailboxd -data ./data < commands.jsonl
```

- 从标准输入逐行读取 JSONL 命令，向标准输出逐行写 JSONL 结果。
- `-data` 指定数据目录（默认 `~/.mailbox-sequence-store`），状态保存在
  其中的 `store.json`；写入采用临时文件 + 原子重命名，正常重启后内容与
  编号完全延续。
- 每条命令要么完整生效并落盘，要么整体失败（`{"ok":false,"error":...}`），
  不会留下部分变更。

## 两种编号的区别

- **UID**：邮箱内永久编号，从 1 递增、绝不复用。消息被清除后其 UID 空缺，
  新消息也不会占用旧 UID。跨删除/复制场景追踪邮件应使用 UID。
- **序号（seq）**：当前消息在邮箱中的位置，从 1 连续排列。清除（expunge）
  后剩余消息重新连续编号，序号会"移动"。

所有查询结果同时返回 `seq` 与 `uid`。

## 命令

### create_mailbox / list_mailboxes

```json
{"cmd":"create_mailbox","name":"inbox"}
{"cmd":"list_mailboxes"}
```

### append

```json
{"cmd":"append","mailbox":"inbox","body":"hello","seen":false}
=> {"ok":true,"uid":1,"seq":1}
```

### fetch（读取正文，不改变已读状态）

```json
{"cmd":"fetch","mailbox":"inbox","set":{"by":"uid","ranges":[{"start":1,"end":3},{"start":5,"end":5}]}}
```

选集规则：

- `by` 为 `"uid"` 或 `"seq"`；`ranges` 为闭区间，单条用 `start == end`。
- 重叠选集自动去重；不存在的编号静默忽略。
- 倒置区间（`start > end`）、0 或非法 `by` 会被明确拒绝。
- 每条命令先解析出完整选集再执行，执行期间序号移动不会影响本命令。

### store（设置已读 / 待删除标记）

```json
{"cmd":"store","mailbox":"inbox","set":{"by":"seq","ranges":[{"start":1,"end":2}]},"seen":true,"deleted":false}
```

`seen` 与 `deleted` 至少给一个；标记 `deleted` 不会立即移除消息。

### expunge（清除待删除消息）

```json
{"cmd":"expunge","mailbox":"inbox"}
=> {"ok":true,"removed":[{"seq":2,"uid":2}]}
```

返回被移除消息的**旧序号**与 UID；剩余消息重新从 1 连续编号，UID 不变。

### copy（复制到另一个已有邮箱）

```json
{"cmd":"copy","mailbox":"src","to":"archive","set":{"by":"uid","ranges":[{"start":1,"end":3}]}}
=> {"ok":true,"copied":[{"src_seq":1,"src_uid":1,"dst_seq":4,"dst_uid":7}]}
```

复制语义：

- 按**源序号顺序**追加到目标邮箱；正文与 seen/deleted 标记原样保留。
- 目标邮箱为副本分配**新的 UID**，返回值给出源到目标的编号对应关系。
- 源消息完全不变；目标邮箱不存在时整条命令失败，不产生任何变更。

### stats

```json
{"cmd":"stats","mailbox":"inbox"}
=> {"ok":true,"stats":{"mailbox":"inbox","messages":3,"unseen":2,"deleted":0,"next_uid":4}}
```

## 测试

```sh
go test ./...
```
