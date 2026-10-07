# morse-audio-decoder

纯 Python 标准库工具：从单声道 16 位 PCM WAV 录音中解码国际摩尔斯电码
（英文字母与数字），输出大写文本，并导出可回查原录音的 JSON 时间线诊断。
不调用任何在线识别服务或第三方音频库。

## 运行示例

```sh
python -m morse_decoder.cli recording.wav params.json \
    --segments-out diag.json --text-out message.txt
```

参数文件 `params.json` 示例：

```json
{
  "window_seconds": 0.01,
  "onset_threshold": 0.3,
  "offset_threshold": 0.15,
  "dot_seconds": 0.06,
  "tolerance": 0.4
}
```

## 参数与时间单位

- 所有时长（`window_seconds`、`dot_seconds`）均以**秒**为单位；
  `tolerance` 为相对于点时长的比例，须满足 `0 <= tolerance < 1`。
- 样本按满幅（32768）归一化到 [-1, 1]，阈值作用于归一化后的 RMS 幅值，
  须满足 `0 <= offset_threshold < onset_threshold <= 1`。
- 窗长不得大于点时长的五分之一，否则拒绝运行。
- 有声/静音判定带迟滞：RMS 高于启声阈值才进入有声，低于停声阈值才
  回到静音，介于两者之间保持原状态。连续同状态窗口合并为时间段，
  最后不足一窗的数据同样参与判定；全静音录音返回空文本。

## 计时规则

点 = 1 单元，划 = 3 单元，码元内间隔 = 1 单元，字符间隔 = 3 单元，
词间隔 ≥ 7 单元。忽略开头与末尾的静音，文件结束时仍结算最后一个字符。
时长无法在容差内唯一解释、或遇到未知码组时，诊断 JSON 的 `errors`
字段记录对应时间范围与原因，不生成成功译文（退出码 1）。

## 窗口量化误差

时间段边界按窗口对齐，量化误差最大为一个窗长（`window_seconds`）。
因此窗长越小边界越精确，但窗口过短会使 RMS 估计不稳定；限制为点时长
的五分之一可在两者之间取得平衡。诊断中所有时间戳单位为秒，取自原录音。

## 输出

- `--segments-out`：JSON 诊断，含采样率、生效配置、有声/静音分段、
  每个字符的码组与音频起止时间、错误列表及最终文本。
- `--text-out`：纯文本译文。两个输出路径若已存在则拒绝覆盖（退出码 2）。
  非法音频格式（非单声道 16 位 PCM）或非法阈值配置同样以退出码 2 拒绝。

## 适用范围

面向干净的电报码录音；不要求识别混合语音或强噪声录音，也不做自动测速。

## 测试

```sh
python -m unittest discover
```
