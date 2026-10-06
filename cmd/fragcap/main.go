// Command fragcap reads a classic pcap file, reassembles IPv4 fragments
// into UDP datagrams, and reports complete traffic separately from
// content that cannot be confirmed complete.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"example.com/pcap-fragment-audit/internal/parse"
	"example.com/pcap-fragment-audit/internal/pcap"
	"example.com/pcap-fragment-audit/internal/reasm"
)

type datagramLine struct {
	SrcIP        string `json:"src_ip"`
	DstIP        string `json:"dst_ip"`
	SrcPort      uint16 `json:"src_port"`
	DstPort      uint16 `json:"dst_port"`
	Ident        uint16 `json:"ident"`
	UDPLength    int    `json:"udp_length"`
	PayloadBytes int    `json:"payload_bytes"`
	Records      []int  `json:"records"`
}

type anomaly struct {
	Kind    string      `json:"kind"`
	Records []int       `json:"records,omitempty"`
	Record  int         `json:"record,omitempty"`
	Group   string      `json:"group,omitempty"`
	Detail  string      `json:"detail"`
	Gaps    []reasm.Gap `json:"gaps,omitempty"`
}

type flowStat struct {
	SrcIP        string `json:"src_ip"`
	DstIP        string `json:"dst_ip"`
	SrcPort      uint16 `json:"src_port"`
	DstPort      uint16 `json:"dst_port"`
	Datagrams    int    `json:"datagrams"`
	PayloadBytes int    `json:"payload_bytes"`
}

type summary struct {
	RecordsRead   int        `json:"records_read"`
	NonUDPRecords int        `json:"non_udp_records"`
	TruncCaptures int        `json:"truncated_captures"`
	Complete      int        `json:"complete_datagrams"`
	Flows         []flowStat `json:"flows"`
	Anomalies     []anomaly  `json:"anomalies"`
}

func ipStr(b [4]byte) string {
	return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
}

func main() {
	outPath := flag.String("out", "", "datagram JSONL output file (default stdout)")
	sumPath := flag.String("summary", "", "anomaly summary JSON file (default stderr)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: fragcap [-out file] [-summary file] capture.pcap")
		os.Exit(2)
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	rd, err := pcap.NewReader(f)
	if err != nil {
		fatal(err)
	}
	datagrams, sum := analyze(rd)

	out := io.Writer(os.Stdout)
	if *outPath != "" {
		w, err := os.Create(*outPath)
		if err != nil {
			fatal(err)
		}
		defer w.Close()
		out = w
	}
	enc := json.NewEncoder(out)
	for _, d := range datagrams {
		if err := enc.Encode(d); err != nil {
			fatal(err)
		}
	}

	sumOut := io.Writer(os.Stderr)
	if *sumPath != "" {
		w, err := os.Create(*sumPath)
		if err != nil {
			fatal(err)
		}
		defer w.Close()
		sumOut = w
	}
	senc := json.NewEncoder(sumOut)
	senc.SetIndent("", "  ")
	if err := senc.Encode(sum); err != nil {
		fatal(err)
	}
}

// analyze consumes all records and produces complete datagrams plus a
// summary of traffic and anomalies.
func analyze(rd *pcap.Reader) ([]datagramLine, *summary) {
	r := reasm.New()
	sum := &summary{}
	var anomalies []anomaly

	for {
		rec, err := rd.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			anomalies = append(anomalies, anomaly{
				Kind: "read_error", Detail: err.Error(),
			})
			break
		}
		sum.RecordsRead++
		if rec.Truncated {
			sum.TruncCaptures++
		}
		pkt, err := parse.Decode(rec.Data)
		if errors.Is(err, parse.ErrNotUDP) {
			sum.NonUDPRecords++
			continue
		}
		if err != nil {
			anomalies = append(anomalies, anomaly{
				Kind:   "malformed_packet",
				Record: rec.Index,
				Detail: err.Error(),
			})
			continue
		}
		r.Add(pkt, rec.Index)
	}

	type flowKey struct {
		src, dst     string
		sport, dport uint16
	}
	flows := map[flowKey]*flowStat{}
	var datagrams []datagramLine

	for _, res := range r.Finalize() {
		key := res.Key.String()
		switch {
		case res.Conflict:
			anomalies = append(anomalies, anomaly{
				Kind: "conflict", Group: key, Records: res.Records,
				Detail: "overlapping fragments disagree or last-fragment length conflicts",
			})
		case res.Truncated:
			anomalies = append(anomalies, anomaly{
				Kind: "truncated_group", Group: key, Records: res.Records,
				Detail: "a fragment was cut short by the capture snap length",
			})
		case !res.Complete:
			kind := "missing_fragments"
			detail := "byte ranges never captured"
			if !res.LastSeen {
				kind = "last_fragment_unknown"
				detail = "no MF=0 fragment seen; total datagram size unknown"
			}
			anomalies = append(anomalies, anomaly{
				Kind: kind, Group: key, Records: res.Records,
				Detail: detail, Gaps: res.Gaps,
			})
		default:
			udp := res.UDP
			if len(udp) < 8 {
				anomalies = append(anomalies, anomaly{
					Kind: "invalid_udp", Group: key, Records: res.Records,
					Detail: "reassembled datagram shorter than UDP header",
				})
				continue
			}
			udpLen := int(udp[4])<<8 | int(udp[5])
			if udpLen != len(udp) {
				anomalies = append(anomalies, anomaly{
					Kind: "invalid_udp", Group: key, Records: res.Records,
					Detail: fmt.Sprintf("UDP length field %d != reassembled length %d", udpLen, len(udp)),
				})
				continue
			}
			if !parse.UDPChecksum(res.Key.Src, res.Key.Dst, udp) {
				anomalies = append(anomalies, anomaly{
					Kind: "invalid_udp", Group: key, Records: res.Records,
					Detail: "UDP checksum verification failed",
				})
				continue
			}
			sport := uint16(udp[0])<<8 | uint16(udp[1])
			dport := uint16(udp[2])<<8 | uint16(udp[3])
			datagrams = append(datagrams, datagramLine{
				SrcIP: ipStr(res.Key.Src), DstIP: ipStr(res.Key.Dst),
				SrcPort: sport, DstPort: dport, Ident: res.Key.Ident,
				UDPLength: udpLen, PayloadBytes: udpLen - 8,
				Records: res.Records,
			})
			fk := flowKey{ipStr(res.Key.Src), ipStr(res.Key.Dst), sport, dport}
			fs, ok := flows[fk]
			if !ok {
				fs = &flowStat{SrcIP: fk.src, DstIP: fk.dst, SrcPort: sport, DstPort: dport}
				flows[fk] = fs
			}
			fs.Datagrams++
			fs.PayloadBytes += udpLen - 8
		}
	}

	sum.Complete = len(datagrams)
	sum.Anomalies = anomalies
	if sum.Anomalies == nil {
		sum.Anomalies = []anomaly{}
	}
	for _, fs := range flows {
		sum.Flows = append(sum.Flows, *fs)
	}
	sort.Slice(sum.Flows, func(i, j int) bool {
		a, b := sum.Flows[i], sum.Flows[j]
		if a.SrcIP != b.SrcIP {
			return a.SrcIP < b.SrcIP
		}
		if a.DstIP != b.DstIP {
			return a.DstIP < b.DstIP
		}
		if a.SrcPort != b.SrcPort {
			return a.SrcPort < b.SrcPort
		}
		return a.DstPort < b.DstPort
	})
	if sum.Flows == nil {
		sum.Flows = []flowStat{}
	}
	return datagrams, sum
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "fragcap:", err)
	os.Exit(1)
}
