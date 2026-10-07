package main

import (
	"fmt"
	"math/big"
	"strings"
)

// record 为一条输入记录，seq 为全局原始次序（按文件顺序、文件内记录顺序递增）。
type record struct {
	fields []string
	seq    int64
}

// comparator 按规则比较两条记录；键全等时按 seq 保持稳定。
type comparator struct {
	keys []KeyRule
	cols []int
}

// compare 返回负数、0 或正数。键相等时比较 seq，保证跨文件、跨分块稳定。
func (c *comparator) compare(a, b *record) int {
	for i, k := range c.keys {
		av := a.fields[c.cols[i]]
		bv := b.fields[c.cols[i]]
		if r := compareEmpty(av, bv, k.Empty); r != 0 {
			return r
		}
		if av == "" && bv == "" {
			continue
		}
		var r int
		if k.Type == "decimal" {
			r = compareDecimal(av, bv)
		} else {
			r = strings.Compare(av, bv)
		}
		if r != 0 {
			if k.Order == "desc" {
				return -r
			}
			return r
		}
	}
	if a.seq < b.seq {
		return -1
	}
	if a.seq > b.seq {
		return 1
	}
	return 0
}

// keyEqual 仅比较排序键（含空值位置规则），用于去重判定。
func (c *comparator) keyEqual(a, b *record) bool {
	for i := range c.keys {
		av := a.fields[c.cols[i]]
		bv := b.fields[c.cols[i]]
		if (av == "") != (bv == "") {
			return false
		}
		if av == "" {
			continue
		}
		if c.keys[i].Type == "decimal" {
			if compareDecimal(av, bv) != 0 {
				return false
			}
		} else if av != bv {
			return false
		}
	}
	return true
}

// compareEmpty 处理空字段的独立位置规则；两者均空或均非空时返回 0。
func compareEmpty(a, b, empty string) int {
	ae, be := a == "", b == ""
	if ae == be {
		return 0
	}
	if ae {
		if empty == "first" {
			return -1
		}
		return 1
	}
	if empty == "first" {
		return 1
	}
	return -1
}

// compareDecimal 按有限十进制数值比较，不依赖浮点精度。
func compareDecimal(a, b string) int {
	ra, _ := new(big.Rat).SetString(a)
	rb, _ := new(big.Rat).SetString(b)
	return ra.Cmp(rb)
}

// validateDecimal 在读取阶段校验十进制字段，类型错误定位到文件与逻辑记录号。
func validateDecimal(value, file string, recNo int64) error {
	if value == "" {
		return nil
	}
	if _, ok := new(big.Rat).SetString(value); !ok {
		return fmt.Errorf("类型错误: 文件 %s 第 %d 条记录: 字段 %q 不是合法的十进制数值", file, recNo, value)
	}
	return nil
}
