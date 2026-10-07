# erasure-stripe-recovery

本地条带备份工具：把文件切成条带，每条带用 GF(256) 上的系统型
Reed–Solomon 编码生成校验片。部分分片丢失或损坏后仍可恢复原文件。
仅依赖 Python 标准库（>= 3.12），不需要任何在线存储。

## 有限域约定

- 域：GF(2^8)，约化多项式 `x^8 + x^4 + x^3 + x + 1`（0x11B，AES 多项式）。
- 本原元（生成元）：`0x03`（注意 0x02 在该域中不是本原元）。
- 加法/减法即按位异或；乘除法通过对数/指数表实现。
- 分片序号 `i`（0..n-1）对应求值点 `a_i = i + 1`。
- 生成矩阵 `G = V * inv(V_top)`：V 为 n×k 范德蒙德矩阵（行 i 为
  `[1, a_i, a_i^2, ...]`），V_top 为其前 k 行。因此 `G[:k] == I`，
  数据片保持原字节（系统型），校验片为数据片在 GF(256) 上的线性组合。
- 恢复任意 k 个不同分片：取 G 对应 k 行组成 k×k 子矩阵，在 GF(256) 上
  高斯消元求逆，左乘可用分片即还原 k 个数据片。不是简单的异或方案。

## 布局约束

- 数据片 k：2..8；校验片 p >= 1；总片数 k + p <= 12。
- 条带大小 `--stripe-size` 字节，片长 `shard_length = ceil(stripe_size / k)`。
- 尾部不足一个条带时，最后一个数据片在内存中补零参与编码；清单记录每个
  条带的真实数据长度 `data_length`，恢复时按该长度截断，补零不会混入输出。
- 空文件编码为一个 `data_length = 0` 的条带，可正常恢复为空文件。

## 清单格式（manifest.json）

```json
{
  "format": "erasure-stripe/1",
  "field": {"size": 256, "polynomial": "0x11B", "generator": 3},
  "layout": {"data_shards": 4, "parity_shards": 2, "total_shards": 6,
             "stripe_size": 65536, "shard_length": 16384},
  "generator_matrix": [[1,0,0,0], ...],
  "source": {"name": "file.bin", "length": 150000, "sha256": "..."},
  "stripes": [
    {"index": 0, "data_length": 65536,
     "shards": [{"index": 0, "file": "stripe-000000.shard-00.bin",
                 "length": 16384, "sha256": "..."}, ...]}
  ]
}
```

分片文件与清单同目录，命名为 `stripe-<i>.shard-<j>.bin`。
加载清单时会校验域约定、布局自洽性、生成矩阵与约定一致；分片序号重复、
长度/文件名与约定冲突都会被明确拒绝。

## 使用

```sh
# 编码：生成分片文件与 manifest.json
python -m erasure_stripe.cli encode <源文件> <输出目录> -k 4 -p 2 [--stripe-size 65536]

# 检查：逐片核对长度与 SHA-256，报告缺失/损坏片
python -m erasure_stripe.cli check <输出目录>/manifest.json

# 恢复：每条带取任意 k 个有效片解码，整份摘要核对通过后才发布目标文件
python -m erasure_stripe.cli restore <输出目录>/manifest.json <目标文件>
```

恢复先写入 `<目标>.partial`，整份 SHA-256 与清单一致后才原子替换为正式
文件；任何条带有效片不足 k 个时会报出具体不可用分片序号，不输出部分文件，
也不会修改已有分片。

## 复现与测试

```sh
python -m unittest discover -s tests -v
```

测试覆盖：GF(256) 运算、矩阵求逆、系统型生成矩阵、任意 k-of-n 缺片组合、
中间位置缺片、坏片检测与恢复、尾部补零不外泄、空文件、有效片不足时拒绝
发布、清单元数据冲突拒绝。

手动复现一次丢失恢复：

```sh
python -m erasure_stripe.cli encode big.bin backup -k 4 -p 2
del backup\stripe-000003.shard-00.bin backup\stripe-000003.shard-05.bin
python -m erasure_stripe.cli check backup/manifest.json
python -m erasure_stripe.cli restore backup/manifest.json big-restored.bin
```
