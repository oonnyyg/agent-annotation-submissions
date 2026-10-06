# crossed-gage-study

交叉型量具 R&R 分析工具（仅 Python 标准库）。基于含交互项的平衡双因素随机
效应 ANOVA 模型，把总波动分解为零件差异、操作者差异、零件×操作者交互和
重复测量噪声，避免把总波动直接当成量具误差。

## 用法

```
python -m gage_study measurements.csv config.json --out-dir out
```

输出四个文件到 `--out-dir`（默认 `gage_study_out`）：

- `anova.csv`：各来源（part / operator / part_by_operator / repeatability）
  的平方和、自由度、均方，以及合计行。
- `variance_components.csv`：矩估计方差分量。`variance_estimate` 为原始估计
  （可能为负），`variance_used` 为汇总时使用的值（负值取零），另含方差贡献率
  `contribution_pct`（分量方差 / 总方差）与标准差占比 `stddev_pct`
  （分量标准差 / 总标准差）。两者口径不同，不能混用；总方差为零时留空。
- `means.json`：每个零件×操作者组合的均值与重复残差。
- `diagnostics.json`：观测数、平方和分解核对残差、负方差估计列表及警告。

## 输入格式

CSV 每行一条观测，由零件、操作者、重复编号标识。列名通过 JSON 配置指定
（缺省如下）：

```json
{
  "part_column": "part",
  "operator_column": "operator",
  "replicate_column": "replicate",
  "value_column": "value"
}
```

数据要求（不满足即拒绝，退出码 2，不自动补齐、不删除离群点）：

- 至少 2 个零件、2 个操作者；
- 每个零件×操作者组合至少 2 次重复，且各组合重复次数相同（平衡设计）；
- 同一组合内重复编号不得重复，不允许缺格；
- 测量值必须为有限数值。

输入行次序不影响结果。

## 模型与假设

模型：Y_ijk = μ + P_i + O_j + (PO)_ij + ε_ijk，其中 P、O、PO、ε 均为随机
效应，方差分别为 σ²_P、σ²_O、σ²_PO、σ²_ε。设 a 个零件、b 个操作者、每格
n 次重复，期望均方为：

- E[MS_P] = σ²_ε + n·σ²_PO + b·n·σ²_P
- E[MS_O] = σ²_ε + n·σ²_PO + a·n·σ²_O
- E[MS_PO] = σ²_ε + n·σ²_PO
- E[MS_E] = σ²_ε

由矩估计解出四个方差分量。负估计保留原值并给出警告，汇总时以零参与
（不取绝对值、不改模型）。汇总定义：

- 再现性方差 = σ²_O + σ²_PO
- 量具方差 = 再现性 + σ²_ε
- 总方差 = 量具 + σ²_P

不做显著性筛选，不合并交互项。平方和分解恒等式
（SS_P + SS_O + SS_PO + SS_E = SS_T）在诊断中核对。

## 单位与解释

所有方差分量的单位是测量值单位的平方，标准差占比与方差贡献率为无量纲
百分比。本工具只输出分解结果与诊断，不用固定阈值代替业务验收结论——
量具是否可接受应结合过程公差与业务要求判断。

## 测试

```
python -m unittest discover -s tests
```
