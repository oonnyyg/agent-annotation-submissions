package main

import (
	"container/heap"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
)

// mergeItem 为归并堆中的一个元素。
type mergeItem struct {
	rec *record
	src int
}

// mergeHeap 按 comparator 排序的最小堆；comparator 以 seq 兜底保证稳定。
type mergeHeap struct {
	items []mergeItem
	cmp   *comparator
}

func (h *mergeHeap) Len() int { return len(h.items) }
func (h *mergeHeap) Less(i, j int) bool {
	return h.cmp.compare(h.items[i].rec, h.items[j].rec) < 0
}
func (h *mergeHeap) Swap(i, j int) { h.items[i], h.items[j] = h.items[j], h.items[i] }
func (h *mergeHeap) Push(x any)    { h.items = append(h.items, x.(mergeItem)) }
func (h *mergeHeap) Pop() any {
	old := h.items
	n := len(old)
	it := old[n-1]
	h.items = old[:n-1]
	return it
}

// mergeAll 以不超过 mergeFanIn 的扇入多轮归并，直到只剩一个有序块。
// 每轮只同时打开当轮参与归并的块文件。
func (s *sorter) mergeAll(chunks []*chunk) (*chunk, error) {
	for len(chunks) > 1 {
		var next []*chunk
		fan := s.rules.MergeFanIn
		for i := 0; i < len(chunks); i += fan {
			end := i + fan
			if end > len(chunks) {
				end = len(chunks)
			}
			group := chunks[i:end]
			if len(group) == 1 {
				next = append(next, group[0])
				continue
			}
			merged, err := s.mergeGroup(group)
			cleanupChunks(group...)
			if err != nil {
				cleanupChunks(next...)
				cleanupChunks(chunks[end:]...)
				return nil, err
			}
			next = append(next, merged)
		}
		chunks = next
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	return chunks[0], nil
}

// mergeGroup 归并一组有序块为一个新的有序块。
// 块文件首列为全局原始序号 seq，归并时读回以维持跨块稳定次序。
func (s *sorter) mergeGroup(group []*chunk) (*chunk, error) {
	readers := make([]*csv.Reader, len(group))
	files := make([]*os.File, len(group))
	for i, c := range group {
		f, err := os.Open(c.path)
		if err != nil {
			for j := 0; j < i; j++ {
				files[j].Close()
			}
			return nil, fmt.Errorf("打开临时块失败: %w", err)
		}
		files[i] = f
		readers[i] = csv.NewReader(f)
		readers[i].FieldsPerRecord = -1
	}
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()

	out, err := os.CreateTemp(s.tmpDir, "chunk-*.csv")
	if err != nil {
		return nil, fmt.Errorf("创建临时块失败: %w", err)
	}
	w := csv.NewWriter(out)

	h := &mergeHeap{cmp: s.cmp}
	next := func(src int) error {
		row, err := readers[src].Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("读取临时块失败: %w", err)
		}
		rec, err := decodeChunkRow(row)
		if err != nil {
			return err
		}
		heap.Push(h, mergeItem{rec: rec, src: src})
		return nil
	}
	for i := range readers {
		if err := next(i); err != nil {
			out.Close()
			return nil, err
		}
	}

	count := 0
	for h.Len() > 0 {
		it := heap.Pop(h).(mergeItem)
		if err := w.Write(encodeChunkRow(it.rec)); err != nil {
			out.Close()
			return nil, fmt.Errorf("写入临时块失败: %w", err)
		}
		count++
		if err := next(it.src); err != nil {
			out.Close()
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		out.Close()
		return nil, fmt.Errorf("写入临时块失败: %w", err)
	}
	name := out.Name()
	if err := out.Close(); err != nil {
		return nil, err
	}
	return &chunk{path: name, count: count}, nil
}

// encodeChunkRow 把记录编码为块文件行：首列为 seq。
func encodeChunkRow(rec *record) []string {
	row := make([]string, 0, len(rec.fields)+1)
	row = append(row, strconv.FormatInt(rec.seq, 10))
	row = append(row, rec.fields...)
	return row
}

// decodeChunkRow 解码块文件行，还原 seq 与字段。
func decodeChunkRow(row []string) (*record, error) {
	if len(row) < 1 {
		return nil, fmt.Errorf("临时块损坏: 空行")
	}
	seq, err := strconv.ParseInt(row[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("临时块损坏: 非法序号 %q", row[0])
	}
	return &record{fields: row[1:], seq: seq}, nil
}
