# gzipcheck

离线文件接收环节的 GZIP 检查与展开工具。逐成员解析 RFC 1952 流，验证每个成员的头、DEFLATE 数据与尾部校验，输出 JSON 报告；可选地把全部成员的原文依次拼接发布到新文件。

## 用法

    go run ./cmd/gzipcheck -config config.json [-input file.gz] [-check]

- -config：JSON 配置文件路径（必填）。
- -input：覆盖配置中的输入路径。
- -check：强制仅检查模式，不写解压文件。

配置示例：

    {
      "input": "incoming.gz",
      "output": "payload.bin",
      "check_only": false,
      "max_header_bytes": 1048576,
      "max_members": 1000,
      "max_expanded_bytes": 1073741824
    }

output 为空或 check_only 为真时仅检查。三个上限字段为 0 或缺省表示不限制；计数按实际读取与展开字节进行，超限立即停止，整个流程不会把成员原文装入内存。

## 解析与校验

- 仅接受压缩方法 8（DEFLATE）；保留标志位（FLG 高 3 位）直接拒绝。
- 支持 FEXTRA、零结尾的 FNAME 与 FCOMMENT；存在 FHCRC 时验证头 CRC16（头 CRC32 的低 16 位）。成员名称只作为元数据出现在报告中，绝不用作输出路径。
- 每个成员独立验证尾部 CRC32 与 ISIZE（原文长度按 2 的 32 次方取模），支持空成员与多成员直接串联。
- 解压器按字节精确消费输入（向 flate 提供 io.ByteReader），不会因预读吞掉下一成员；成员边界由字节计数得出，报告各成员的起止偏移、压缩长度与展开长度。

## 报告

JSON 写至标准输出。顶层字段：ok、error_kind、error、total_members、compressed_total、expanded_total、members[]、input、output。每个成员含 index、header_offset、data_offset、end_offset（不含）、compressed_bytes、expanded_bytes、name、crc32、isize、error。

error_kind 取值：

- no_members：输入不含任何成员
- header_truncated：成员头截断
- bad_magic：首成员魔数错误
- unsupported_method：非 DEFLATE 压缩方法
- reserved_flags：保留标志位被设置
- header_crc_mismatch：FHCRC 校验失败
- header_limit_exceeded：头长度超限
- deflate_corrupt：压缩流损坏
- deflate_truncated：压缩流截断
- trailer_truncated：尾部 8 字节截断
- crc_mismatch：CRC32 校验失败
- isize_mismatch：ISIZE 校验失败
- trailing_garbage：成员之后存在非法字节
- member_limit_exceeded：成员数量超限
- expanded_limit_exceeded：累计展开字节超限

## 输出发布

只有整文件全部成员通过校验才发布输出：先写入输出目录下的临时文件，全部通过后重命名到目标；失败即删除临时文件。目标已存在时拒绝覆盖（检查前后各判断一次，尽力而为，非严格防竞态）。仅检查模式不产生任何解压文件。

## 退出码

- 0：校验通过（如配置输出则已发布）。
- 1：校验失败（报告照常输出到 stdout）。
- 2：用法或运行错误（配置缺失、IO 失败、目标已存在等），此时报告可能不完整。

## 限制

- 不支持的压缩方法一律拒绝，不做降级猜测。
- 串联成员必须首尾相接；成员之间的任何额外字节视为非法。
- 目标已存在的判断与重命名之间存在极小的竞态窗口。
- 名称/注释按字节原样报告，不做字符集转码。

## 测试

    go test ./...

覆盖串联成员边界、空成员、头截断、压缩流损坏、CRC/ISIZE 不匹配、尾部截断、成员后非法字节、保留标志、非 DEFLATE 方法及三类上限。
