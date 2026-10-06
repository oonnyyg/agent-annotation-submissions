// stocktake 是盘点工作台 CLI：按 JSONL 事件日志管理库存、出入库与盘点。
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"example.com/stocktake-adjustment-desk/internal/desk"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	cmd, rest := args[0], args[1:]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	dir := fs.String("dir", envOr("STOCKTAKE_DIR", "data"), "数据目录")
	var takeID, loc, sku, recID string
	var qty int
	var locs multiFlag
	fs.StringVar(&takeID, "take", "", "盘点编号")
	fs.StringVar(&loc, "loc", "", "库位")
	fs.StringVar(&sku, "sku", "", "商品")
	fs.StringVar(&recID, "rec", "", "清点记录编号")
	fs.IntVar(&qty, "qty", 0, "数量（非负整数）")
	fs.Var(&locs, "locs", "盘点库位（可重复或逗号分隔）")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	st, err := desk.Open(*dir)
	if err != nil {
		return err
	}
	printJSON := func(v any) error {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	switch cmd {
	case "inbound", "outbound":
		if loc == "" || sku == "" {
			return errors.New("需要 -loc 与 -sku")
		}
		res, err := st.Append(cmd, desk.MoveEvent{Loc: loc, SKU: sku, Qty: qty})
		if err != nil {
			return err
		}
		fmt.Printf("%s/%s 当前库存: %v\n", loc, sku, res)
		return nil
	case "query":
		return printJSON(st.State().SortedStock(loc))
	case "take-start":
		if takeID == "" || len(locs) == 0 {
			return errors.New("需要 -take 与至少一个 -locs")
		}
		_, err := st.Append("take_start", desk.TakeStartEvent{TakeID: takeID, Locations: locs})
		if err != nil {
			return err
		}
		fmt.Printf("盘点 %s 已开始，基线已冻结\n", takeID)
		return nil
	case "count-record":
		_, err := st.Append("count_record", desk.CountRecordEvent{TakeID: takeID, RecID: recID, Loc: loc, SKU: sku, Qty: qty})
		return err
	case "count-void":
		_, err := st.Append("count_void", desk.CountVoidEvent{TakeID: takeID, Loc: loc, RecID: recID})
		return err
	case "count-complete":
		_, err := st.Append("count_complete", desk.CountCompleteEvent{TakeID: takeID, Loc: loc})
		return err
	case "take-seal":
		res, err := st.Append("take_seal", desk.TakeRefEvent{TakeID: takeID})
		if err != nil {
			return err
		}
		return printJSON(res)
	case "take-post":
		res, err := st.Append("take_post", desk.TakeRefEvent{TakeID: takeID})
		if err != nil {
			return err
		}
		return printJSON(res)
	case "take-reopen":
		_, err := st.Append("take_reopen", desk.TakeRefEvent{TakeID: takeID})
		return err
	case "take-show":
		t, ok := st.State().Takes[takeID]
		if !ok {
			return fmt.Errorf("盘点 %s 不存在", takeID)
		}
		return printJSON(t)
	default:
		return fmt.Errorf("未知命令 %q\n%s", cmd, usage)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type multiFlag []string

func (m *multiFlag) String() string { return fmt.Sprint([]string(*m)) }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

const usage = `用法: go run ./cmd/stocktake <命令> [-dir 数据目录] [参数]
命令:
  inbound  -loc L -sku S -qty N        入库
  outbound -loc L -sku S -qty N        出库（不允许负库存）
  query    [-loc L]                    查询当前库存
  take-start   -take T -locs L [-locs L2...]  开始盘点并冻结基线
  count-record -take T -loc L -rec R -sku S -qty N  导入清点记录（幂等）
  count-void   -take T -loc L -rec R   撤销未封存清点记录
  count-complete -take T -loc L        确认库位清点完成
  take-seal    -take T                 封存并冻结差异明细
  take-post    -take T                 差异入账（可重试，幂等）
  take-reopen  -take T                 重开未入账盘点
  take-show    -take T                 查看盘点状态与明细`
