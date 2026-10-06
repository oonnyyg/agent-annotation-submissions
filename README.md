# gzipcheck — 离线 GZIP 成员检查与展开工具

按 RFC 1952 逐成员解析本地 gzip 文件，输出 JSON 报告；可选地把全部成员的
解压内容依次拼接写入指定的新文件。仅使用 Go 标准库。

## 用法

```
go run ./cmd/gzipcheck <config.json>
```

配置示例：

```json
{
  "input": "payload.gz",
  "output": "payload.bin",
  "maxHeaderBytes": 4096,
  "maxMembers": 64,
  "maxExpandedBytes": 1073741824
}
```
  "maxExpandedBytes": 1073741824
}
```

- `input`（必填）：本地 gzip 文件路径。
- `output`（可选）：解压输出路径。省略时为仅检查模式，不写任何解压文件。
  目标文件必须不存在；只有整文件全部成员校验通过才原子发布（临时文件 +
  重命名），失败会清理临时内容且不覆盖已有目标。
- `maxHeaderBytes` / `maxMembers` / `maxExpandedBytes`（可选，0 表示不限）：
  单成员头部长度、成员数量、累计展开字节上限。按实际读取与展开计数，
  超限立即停止；解压以流式进行，不把原文装入内存。

## 报告

标准输出为 JSON：每个成员的起止字节位置（`startOffset`/`endOffset`）、
头部长度、DEFLATE 压缩数据长度（`compressedBytes`）、实际展开长度
（`expandedBytes`）、可选的名称与注释。失败时 `error` 字段给出成员序号、
字节定位范围及分类：

- `header_truncated`：头部截断
- `bad_header`：魔数/方法/保留标志/头校验错误
- `deflate_corrupt`：压缩流损坏
- `trailer_check_failed`：CRC32 或 ISIZE（原文长度 mod 2^32）校验失败
- `trailing_garbage`：成员之后出现非法字节
- `limit_exceeded`：超出配置上限

## 解析行为

- 只接受 DEFLATE（CM=8），拒绝保留标志位与不支持的压缩方法。
- 支持 FEXTRA 额外字段、零结尾的 FNAME/FCOMMENT；存在 FHCRC 时验证头校验。
- 成员名称仅作为元数据报告，绝不作为输出路径。
- 支持空成员与多成员直接串联；读取器按字节计数，解压器不会预读跳过
  下一成员，也不会把第一个成员的结束当成整个文件的结束。

## 退出码

- `0`：全部成员校验通过（若配置了输出则已发布）。
- `1`：gzip 校验失败或超出上限（详见报告中的 `error`）。
- `2`：用法、配置或文件 IO 错误。

## 限制

- 逐成员顺序处理，不支持并行或随机访问。
- 名称/注释按原字节原样报告，不做字符集转码。
- MTIME/XFL/OS 等头字段被解析但不纳入报告。

## 测试

```
go test ./...
```

覆盖串联成员边界、空成员、损坏的压缩流与尾部校验、头部截断、成员后
非法字节、FHCRC 验证、各类上限以及输出发布/清理行为。
