package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Sheet 表示一张纸：Front 为正面页码，Back 为背面页码（0 表示留空）。
type Sheet struct {
	Front int `json:"front"`
	Back  int `json:"back"` // 0 = 空白
}

// parseRanges 解析选页范围（如 "1-3,5,7-8"），去重并按原文页码排序。
// 任一范围非法则整体拒绝。
func parseRanges(spec string, totalPages int) ([]int, error) {
	if totalPages <= 0 {
		return nil, fmt.Errorf("文档总页数必须为正数")
	}
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("选页范围不能为空")
	}
	seen := map[int]bool{}
	var pages []int
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		lo, hi, err := parseRangePart(part)
		if err != nil {
			return nil, err
		}
		if lo < 1 || hi > totalPages {
			return nil, fmt.Errorf("范围 %q 超出文档页数 1-%d", part, totalPages)
		}
		for p := lo; p <= hi; p++ {
			if !seen[p] {
				seen[p] = true
				pages = append(pages, p)
			}
		}
	}
	sort.Ints(pages)
	return pages, nil
}

func parseRangePart(part string) (int, int, error) {
	if part == "" {
		return 0, 0, fmt.Errorf("范围包含空片段")
	}
	if strings.Contains(part, "-") {
		pieces := strings.Split(part, "-")
		if len(pieces) != 2 {
			return 0, 0, fmt.Errorf("非法范围 %q", part)
		}
		lo, err1 := strconv.Atoi(strings.TrimSpace(pieces[0]))
		hi, err2 := strconv.Atoi(strings.TrimSpace(pieces[1]))
		if err1 != nil || err2 != nil || lo > hi || lo < 1 {
			return 0, 0, fmt.Errorf("非法范围 %q", part)
		}
		return lo, hi, nil
	}
	n, err := strconv.Atoi(part)
	if err != nil || n < 1 {
		return 0, 0, fmt.Errorf("非法页码 %q", part)
	}
	return n, n, nil
}

// buildSheets 按份编排纸张。每份从正面开始；
// 双面时奇数末页背面留空。
func buildSheets(pages []int, copies int, duplex bool) ([]Sheet, error) {
	if copies <= 0 {
		return nil, fmt.Errorf("份数必须为正数")
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("选页结果为空")
	}
	var sheets []Sheet
	for c := 0; c < copies; c++ {
		if !duplex {
			for _, p := range pages {
				sheets = append(sheets, Sheet{Front: p})
			}
			continue
		}
		for i := 0; i < len(pages); i += 2 {
			s := Sheet{Front: pages[i]}
			if i+1 < len(pages) {
				s.Back = pages[i+1]
			}
			sheets = append(sheets, s)
		}
	}
	return sheets, nil
}
