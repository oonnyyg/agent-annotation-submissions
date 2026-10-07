# ssh-known-host-audit

离线 SSH 主机密钥核对工具：读取**显式指定**的 known_hosts 样本文件，
解释某个主机密钥为什么可信或被拒绝。工具**不建立任何网络连接**，
**不读取用户真实的 `~/.ssh` 目录**，也**不修改样本文件**。仅使用 Go 标准库。

## 用法

```
ssh-known-host-audit -knownhosts FILE [-query SPEC ...] [-queries FILE] [-pretty]
```

- `-knownhosts FILE`：known_hosts 样本（必填，只处理显式指定的文件）。
- `-query SPEC`：单条查询，格式为 `目标 密钥类型 base64密钥`，可重复。
- `-queries FILE`：批量查询文件，每行一条同格式查询，`#` 开头为注释。
- `-pretty`：缩进 JSON 输出。

目标形式：`host`（默认端口 22）、`host:port`、`[host]:port`。
裸主机名与 `[host]:port` 是不同目标，不会混为同一记录。

输出为 JSON：每条查询给出 `verdict`、`reason`、命中的样本行（行号、
指纹、是否同密钥）、样本级 `diagnostics` 以及 `complete` 标记。

## 支持范围

- 主机模式：逗号分隔列表、`*`/`?` 通配、`!` 否定、OpenSSH 盐化哈希
  主机名（`|1|salt|hash`，HMAC-SHA1）。
- 标记：`@revoked` 支持；`@cert-authority` 识别但**不支持**，命中即
  记入诊断并标记审计不完整。
- 公钥：仅支持 `ssh-ed25519`。校验 Base64 解码后的 SSH 二进制结构
  （内嵌算法名一致、密钥长度 32 字节、无截断/尾随字节），并计算
  `SHA256:` 指纹。证书与其他算法明确报告 `unsupported`。

## 判定规则

汇总**所有**匹配行后判定，绝不因前面命中普通记录而提前放行：

- `revoked`：任一 `@revoked` 行与待核对密钥相同 —— 撤销优先，直接拒绝。
- `trusted`：存在普通行与待核对密钥相同，且样本无错误。
- `changed`：目标有匹配记录，但密钥全部不同。
- `unknown`：无任何匹配行（首次出现）。
- `unsupported` / `invalid`：待核对密钥算法不受支持或数据损坏。
- `inconclusive`：密钥本可信，但样本含解析错误或不可用记录
  （`complete: false`），不能给出无条件可信结论。

否定模式（`!pattern`）只排除其所在行，不影响其他行。损坏记录只产生
诊断，绝不作为有效凭据。

## 开发

```
go test ./...
```
