# erasure-stripe-recovery

本地条带备份工具（纯 Python 标准库，无第三方依赖、无在线存储）。
把文件切成若干条带，每个条带分为 `k` 个等长数据片，并用
GF(256) 上的系统型 Reed–Solomon 编码生成 `m` 个校验片。
只要每个条带仍存在任意 `k` 个不同的有效分片，就能完整恢复原文件。

## 有限域约定

- 域：GF(2^8)，约化多项式 `0x11B`（x^8 + x^4 + x^3 + x + 1，即 AES 多项式）。
- 域生成元：`0x03`（注意 `0x02` 模 `0x11B` 的阶只有 51，不能用作生成元）。
- 加法为按位异或；乘法经 log/exp 表实现。
- 生成矩阵：先取求值点 `0..n-1` 的 Vandermonde 矩阵 `V`（`n = k + m`），
  再计算 `G = V * inverse(V[:k])`，得到系统型矩阵 `[I | P]`。
  数据片原样保留（前 `k` 行为单位阵），校验片是数据片在 GF(256) 上的
  非平凡线性组合——不是简单异或，任意 `k` 个不同分片都可解码。
- 解码：取任意 `k` 个有效分片对应的 `G` 行组成子矩阵，在 GF(256) 上
  做高斯-约当求逆，左乘分片向量重建全部 `k` 个数据片。

## 参数限制

- 数据片 `k`：2 至 8；校验片 `m`：至少 1；总片数 `k + m` 不超过 12。
- 尾部不足一个条带时以零字节补齐参与编码；清单记录 `original_length`，
  恢复时按原始长度截断，补零不会混入输出。空文件合法（零个条带）。

## 清单格式（manifest.json）

```json
{
  "format": "erasure-stripe/1",
  "field": {"size": 256, "polynomial": "0x11b", "generator": "0x3"},
  "data_shards": 4,
  "parity_shards": 2,
  "stripe_size": 65536,
  "generator_matrix": [[1, 0, 0, 0], "..."],
  "original_length": 200000,
  "original_sha256": "<hex>",
  "stripes": [
    {
      "index": 0,
      "shards": [
        {"index": 0, "file": "stripe-000000-00.bin",
         "length": 65536, "sha256": "<hex>"}
      ]
    }
  ]
}
```

分片文件命名为 `stripe-<6位条带号>-<2位片号>.bin`，与清单位于同一目录。
加载清单时会拒绝：格式不符、域约定不符、生成矩阵与 `k/m` 不一致、
条带或分片序号重复、序号越界等元数据冲突。

## 使用

```sh
# 编码：生成分片文件和 manifest.json
python -m erasure_stripe.cli encode input.bin outdir -k 4 -m 2 [--stripe-size 65536]

# 检查：逐片验证长度与 SHA-256，报告缺失/损坏/不可恢复条带
python -m erasure_stripe.cli check outdir [--json]

# 恢复：重建原文件，整份 SHA-256 核对通过后才发布目标文件
python -m erasure_stripe.cli recover outdir restored.bin
```

行为保证：

- 编码与恢复均按条带流式处理，不会把整份文件及全部冗余一次载入内存。
- 缺失或损坏（长度/摘要不符）的分片一律视为不可用；每条带只要存在
  至少 `k` 个不同的有效片即可恢复，不限于丢失末尾几片。
- 有效片不足时报出具体不可用片序号，不写出目标文件、不留部分文件
  （临时文件会清理），也不修改任何已有分片。

## 复现与测试

```sh
python -m unittest discover -s tests
```

测试覆盖：GF(256) 矩阵可逆性、全部缺片组合恢复、中间片丢失、
坏片（篡改/截断）检测、尾部补零不泄漏、空文件、元数据冲突拒绝、
恢复失败不产生输出且不改动分片。
