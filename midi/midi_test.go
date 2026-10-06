package midi

import (
	"bytes"
	"strings"
	"testing"
)

// smfBuilder 在测试中构造标准 MIDI 文件字节。
type smfBuilder struct {
	format   uint16
	division uint16
	tracks   [][]byte
}

func newSMF(format, division uint16) *smfBuilder {
	return &smfBuilder{format: format, division: division}
}

func (b *smfBuilder) track(events ...[]byte) *smfBuilder {
	var tr []byte
	for _, e := range events {
		tr = append(tr, e...)
	}
	b.tracks = append(b.tracks, tr)
	return b
}

func (b *smfBuilder) build() []byte {
	var out bytes.Buffer
	out.WriteString("MThd")
	out.Write([]byte{0, 0, 0, 6})
	out.Write([]byte{byte(b.format >> 8), byte(b.format)})
	out.Write([]byte{byte(len(b.tracks) >> 8), byte(len(b.tracks))})
	out.Write([]byte{byte(b.division >> 8), byte(b.division)})
	for _, tr := range b.tracks {
		out.WriteString("MTrk")
		out.Write([]byte{byte(len(tr) >> 24), byte(len(tr) >> 16), byte(len(tr) >> 8), byte(len(tr))})
		out.Write(tr)
	}
	return out.Bytes()
}

// ev 构造一条事件：delta（变长）+ 载荷。
func ev(delta uint32, payload ...byte) []byte {
	return append(varlenBytes(delta), payload...)
}

func varlenBytes(v uint32) []byte {
	var tmp [4]byte
	i := len(tmp)
	tmp[i-1] = byte(v & 0x7F)
	i--
	for v >>= 7; v > 0; v >>= 7 {
		i--
		tmp[i] = byte(v&0x7F) | 0x80
	}
	return tmp[i:]
}

func meta(delta uint32, typ byte, data ...byte) []byte {
	p := []byte{0xFF, typ}
	p = append(p, varlenBytes(uint32(len(data)))...)
	p = append(p, data...)
	return ev(delta, p...)
}

func endOfTrack(delta uint32) []byte { return meta(delta, MetaEndOfTrack) }

func tempoEv(delta uint32, us uint32) []byte {
	return meta(delta, MetaTempo, byte(us>>16), byte(us>>8), byte(us))
}

func noteOn(delta uint32, ch, pitch, vel byte) []byte {
	return ev(delta, 0x90|ch, pitch, vel)
}

func noteOff(delta uint32, ch, pitch, vel byte) []byte {
	return ev(delta, 0x80|ch, pitch, vel)
}

func cc(delta uint32, ch, ctrl, val byte) []byte {
	return ev(delta, 0xB0|ch, ctrl, val)
}

func mustParse(t *testing.T, data []byte) *File {
	t.Helper()
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	return f
}

func TestParseFormat0And1(t *testing.T) {
	f0 := newSMF(0, 480).track(
		noteOn(0, 0, 60, 100), noteOff(480, 0, 60, 0), endOfTrack(0),
	).build()
	f := mustParse(t, f0)
	if f.Format != 0 || f.Division != 480 {
		t.Fatalf("格式/时基错误: %d %d", f.Format, f.Division)
	}

	f1 := newSMF(1, 480).
		track(tempoEv(0, 600000), endOfTrack(0)).
		track(noteOn(0, 1, 64, 90), noteOff(240, 1, 64, 0), endOfTrack(0)).
		build()
	f = mustParse(t, f1)
	if len(f.Events) != 5 {
		t.Fatalf("事件数 = %d, 期望 5", len(f.Events))
	}
}

func TestRejectFormat2AndSMPTE(t *testing.T) {
	f2 := newSMF(2, 480).track(endOfTrack(0)).build()
	if _, err := Parse(f2); err == nil || !strings.Contains(err.Error(), "二型") {
		t.Fatalf("应拒绝二型: %v", err)
	}
	smpte := newSMF(1, 0xE728).track(endOfTrack(0)).build()
	if _, err := Parse(smpte); err == nil || !strings.Contains(err.Error(), "SMPTE") {
		t.Fatalf("应拒绝 SMPTE: %v", err)
	}
}

func TestRunningStatus(t *testing.T) {
	// 第二个音符使用运行状态（省略状态字节）。
	tr := bytes.Join([][]byte{
		noteOn(0, 0, 60, 100),
		ev(240, 62, 100), // 运行状态 noteOn
		ev(0, 60, 0),     // 运行状态零力度 = 关闭
		noteOff(240, 0, 62, 0),
		endOfTrack(0),
	}, nil)
	data := newSMF(0, 480).track(tr).build()
	f := mustParse(t, data)
	res := Analyze(f)
	if len(res.Notes) != 2 {
		t.Fatalf("音符数 = %d, 期望 2", len(res.Notes))
	}
	if len(res.Anomalies) != 0 {
		t.Fatalf("不应有异常: %+v", res.Anomalies)
	}
}

