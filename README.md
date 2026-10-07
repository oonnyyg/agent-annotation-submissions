# Clifford Shot Simulator

一个仅使用 Python 标准库的小型量子线路模拟器。它在本地维护复数状态向量，不连接任何量子硬件。

## 能力范围

- 支持 `1` 到 `10` 个量子位，初始量子态为全零态 `|0...0⟩`。
- 支持单量子位门 `H`、`X`、`S` 和受控非门 `CNOT`。
- 支持计算基中途测量；结果写入经典位，坍缩后状态重新归一化。
- 支持按经典位等于 `0` 或 `1` 条件执行门或测量。
- 每次 shot 都重新初始化量子态和经典位，避免上一次试验污染下一次。
- 固定 `--seed` 可重复采样；不传种子则由 Python 随机数生成器提供熵。

## 线路格式

线路是 UTF-8 JSON 对象：

```json
{
  "num_qubits": 1,
  "num_classical_bits": 1,
  "operations": [
    {"gate": "H", "qubit": 0},
    {"gate": "measure", "qubit": 0, "classical_bit": 0},
    {
      "gate": "X",
      "qubit": 0,
      "condition": {"classical_bit": 0, "equals": 1}
    }
  ]
}
```

`num_classical_bits` 可省略；省略时按线路中出现的最大经典位索引自动确定。所有索引、门名称、字段和条件会在运行前完整校验，错误时不开始执行线路。

操作格式：

- `{"gate": "H", "qubit": 0}`
- `{"gate": "X", "qubit": 0}`
- `{"gate": "S", "qubit": 0}`
- `{"gate": "CNOT", "control": 0, "target": 1}`
- `{"gate": "measure", "qubit": 0, "classical_bit": 0}`
- 任意操作都可以附带 `"condition": {"classical_bit": 0, "equals": 1}`；条件不满足时量子态和经典位均保持不变。

`CNOT` 的控制位和目标位必须不同。

## 位序

量子位编号从 `0` 开始。量子位 `0` 对应状态整数索引的最低有效位。例如双量子位索引：

- `0b00` 是 `|q1=0, q0=0⟩`
- `0b01` 是 `|q1=0, q0=1⟩`
- `0b10` 是 `|q1=1, q0=0⟩`
- `0b11` 是 `|q1=1, q0=1⟩`

输出位串按最高位量子位到最低位量子位排列。双量子位串 `"10"` 表示 `q1=1, q0=0`，对应内部索引 `0b10`。

## 命令行用法

单次诊断：

```powershell
python -m quantum_sim examples/entanglement.json --seed 42
```

输出包含：

- `measurements`：每个中途测量的位置、量子位、经典位、抽样结果和测量前该结果的概率。
- `state_vector`：执行完线路、最终全位测量前的复数状态向量，元素按内部索引 `0` 到 `2^n-1` 排列。
- `state_vector_norm_squared`：诊断状态向量的归一化检查值，正常应接近 `1.0`。
- `final_bitstring`：诊断运行结束时对全部量子位计算基测量得到的位串。

多次采样：

```powershell
python -m quantum_sim examples/entanglement.json --shots 1000 --seed 7
```

输出 `counts` 与 `frequencies`。为便于检查分布，所有 `2^n` 个位串都会出现，零频也保留。

示例：

- `examples/phase_interference.json`：展示 `S` 门保持的 `i` 相位以及第二次 `H` 后的相位干涉。
- `examples/entanglement.json`：`H` 加 `CNOT` 生成贝尔态，最终只出现 `"00"` 和 `"11"`。
- `examples/feedback_correction.json`：中途测量后，当经典位为 `1` 时施加 `X`，多次采样最终都为 `"0"`。

## 浮点容差

模拟器默认归一化容差为 `1e-10`。每个门后检查状态向量长度以及概率和是否接近 `1`；中途测量仅保留抽样结果对应的分支，并将该分支除以其概率平方根。浮点结果通常存在 `1e-15` 到 `1e-12` 量级误差，因此断言分布或相位时应使用容差比较，而不是要求十进制值完全相等。

## 运行测试

```powershell
python -m unittest discover -s tests
```
