# Prismatic Hull Hydrostatics

静水核算工具：对棱柱浮体（单位长度）在不同横倾角下求解平衡位置，
并判断浮力矩是扶正还是加剧倾斜。仅依赖 Python 标准库。

## 用法

```
python -m hull_hydrostatics config.json --csv results.csv --diag diagnostics.json
```

### 输入 JSON

```json
{
  "polygon": [[-1, 0], [1, 0], [1, 1], [-1, 1]],
  "center_of_gravity": [0.0, 0.5],
  "unit_length_mass": 1.0,
  "fluid_density": 1.0,
  "gravity": 9.81,
  "angles_deg": [0, 10, 20, 30],
  "moment_neutral_tolerance": 1e-9
}
```

- `polygon`：封闭简单多边形顶点（无孔），允许凹形，两种绕序均可；
  自交或零面积会被拒绝。
- `unit_length_mass`：单位长度质量，必须严格大于 0 且小于满浸排水质量
  （`fluid_density × 截面面积`）。
- `fluid_density`、`gravity`：必须为正。
- `moment_neutral_tolerance`：力矩绝对值不超过该值时标记为 `neutral`。

### 坐标约定

身体坐标 x 水平向右、y 竖直向上，正横倾角为逆时针。每个角度下，
截面与重心一起旋转，再求竖向平移使水面（固定在 y = 0）以下的面积
产生的浮力等于重量。

### 输出

- CSV：每个角度一行，含平衡平移量、浮心/重心位置、排水质量、
  浮力、关于重心的力矩、平衡残差与行为标记
  （`righting` / `aggravating` / `neutral`）。求解失败的角度
  `status` 为 `failed` 并给出 `error`。
- 诊断 JSON：截面面积与形心、物性参数及逐角度详细结果
  （含面积残差）。

力矩按单位长度计算：`M = 浮力 × (浮心x − 重心x)`，逆时针为正。
力矩与非零横倾角反号标记 `righting`，同号标记 `aggravating`；
`neutral` 只表示力矩接近零，**不**意味着稳定。

浮心与排水量始终由实际浸没多边形（水线裁剪后的区域）计算，
不使用整截面形心替代。各角度独立求解，输入角度的采样顺序
不影响任何角度的物理结果。

## 假设与限制

静水平衡：不计波浪、航速、进水（舱室始终完整）以及纵向变化
（棱柱体，按单位长度分析）。

## 测试

```
python -m unittest discover -s tests
```
