# podesk — gettext 目录更新工具

`podesk` 用旧 PO、新 POT 和 JSON 配置生成更新后的 PO 与审核 JSON，全程仅使用 Go 标准库，不调用 gettext 命令、不编译 MO。

## 用法

```
go run ./cmd/podesk -old old.po -pot new.pot -config config.json -out updated.po -audit audit.json
```

退出码：`0` 发布通过；`1` 有问题（仍输出待修订 PO 与审核 JSON）；`2` 硬错误（解析失败、重复活动键等）。源文件不会被修改。

## 配置

```json
{"language": "zh", "plural_slots": 2}
```

`plural_slots` 为目标语言的复数槽数。单数条目只能有一个译文；复数条目的 `msgstr[0..n-1]` 必须从零连续覆盖全部槽。旧 PO 头部的语言信息原样保留，不求值复数公式。

## 支持的 PO 格式子集

- UTF-8 双引号字符串，支持跨行拼接与标准转义（`\n`、`\t`、`\"`、`\\` 等）。
- 支持 `msgctxt`、`msgid_plural`、`msgstr[n]`；保留注释行（`#`、`#,` 等）与 fuzzy 标记。
- 键 = 上下文 + 原文：相同上下文与原文视为同一键；无 `msgctxt` 与空 `msgctxt ""` 是不同键。重复的活动键会被拒绝。
- 仅精确匹配键，且复数原文（`msgid_plural`）也一致时才保留旧译文，否则清空待译。旧有但已删除的条目转为 `#~` 废弃记录，不参与发布统计；不会用相似文字自动猜配。

## 占位符

`{identifier}` 为占位符（字母/下划线开头，可含数字）；`{{` 与 `}}` 表示字面花括号；不支持 `{name:fmt}` 之类的格式后缀。每份非空译文的占位符及出现次数必须与原文一致；复数各槽对照复数原文（`msgid_plural`）。

## 审核输出

审核 JSON 区分三类问题：`empty`（空译文）、`fuzzy`（模糊条目）、`placeholder`（占位符错误/复数槽缺失）。`release_ready` 仅在无任何问题时为 `true`；废弃项不计入发布统计。
