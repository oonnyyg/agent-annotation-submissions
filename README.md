# tenpin-score-desk

十瓶保龄球练习馆记分工具。Go 标准库实现，通过 JSONL CLI 持续记录多人各自的球局，
支持更正历史投球并重新计分。本地 JSONL 文件持久化，重启后可继续录入或查询。不做赛程编排。

## 运行

```sh
go build -o tenpin .
TENPIN_DATA=games.jsonl ./tenpin   # 从 stdin 读 JSONL 命令，向 stdout 写 JSONL 响应
```

`TENPIN_DATA` 指定数据文件，缺省为当前目录 `games.jsonl`。每行一个命令，每行一个响应。

## 命令格式

- 开新局：`{"op":"new","player":"alice","game":"g1"}`
- 记录投球：`{"op":"roll","player":"alice","game":"g1","pins":10}`
- 更正投球：`{"op":"correct","player":"alice","game":"g1","index":1,"pins":7}`
  （`index` 为整局投球序列中的下标，从 0 起）
- 查询记分单：`{"op":"query","player":"alice","game":"g1"}`

响应为 `{"ok":true,"game":{...}}` 或 `{"ok":false,"error":"..."}`。

## 记分单字段

- `frames[].rolls`：该轮原始投球。
- `frames[].score`：已确定的轮分；缺少后续奖励球时**待定**，字段不输出（不是 0）。
- `frames[].cumulative`：累计分，只在前面所有轮分都已确定时公布。
- `position`：当前应投位置（`frame`、`rollInFrame`、`pinsStanding`）与是否整局完成。
- `total`：仅在整局完成后给出；未完成球局不会输出伪最终成绩。
- `history`：成功更正操作的记录（下标、原值、新值）。

## 规则要点

- 前九轮首球全中即结束该轮，否则投两球；全中加随后两球，补中加随后一球。
- 第十轮全中获两次补投、补中获一次补投，补投后重新摆瓶，不生成新轮。
- 每球击倒数为非负整数且不超过当前剩余瓶数；已完成球局拒绝继续投球。
- 更正保留其后原始记录，并从头重新验证整个序列：若更正使后续记录不再合法，
  整个更正被拒绝，原局保持不变，不会擅自删除投球。

## 测试

```sh
go test ./...
```

覆盖连续全中（满分 300）、末轮补投与重新摆瓶、更正引起的跨轮分数变化、
非法更正回滚以及重启后持久化恢复。