func TestSkipUnknownMetaAndSysex(t *testing.T) {
	tr := bytes.Join([][]byte{
		meta(0, 0x03, []byte("piano")...),   // 轨道名，跳过
		ev(0, 0xF0, 0x03, 0x01, 0x02, 0x03), // SysEx，跳过
		ev(0, 0xF7, 0x02, 0xAA, 0xBB),       // 转义系统事件，跳过
		noteOn(0, 0, 60, 100),
		noteOff(480, 0, 60, 0),
		endOfTrack(0),
	}, nil)
	f := mustParse(t, newSMF(0, 480).track(tr).build())
	res := Analyze(f)
	if len(res.Notes) != 1 || res.Notes[0].OffTick != 480 {
		t.Fatalf("音符还原错误: %+v", res.Notes)
	}
}

func TestTruncationErrors(t *testing.T) {
	// 轨道声明长度越界
	bad := newSMF(0, 480).track(noteOn(0, 0, 60, 100)).build()
	bad[len(bad)-9] = 0x7F // 篡改 MTrk 长度末字节
	if _, err := Parse(bad); err == nil {
		t.Fatal("应报轨道长度越界")
	}
	// 事件中途截断
	trunc := newSMF(0, 480).track(noteOn(0, 0, 60, 100)).build()
	if _, err := Parse(trunc[:len(trunc)-2]); err == nil {
		t.Fatal("应报截断")
	}
	// 变长整数超 4 字节
	badVar := newSMF(0, 480).track(ev(0, 0x90, 60, 100), []byte{0x81, 0x81, 0x81, 0x81, 0x01, 0x80, 60, 0}).build()
	if _, err := Parse(badVar); err == nil {
		t.Fatal("应报变长整数过长")
	}
}

func TestTempoIntegration(t *testing.T) {
	// division=480；tick 0 速度 500000us，tick 480 改为 250000us。
	// tick 960 的秒数 = 0.5 + 0.25 = 0.75。
	data := newSMF(1, 480).
		track(tempoEv(0, 500000), tempoEv(480, 250000), endOfTrack(0)).
		track(noteOn(0, 0, 60, 100), noteOff(960, 0, 60, 0), endOfTrack(0)).
		build()
	res := Analyze(mustParse(t, data))
	if len(res.Notes) != 1 {
		t.Fatalf("音符数 = %d", len(res.Notes))
	}
	if got := res.Notes[0].EndSec; got != 0.75 {
		t.Fatalf("结束秒 = %v, 期望 0.75", got)
	}
	// 同 tick 最后一个速度生效
	data2 := newSMF(1, 480).
		track(tempoEv(0, 500000), tempoEv(0, 250000), endOfTrack(960)).
		build()
	res2 := Analyze(mustParse(t, data2))
	if got := res2.EndSec; got != 0.5 {
		t.Fatalf("同 tick 覆盖后结束秒 = %v, 期望 0.5", got)
	}
}

func TestOverlappingSamePitchFIFO(t *testing.T) {
	// 同键两次开启，两次关闭按 FIFO 配对。
	data := newSMF(0, 480).track(
		noteOn(0, 0, 60, 100),
		noteOn(240, 0, 60, 90),
		noteOff(240, 0, 60, 0), // tick 480，关闭第一个
		noteOff(240, 0, 60, 0), // tick 720，关闭第二个
		endOfTrack(0),
	).build()
	res := Analyze(mustParse(t, data))
	if len(res.Notes) != 2 {
		t.Fatalf("音符数 = %d", len(res.Notes))
	}
	first, second := res.Notes[0], res.Notes[1]
	if first.OffTick != 480 || first.Velocity != 100 {
		t.Fatalf("第一个音符: %+v", first)
	}
	if second.OffTick != 720 || second.Velocity != 90 {
		t.Fatalf("第二个音符: %+v", second)
	}
}

func TestSustainPedal(t *testing.T) {
	// 踩下踏板时抬键只记抬键，松开踏板才结束；
	// 延音期间重新开启同音高不得吞掉之前的延音。
	data := newSMF(0, 480).track(
		cc(0, 0, CCSustain, 127), // 踩下
		noteOn(0, 0, 60, 100),    // tick 0 开
		noteOff(240, 0, 60, 0),   // tick 240 抬键（延住）
		noteOn(240, 0, 60, 90),   // tick 480 再开同音高
		noteOff(240, 0, 60, 0),   // tick 720 抬键（延住）
		cc(240, 0, CCSustain, 0), // tick 960 松开，两个音符同时结束
		endOfTrack(0),
	).build()
	res := Analyze(mustParse(t, data))
	if len(res.Notes) != 2 {
		t.Fatalf("音符数 = %d", len(res.Notes))
	}
	for _, n := range res.Notes {
		if !n.PedalHeld {
			t.Fatalf("应标记踏板延音: %+v", n)
		}
		if n.EndTick != 960 {
			t.Fatalf("声音结束 tick = %d, 期望 960: %+v", n.EndTick, n)
		}
	}
	if res.Notes[0].OffTick != 240 || res.Notes[1].OffTick != 720 {
		t.Fatalf("抬键 tick 错误: %+v", res.Notes)
	}
}

