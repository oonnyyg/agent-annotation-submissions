# torrent-piece-auditor

只读的种子文件核验器：解析 v1 `.torrent` 元信息，把分片哈希与本地文件逐一对应，
定位损坏分片究竟影响哪些文件范围。不连接任何节点、不下载数据、不修改被检文件。

## 用法

```sh
go run ./cmd/torrentcheck <file.torrent> <content-root>
```

- 退出码：`0` 全部校验通过；`1` 校验完成但存在损坏/不可核验分片；`2` 用法或元信息错误。
- 报告以 JSON 输出到 stdout，诊断信息输出到 stderr。

## 支持范围

- BitTorrent v1 元信息：单文件（`info.length`）与多文件（`info.files`）模式。
- 严格的 bencode 解析：拒绝截断数据、重复字典键、非法整数编码（前导零、`-0`、空整数等）。
  字典键乱序不拒绝，但在 `warnings` 中告警。
- info 哈希取自原始 info 字典字节片段的 SHA-1，绝不重编码。
- 校验文件长度非负、分片长度为正、哈希数量与总长度一致；空文件不产生额外分片，
  最后一个分片允许不足标准长度。
- 路径安全：拒绝绝对路径、`..` 跳转、分隔符/NUL 等非法组件；Windows 下拒绝大小写冲突；
  打开文件前解析符号链接，禁止越出内容根目录。所有路径通过检查后才会打开内容文件。
- 流式核验：跨文件边界拼接分片，内存占用恒定（256 KiB 缓冲），不随内容总量增长。
  缺失或长度不符的文件对应分片标记为 `unverifiable`，后续分片仍按声明长度定位，不整体错位。

## 报告格式

```jsonc
{
  "torrent_file": "...",      // 种子文件路径
  "content_root": "...",      // 内容根目录
  "name": "...",              // info.name
  "info_hash": "hex",         // 原始 info 字节的 SHA-1
  "piece_length": 262144,
  "num_pieces": 10,
  "total_length": 2500000,
  "warnings": ["..."],        // 乱序字典键、读取错误等
  "files": [
    {
      "path": "sub/b.bin",        // 相对根目录，'/' 分隔
      "declared_length": 6,       // 元信息声明长度
      "actual_length": 6,         // 实际长度，缺失时为 -1
      "status": "ok"              // ok | missing | length_mismatch
    }
  ],
  "pieces": [
    {
      "index": 1,
      "offset": 4,                // 在拼接流中的起始偏移
      "length": 4,                // 最后一片可能小于 piece_length
      "status": "ok",             // ok | corrupt | unverifiable
      "expected_sha1": "hex",
      "actual_sha1": "hex",       // 仅实际计算过时存在
      "ranges": [                 // 本分片覆盖的文件区间
        { "path": "a.bin", "file_offset": 4, "length": 1 },
        { "path": "sub/b.bin", "file_offset": 0, "length": 3 }
      ]
    }
  ],
  "summary": {
    "pieces_ok": 1,
    "pieces_corrupt": 0,
    "pieces_unverifiable": 2,
    "files_ok": 2,
    "files_with_issues": 1,
    "verified": false
  }
}
```

## 开发与测试

```sh
go test ./...
```

代码结构：

- `internal/bencode` — 保留原始字节区间的严格 bencode 解析器。
- `internal/torrent` — 元信息模型、info 哈希、路径安全校验。
- `internal/audit` — 流式分片核验与报告生成。
- `cmd/torrentcheck` — CLI 入口。
