# tagsync — 离线标签同步工具（Observed-Remove Set）

多个离线副本各自增删标签，通过交换 JSON 快照收敛到同一结果。
纯 Go 标准库实现，无网络服务，不做历史压缩（删除墓碑永久保留）。

## 数据模型

- 每个副本有固定唯一身份 `replica` 与单调递增序号 `next_seq`，
  持久化在 `<dir>/state.json`，重启后续号不复用。
- 每次 `add` 产生不可重复的新增标记 `mark = {replica, seq}`。
  同一标签重复新增会产生多个独立标记。
- `remove` 只撤销本副本**已经观察到**的该标签全部新增标记（写入墓碑）。
  另一副本并发新增、尚未被观察到的标记不受影响（add-wins），
  不存在"时间戳较新的操作覆盖全部历史"的问题。
- 标签可见 ⇔ 至少有一个新增标记没有对应墓碑。显示列表按标签去重。

## 快照格式

`export` 输出单行 JSON：

```json
{"replica":"A",
 "adds":[{"mark":{"replica":"A","seq":1},"tag":"go"}],
 "removes":[{"replica":"B","seq":2}]}
```

- `adds`：新增证据（标记 → 标签）。
- `removes`：删除证据（被撤销的标记列表）。

合并语义：

- 幂等且与顺序无关：重复投递、任意合并顺序结果相同。
- 删除证据先到、对应新增后到时，新增不会复活（墓碑先生效）。
- 同一标记对应不同标签（数据损坏或身份冲突）时**整次合并拒绝**，
  本地状态不变。
- 合并不会回退本地 `next_seq` 进度。

## CLI（JSONL 输出，每条命令输出一行 JSON）

```sh
tagsync -dir <状态目录> init -replica A     # 初始化副本（身份固定）
tagsync -dir data add go offline           # 新增标签，输出新标记
tagsync -dir data remove go                # 删除标签，输出被撤销的标记
tagsync -dir data list                     # 可见标签（去重）
tagsync -dir data explain [tag]            # 每个标签的存活/已删标记证据
tagsync -dir data export > snap.json       # 导出快照
tagsync -dir data merge snap.json          # 合并快照（出错则整体拒绝）
```

## 交换步骤

1. 副本 A：`tagsync -dir a export > a.json`
2. 副本 B：`tagsync -dir b export > b.json`
3. 交叉合并：`tagsync -dir a merge b.json`、`tagsync -dir b merge a.json`
4. 双方 `list` 结果一致即收敛；可重复交换，无副作用。

## 测试

```sh
go test ./...
```

覆盖：并发增删（add-wins）、乱序/重复合并、删除证据先于新增到达、
重启后身份与序号延续、旧快照不回退进度、冲突标记整次拒绝。
