# khcheck — SSH 主机密钥离线核对工具

`khcheck` 面向运维人员：读取**显式指定**的 known_hosts 样本文件，解释某个
SSH 主机密钥为什么可信或被拒绝。工具完全离线工作——不建立任何网络连接，
不读取用户真实的 `~/.ssh` 目录，也不修改任何样本文件。仅使用 Go 标准库。

## 用法

```
go run . -known-hosts <样本文件> -query "host[:port] ssh-ed25519 <base64-key>" [-query ...]
go run . -known-hosts <样本文件> -queries <查询文件>
```

- `-known-hosts`：必填，必须显式给出路径，不存在任何默认读取位置。
- `-query`：可重复，格式为 `host[:port] <key-type> <base64-key>`，端口缺省 22。
- `-queries`：每行一条查询的文件，`#` 开头为注释。

输出为 JSON：每条查询给出 `verdict`、`reason`、`fingerprint`（SHA256，
OpenSSH 的 `SHA256:...` 格式）以及引用的 known_hosts 行号；文件级解析问题
单列在 `diagnostics` 中，绝不静默丢弃。

## 支持范围

- 记录格式：普通记录、`@revoked` 撤销标记；保留原始行号。
- 主机模式：逗号分隔、`*`/`?` 通配、`!` 否定（只排除所在行，不影响其他行）、
  OpenSSH 盐化哈希主机名 `|1|base64(salt)|base64(HMAC-SHA1(salt, host))`。
- 端口语义：默认端口（22）匹配裸主机名；非默认端口只匹配 `[host]:port`
  形式，二者不混为同一目标。
- 公钥：仅支持 `ssh-ed25519`。校验 Base64 解码后的 SSH 二进制结构
  （`string alg || string key`，算法名须与记录一致，密钥须为 32 字节，
  不允许截断或尾随字节）。
- 明确不支持：证书类型（`*-cert-v01@openssh.com`）、`ssh-rsa`/`ecdsa` 等
  其他算法、`@cert-authority` 标记——均计入 `diagnostics`，且对应行永不
  作为有效凭据参与判定。

## 判定规则

对每条查询汇总样本中**所有**匹配行后给出结论：

- `revoked`：相同密钥命中 `@revoked` 行。撤销优先于一切，即使前面已有
  普通记录匹配也拒绝（不提前放行）。
- `trusted`：相同密钥命中普通记录，且无撤销命中。
- `key-changed`：主机模式有匹配，但所给密钥与所有记录密钥都不同。
- `first-seen`：没有任何主机模式匹配。
- `unsupported`：所给密钥类型不受支持或结构损坏。

样本存在任何解析错误时，报告顶层 `auditComplete` 为 `false`，且
`trusted` 降级为 `trusted-incomplete`——含错误的样本不能给出无条件可信
结论。损坏记录（Base64 非法、结构不符、长度错误）计入诊断，绝不当作
有效凭据。

## 测试

```
go test ./...
```

覆盖：行号与诊断收集、Ed25519 结构校验的异常输入、通配/否定/哈希主机名
匹配、端口隔离、撤销跨记录优先、否定模式只排除所在行、损坏记录不作为
凭据等。
