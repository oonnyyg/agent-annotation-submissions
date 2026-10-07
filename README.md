# ranked-set-enrichment

离线特征集合富集分析工具（仅依赖 Python 标准库）。判断预定义特征集合
是否集中在评分榜单的顶部或底部，输出可追溯的富集曲线与统计摘要，
全程不连接任何在线数据库。本工具不是完整 GSEA 软件的复刻，只实现
本文所述的单侧游走统计量与双侧置换检验口径。

## 安装与运行

需要 Python >= 3.12，无第三方依赖。

```bash
python -m ranked_set_enrichment \
    --scores scores.csv \
    --sets sets.json \
    --outdir out \
    --permutations 1000 \
    --seed 1
```

安装为包后也可使用 `ranked-set-enrichment` 命令（见 `pyproject.toml`
的 `[project.scripts]`）。

## 输入

- `--scores`：CSV，必须包含 `id,score` 两列。特征标识必须唯一，
  评分必须为有限数值（拒绝 `inf`、`nan`、非数值）。内部按评分降序
  排列，同分按标识字典序排列，保证排序确定。
- `--sets`：JSON。支持两种形式：`{"集合名": ["成员", ...]}` 或
  `[{"name": ..., "members": [...]}, ...]`。成员为字符串列表。

## 集合预处理

- 集合内重复成员按首次出现去重，并在摘要的 `duplicate_members` 列报告。
- 不在榜单内的成员被忽略，并在 `missing_members` 列报告。
- 过滤后为空（`empty_after_filtering`）或覆盖整个榜单
  （`covers_entire_ranked_list`）的集合标记为不可计算：摘要中
  `computable=False`，统计列留空，不导出曲线。

## 富集分数

沿排序后的榜单累计游走：命中集合成员时增加 `|score|` 在集合内归一化
后的权重（若集合内命中权重全为零，则各成员均分增量 `1/H`）；未命中时
等额减少 `1/(N-H)`。取游走绝对值最大的值作为有符号富集分数（ES）：
正负并列时取正值，同值位置取最早者。ES 为正表示集中于榜首，为负表示
集中于榜尾。

## 置换检验（双侧）

评分与位置固定，每次置换均匀无放回地抽取与集合等大的位置集合，重新
计算 ES 形成零分布。双侧经验概率为

```
p = (1 + #{|ES_perm| >= |ES_obs|}) / (1 + permutations)
```

分子分母各加一，因此永远不会输出零概率。对全部有效集合的经验概率做
Benjamini–Hochberg 校正，输出 `q_value_bh`。

## 随机复现

置换使用 `random.Random(seed)`，每个集合独立以同一种子初始化，因此：

- 相同 `--seed` 与 `--permutations` 下结果完全可复现；
- 集合在 JSON 中的输入顺序、集合内成员顺序均不影响置换结果
  （成员先经过去重与位置集合化）。

## 输出

写入 `--outdir`：

- `summary.csv` / `summary.json`：每个集合一行，包含 `computable`、
  `reason`、`effective_size`、`enrichment_score`、`extremum_position`
  （0 基位置）、`p_value`、`q_value_bh`、`exceedances`、
  `missing_members`、`duplicate_members`；JSON 另含运行参数
  （置换次数、种子、榜单大小）以便追溯。
- `curves/<集合名>.csv`：逐位置游走曲线，列为
  `position,id,score,running_enrichment`，仅对可计算集合导出。

## 测试

```bash
python -m unittest discover -s tests
```