func TestPedalPerTrackChannel(t *testing.T) {
	// 通道 0 踩踏板不影响通道 1。
	data := newSMF(0, 480).track(
		cc(0, 0, CCSustain, 127),
		noteOn(0, 0, 60, 100),
		noteOn(0, 1, 60, 100),
		noteOff(240, 0, 60, 0), // 通道 0 延住
		noteOff(0, 1, 60, 0),   // 通道 1 立即结束
		cc(240, 0, CCSustain, 0),
		endOfTrack(0),
	).build()
	res := Analyze(mustParse(t, data))
	if len(res.Notes) != 2 {
		t.Fatalf("音符数 = %d", len(res.Notes))
	}
	for _, n := range res.Notes {
		if n.Channel == 1 && n.EndTick != 240 {
			t.Fatalf("通道 1 不应被延音: %+v", n)
		}
		if n.Channel == 0 && n.EndTick != 480 {
			t.Fatalf("通道 0 应延到 480: %+v", n)
		}
	}
}

func TestAnomalies(t *testing.T) {
	// 无匹配关闭 + 文件结束未关闭（截到全文件最大 tick）。
	data := newSMF(0, 480).track(
		noteOff(0, 0, 64, 0),   // 无匹配关闭
		noteOn(0, 0, 60, 100),  // 永不关闭
		noteOn(480, 0, 62, 90), // tick 480 开
		noteOff(480, 0, 62, 0), // tick 960 关，全文件最大 tick = 960
		endOfTrack(0),
	).build()
	res := Analyze(mustParse(t, data))
	var unmatched, unclosed int
	for _, a := range res.Anomalies {
		switch a.Type {
		case "unmatched_note_off":
			unmatched++
		case "unclosed_note":
			unclosed++
		}
	}
	if unmatched != 1 || unclosed != 1 {
		t.Fatalf("异常统计错误: %+v", res.Anomalies)
	}
	for _, n := range res.Notes {
		if n.Pitch == 60 {
			if !n.Truncated || n.EndTick != 960 {
				t.Fatalf("未关闭音符应截到 960: %+v", n)
			}
		}
	}
}

func TestPedalHeldAtFileEnd(t *testing.T) {
	// 踏板未松开，延音音符截到文件结束。
	data := newSMF(0, 480).track(
		cc(0, 0, CCSustain, 127),
		noteOn(0, 0, 60, 100),
		noteOff(240, 0, 60, 0),
		noteOn(240, 0, 62, 90), // tick 480，用于撑大文件结束
		noteOff(480, 0, 62, 0), // tick 960
		endOfTrack(0),
	).build()
	res := Analyze(mustParse(t, data))
	for _, n := range res.Notes {
		if n.Pitch == 60 && n.EndTick != 960 {
			t.Fatalf("踏板延音应截到 960: %+v", n)
		}
	}
}

func TestMaxPolyphony(t *testing.T) {
	// 三个音符：A[0,480) B[240,720) C[480,480)（零时长不计）。
	data := newSMF(0, 480).track(
		noteOn(0, 0, 60, 100),
		noteOn(240, 0, 62, 100),
		noteOn(240, 0, 64, 100), // tick 480 开
		noteOff(0, 0, 64, 0),    // tick 480 关，零时长
		noteOff(0, 0, 60, 0),    // tick 480 关 A
		noteOff(240, 0, 62, 0),  // tick 720 关 B
		endOfTrack(0),
	).build()
	res := Analyze(mustParse(t, data))
	if res.MaxPolyphony[0] != 2 {
		t.Fatalf("最大同时发声数 = %d, 期望 2", res.MaxPolyphony[0])
	}
}

func TestOutputFormats(t *testing.T) {
	data := newSMF(0, 480).track(
		noteOn(0, 0, 60, 100), noteOff(480, 0, 60, 0), endOfTrack(0),
	).build()
	res := Analyze(mustParse(t, data))
	var csvBuf, jsonBuf bytes.Buffer
	if err := WriteCSV(&csvBuf, res); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(csvBuf.String()), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "track,channel") {
		t.Fatalf("CSV 输出错误: %q", csvBuf.String())
	}
	if !strings.Contains(lines[1], "0,0,60,100,0,480,480,0.000000,0.500000,0.500000") {
		t.Fatalf("CSV 行错误: %q", lines[1])
	}
	if err := WriteJSON(&jsonBuf, res); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(jsonBuf.String(), `"anomalies": []`) {
		t.Fatalf("JSON 输出错误: %q", jsonBuf.String())
	}
}
