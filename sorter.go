package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
)

// Stats 为处理统计。
type Stats struct {
	Read    int64 `json:"read"`    // 读取的记录数
	Kept    int64 `json:"kept"`    // 保留的记录数
	Dropped int64 `json:"dropped"` // 去重删除的记录数
}

// chunk 为磁盘上的临时有序块。
type chunk struct {
	path  string
	count int
}

// sorter 执行外排。
type sorter struct {
	rules  *Rules
	cmp    *comparator
	tmpDir string
}

// run 读取全部输入文件，分块排序后多轮归并，输出到 tmpOut，返回统计。
// 调用方负责在成功后把 tmpOut 原子替换为目标文件。
func (s *sorter) run(inputs []string, tmpOut string) (*Stats, error) {
	chunks, header, stats, err := s.buildChunks(inputs)
	if err != nil {
		cleanupChunks(chunks...)
		return nil, err
	}
	final, err := s.mergeAll(chunks)
	if err != nil {
		return nil, err
	}
	defer cleanupChunks(final)
	if err := s.writeOutput(final, header, tmpOut, stats); err != nil {
		return nil, err
	}
	return stats, nil
}

// buildChunks 按 memoryRecords 上限分块，排序后写入临时文件。
func (s *sorter) buildChunks(inputs []string) ([]*chunk, []string, *Stats, error) {
	stats := &Stats{}
	var header []string
	var chunks []*chunk
	buf := make([]*record, 0, s.rules.MemoryRecords)
	var seq int64

	flush := func() error {
		if len(buf) == 0 {
			return nil
		}
		sort.SliceStable(buf, func(i, j int) bool { return s.cmp.compare(buf[i], buf[j]) < 0 })
		c, err := s.writeChunk(buf)
		if err != nil {
			return err
		}
		chunks = append(chunks, c)
		buf = buf[:0]
		return nil
	}

	for _, path := range inputs {
		f, err := os.Open(path)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("打开输入文件失败: %w", err)
		}
		r := csv.NewReader(f)
		r.FieldsPerRecord = -1
		h, err := r.Read()
		if err != nil {
			f.Close()
			return nil, nil, nil, fmt.Errorf("读取表头失败 %s: %w", path, err)
		}
		if header == nil {
			header = h
			cols, err := resolveColumns(header, s.rules.Keys)
			if err != nil {
				f.Close()
				return nil, nil, nil, err
			}
			s.cmp.cols = cols
		} else if !equalStrings(header, h) {
			f.Close()
			return nil, nil, nil, fmt.Errorf("表头不一致: 文件 %s 的表头与首个输入文件不同", path)
		}
		var recNo int64
		for {
			row, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				f.Close()
				return nil, nil, nil, fmt.Errorf("解析 CSV 失败 %s 第 %d 条记录: %w", path, recNo+1, err)
			}
			recNo++
			if len(row) != len(header) {
				f.Close()
				return nil, nil, nil, fmt.Errorf("列数不符: 文件 %s 第 %d 条记录有 %d 列，期望 %d 列", path, recNo, len(row), len(header))
			}
			for i, k := range s.rules.Keys {
				if k.Type == "decimal" {
					if err := validateDecimal(row[s.cmp.cols[i]], path, recNo); err != nil {
						f.Close()
						return nil, nil, nil, err
					}
				}
			}
			seq++
			stats.Read++
			buf = append(buf, &record{fields: row, seq: seq})
			if len(buf) >= s.rules.MemoryRecords {
				if err := flush(); err != nil {
					f.Close()
					return nil, nil, nil, err
				}
			}
		}
		f.Close()
	}
	if header == nil {
		return nil, nil, nil, fmt.Errorf("没有输入文件")
	}
	if err := flush(); err != nil {
		return nil, nil, nil, err
	}
	return chunks, header, stats, nil
}

// writeChunk 把有序记录写入临时块文件（CSV 格式，首列为全局序号）。
func (s *sorter) writeChunk(recs []*record) (*chunk, error) {
	f, err := os.CreateTemp(s.tmpDir, "chunk-*.csv")
	if err != nil {
		return nil, fmt.Errorf("创建临时块失败: %w", err)
	}
	w := csv.NewWriter(f)
	for _, rec := range recs {
		if err := w.Write(encodeChunkRow(rec)); err != nil {
			f.Close()
			return nil, fmt.Errorf("写入临时块失败: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return nil, fmt.Errorf("写入临时块失败: %w", err)
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		return nil, err
	}
	return &chunk{path: name, count: len(recs)}, nil
}

// writeOutput 流式读取最终有序块，按需去重后写出 CSV。
// 去重保留排序键全等记录中原始次序最早的一条（稳定排序保证其最先出现）。
func (s *sorter) writeOutput(final *chunk, header []string, tmpOut string, stats *Stats) error {
	out, err := os.Create(tmpOut)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	w := csv.NewWriter(out)
	if err := w.Write(header); err != nil {
		out.Close()
		return fmt.Errorf("写入输出失败: %w", err)
	}
	if final != nil {
		f, err := os.Open(final.path)
		if err != nil {
			out.Close()
			return fmt.Errorf("打开临时块失败: %w", err)
		}
		defer f.Close()
		r := csv.NewReader(f)
		r.FieldsPerRecord = -1
		var prev *record
		for {
			row, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				out.Close()
				return fmt.Errorf("读取临时块失败: %w", err)
			}
			rec, err := decodeChunkRow(row)
			if err != nil {
				out.Close()
				return err
			}
			if s.rules.Dedup && prev != nil && s.cmp.keyEqual(prev, rec) {
				stats.Dropped++
				continue
			}
			if err := w.Write(rec.fields); err != nil {
				out.Close()
				return fmt.Errorf("写入输出失败: %w", err)
			}
			stats.Kept++
			prev = rec
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		out.Close()
		return fmt.Errorf("写入输出失败: %w", err)
	}
	return out.Close()
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// cleanupChunks 删除本次运行创建的临时块文件。
func cleanupChunks(chunks ...*chunk) {
	for _, c := range chunks {
		if c != nil {
			os.Remove(c.path)
		}
	}
}
