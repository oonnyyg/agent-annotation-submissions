package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	tripsPath := flag.String("trips", "", "班次 CSV 文件路径（表头 id,from,to,depart,arrive）")
	netPath := flag.String("network", "", "空驶网络 JSON 文件路径")
	flag.Parse()
	if *tripsPath == "" || *netPath == "" {
		fmt.Fprintln(os.Stderr, "用法: vehicle-duty-chain -trips trips.csv -network network.json")
		os.Exit(2)
	}
	if err := run(*tripsPath, *netPath, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(tripsPath, netPath string, out io.Writer) error {
	tf, err := os.Open(tripsPath)
	if err != nil {
		return err
	}
	defer tf.Close()
	trips, err := parseTripsCSV(tf)
	if err != nil {
		return err
	}
	nf, err := os.Open(netPath)
	if err != nil {
		return err
	}
	defer nf.Close()
	net, err := parseNetworkJSON(nf)
	if err != nil {
		return err
	}
	p := buildProblem(trips, net)
	sol := solve(p)
	printSolution(out, p, sol)
	return nil
}

// printSolution 输出逐车有序班次链、每次接续的空驶/等待与总计。
func printSolution(out io.Writer, p *Problem, sol *Solution) {
	for ci, chain := range sol.Chains {
		fmt.Fprintf(out, "车辆 %d（%d 班）:\n", ci+1, len(chain))
		for pos, idx := range chain {
			tr := p.Trips[idx]
			fmt.Fprintf(out, "  %s  %s->%s  %s - %s\n",
				tr.ID, tr.From, tr.To,
				tr.Depart.Format("15:04"), tr.Arrive.Format("15:04"))
			if pos+1 < len(chain) {
				c := sol.Conns[[2]int{idx, chain[pos+1]}]
				fmt.Fprintf(out, "    -> 接续: 空驶 %s / %d km, 等待 %s\n",
					formatDuration(c.Travel), c.Dist, formatDuration(c.Wait))
			}
		}
	}
	fmt.Fprintf(out, "总用车数: %d\n", sol.Vehicles)
	fmt.Fprintf(out, "总空驶里程: %d km\n", sol.TotalDist)
}
