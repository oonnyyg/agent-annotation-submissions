# podesk — gettext 目录更新与发布审核工具

`podesk` 用旧 PO 译文、新 POT 模板和 JSON 配置生成更新后的 PO 与审核 JSON，全程仅使用 Go 标准库，不调用 gettext 命令，不编译 MO，也不修改源文件。

## 用法

```
go run ./cmd/podesk -old old.po -pot new.pot -config config.json -out updated.po -audit audit.json
```

- 有问题时仍输出待修订 PO 与审核 JSON，但退出码为 1 且 `release_ready` 为 `false`。
- 全部通过时退出码为 0。

## 配置（JSON）

```json
{ "language": "zh", "plural_slots": 2 }
```

- `plural_slots`：目标语言的复数槽数。复数条目的 `msgstr` 索引必须从 0 连续覆盖全部槽；单数条目只能有一个 `msgstr`。
- 旧 PO 头部（含 `Language`、`Plural-Forms`）原样保留；工具不求值复数公式。

## 支持的 PO 格式子集

- UTF-8 双引号字符串；同一字段可跨多行拼接。
- 转义：`\n` `\t` `\r` `\"` `\\` `\0`。
- `msgctxt`、`msgid`、`msgid_plural`、`msgstr`、`msgstr[n]`。
- 注释行（`#`、`#:`、`#,` 等）原样保留；`#, fuzzy` 识别为 fuzzy 标记。
- `#~` 前缀的废弃条目可解析并原样输出。
- 不支持：复数公式求值、MO 编译、domain 指令等 gettext 扩展。

## 合并规则

- 键 = （是否有上下文, 上下文, msgid)。无 `msgctxt` 与 `msgctxt ""` 是不同的键。
- 仅精确匹配：键相同且（复数条目的）`msgid_plural` 也一致时才保留旧译文，否则清空待译。
- 模板中新增的条目为空待译；旧目录中已删除的条目转为 `#~` 废弃记录，不参与发布统计。
- 不做相似度猜配（无 fuzzy matching）。
- 旧目录或模板中出现重复的活动键会直接报错拒绝。

## 审核规则

审核 JSON 区分三类问题，任一出现即阻止发布通过：

- `empty_translation`：译文为空，或复数槽缺失/为空。
- `fuzzy`：条目带 fuzzy 标记。
- `placeholder_mismatch`：非空译文的占位符与原文不一致。

## 占位符语法

- `{identifier}` 为占位符（标识符由字母、数字、下划线组成）。
- `{{` 与 `}}` 表示字面花括号。
- 不支持格式后缀（如 `{n:0.00}`）。
- 每份非空译文的占位符及出现次数必须与原文一致；复数条目各槽对照 `msgid_plural`。

## 测试

```
go test ./...
```