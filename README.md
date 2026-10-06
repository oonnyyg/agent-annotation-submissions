# crossed-gage-study

交叉型量具 R&R 分析工具（纯 Python 标准库）。基于含交互项的平衡双因素随机效应
模型，把总波动分解为零件差异、操作者差异、零件×操作者交互和重复测量噪声，
避免把总波动直接当成量具误差。

## 用法

```
python -m gage_study measurements.csv config.json --outdir out
```

### 测量 CSV

默认列名 `part,operator,replicate,value`（可在配置中改名）。每行一个观测，
由零件、操作者和重复编号共同标识。要求：

- 至少 2 个零件、2 个操作者；
- 每个零件×操作者组合至少 2 次重复，且各组合重复次数相同（平衡设计）；
- 缺格、重复编号、非数值或非有限值（NaN/Inf）都会报错拒绝，
  工具不会自动补齐或删除离群点。

### 配置 JSON

```json
{"columns": {"part": "part", "operator": "operator",
             "replicate": "replicate", "value": "value"}}
```

空对象 `{}` 表示全部使用默认列名。

### 输出

- `anova.csv`：各来源（part / operator / part_operator / repeatability /
  total）的平方和、自由度、均方。
- `variance_components.csv`：方差分量矩估计及汇总（reproducibility、gage、
  total），含方差贡献率 `percent_variance` 与标准差占比 `percent_stddev`。
- `diagnostics.json`：各组合均值、逐观测残差、平方和核对结果与警告。

## 模型与假设

模型：`y_ijk = μ + P_i + O_j + (PO)_ij + ε_ijk`，其中零件、操作者、交互和
重复误差均为独立随机效应，方差分别为 σ²_p、σ²_o、σ²_po、σ²_e。
期望均方：E[MS_p] = σ²_e + nσ²_po + bnσ²_p，E[MS_o] = σ²_e + nσ²_po + anσ²_o，
E[MS_po] = σ²_e + nσ²_po，E[MS_e] = σ²_e（a/b/n 为零件数/操作者数/重复数）。
由此矩估计各方差分量；不做显著性筛选，也不合并交互项。

- 再现性方差 = σ²_o + σ²_po；量具方差 = 再现性 + σ²_e；总方差 = 量具 + σ²_p。
- 负的方差估计保留原值输出并给出警告，汇总时按 0 计；不取绝对值、不改模型。
- `percent_variance` 是方差贡献率，`percent_stddev` 是标准差占比
  （√(分量/总量)），两者口径不同，不能混用。总方差为零时比例留空。
- 平方和核对：各来源平方和之和应等于总平方和，结果见诊断 JSON 的
  `ss_check`。

## 单位与结论

所有方差/标准差与输入测量值同单位（方差为单位的平方）。工具只输出数值
结果，不设固定阈值，也不代替业务验收结论；是否可接受需结合工艺公差与
公司规范判断。

## 测试

```
python -m unittest discover -s tests
```

覆盖纯重复噪声、明显交互、全常数数据、负方差估计、缺格/重复/不平衡拒绝、
输入次序不变性及 CLI 端到端。
