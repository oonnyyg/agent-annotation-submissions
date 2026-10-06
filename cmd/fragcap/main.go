package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
)

type datagramOut struct {
	Record  int    `json:"record"` // last record index contributing to this datagram
	Src     string `json:"src"`
	Dst     string `json:"dst"`
	SrcPort uint16 `json:"src_port"`
	DstPort uint16 `json:"dst_port"`
	ID      uint16 `json:"ip_id"`
	Length  int    `json:"udp_payload_length"`
	Payload []byte `json:"udp_payload"` // JSON: base64
}

type anomalyOut struct {
	Kind   string `json:"kind"` // "invalid_packet", "conflict", "incomplete", "last_fragment_missing"
	Record int    `json:"record,omitempty"`
	Key    string `json:"key,omitempty"`
	Detail string `json:"detail"`
}

type flowStat struct {
	Src, Dst         string
	SrcPort, DstPort uint16
	Datagrams        int
	PayloadBytes     int
}

type flowOut struct {
	Flow         string `json:"flow"`
	Datagrams    int    `json:"datagrams"`
	PayloadBytes int    `json:"udp_payload_bytes"`
}

type summaryOut struct {
	Records          int         `json:"records"`
	TruncatedRecords int         `json:"truncated_records"`
	NonIPv4          int         `json:"non_ipv4_frames"`
	NonUDP           int         `json:"non_udp_packets"`
	InvalidPackets   int         `json:"invalid_packets"`
	Complete         int         `json:"complete_datagrams"`
	Anomalies        []anomalyOut `json:"anomalies"`
	Flows            []flowOut   `json:"flows"`
}

func main() {
	datagramsPath := flag.String("datagrams", "", "write datagram JSONL here (default stdout)")
	summaryPath := flag.String("summary", "", "write anomaly summary JSON here (default stderr)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: fragcap [-datagrams file] [-summary file] <input.pcap>")
		os.Exit(2)
	}
	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fatal(err)
	}
	defer f.Close()

	var dgW, smW io.Writer = os.Stdout, os.Stderr
	if *datagramsPath != "" {
		df, err := os.Create(*datagramsPath)
		if err != nil {
			fatal(err)
		}
		defer df.Close()
		dgW = df
	}
	if *summaryPath != "" {
		sf, err := os.Create(*summaryPath)
		if err != nil {
			fatal(err)
		}
		defer sf.Close()
		smW = sf
	}
	if err := run(f, dgW, smW); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "fragcap:", err)
	os.Exit(1)
}

func run(r io.Reader, dgW, smW io.Writer) error {
	p, err := newPcapReader(r)
	if err != nil {
		return err
	}
	if p.linkType != 1 {
		return fmt.Errorf("unsupported link type %d (only Ethernet / LINKTYPE_ETHERNET=1)", p.linkType)
	}
	sum := summaryOut{}
	groups := map[fragKey]*fragGroup{}
	var order []fragKey
	// last record index contributing to each group
	lastRec := map[fragKey]int{}

	for {
		rec, err := p.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("record %d: %w", sum.Records+1, err)
		}
		sum.Records++
		rec.Index = sum.Records
		if rec.Truncated {
			sum.TruncatedRecords++
			sum.Anomalies = append(sum.Anomalies, anomalyOut{
				Kind:   "truncated_record",
				Record: rec.Index,
				Detail: fmt.Sprintf("captured %d of %d on-wire bytes", len(rec.Data), rec.OrigLen),
			})
		}
		pkt, err := parseEthernetIPv4(rec.Data)
		if err != nil {
			sum.InvalidPackets++
			sum.Anomalies = append(sum.Anomalies, anomalyOut{
				Kind: "invalid_packet", Record: rec.Index, Detail: err.Error(),
			})
			continue
		}
		if pkt == nil {
			sum.NonIPv4++
			continue
		}
		if pkt.Protocol != protoUDP {
			sum.NonUDP++
			continue
		}
		key := fragKey{Src: pkt.Src, Dst: pkt.Dst, Proto: pkt.Protocol, ID: pkt.ID}
		g := groups[key]
		if g == nil {
			g = newFragGroup(key)
			groups[key] = g
			order = append(order, key)
		}
		g.add(pkt, rec.Index)
		lastRec[key] = rec.Index
	}

	enc := json.NewEncoder(dgW)
	flows := map[[4]string]*flowStat{}
	for _, key := range order {
		g := groups[key]
		payload, gaps, err := g.finalize()
		keyStr := fmt.Sprintf("%s>%s proto=%d id=%d", key.Src, key.Dst, key.Proto, key.ID)
		if err != nil {
			kind := "conflict"
			detail := err.Error()
			switch err {
			case errLastFragmentMissing:
				kind = "last_fragment_missing"
			case errIncomplete:
				kind = "incomplete"
				detail = fmt.Sprintf("total %d bytes, gaps: %v", g.total, gaps)
			}
			sum.Anomalies = append(sum.Anomalies, anomalyOut{Kind: kind, Key: keyStr, Detail: detail})
			continue
		}
		dg, err := parseUDP(payload, key.Src, key.Dst)
		if err != nil {
			sum.Anomalies = append(sum.Anomalies, anomalyOut{Kind: "invalid_udp", Key: keyStr, Detail: err.Error()})
			continue
		}
		sum.Complete++
		enc.Encode(datagramOut{
			Record:  lastRec[key],
			Src:     key.Src.String(),
			Dst:     key.Dst.String(),
			SrcPort: dg.SrcPort,
			DstPort: dg.DstPort,
			ID:      key.ID,
			Length:  len(dg.Payload),
			Payload: dg.Payload,
		})
		fk := [4]string{key.Src.String(), key.Dst.String(), fmt.Sprint(dg.SrcPort), fmt.Sprint(dg.DstPort)}
		fs := flows[fk]
		if fs == nil {
			fs = &flowStat{Src: key.Src.String(), Dst: key.Dst.String(), SrcPort: dg.SrcPort, DstPort: dg.DstPort}
			flows[fk] = fs
		}
		fs.Datagrams++
		fs.PayloadBytes += len(dg.Payload)
	}
	for _, fs := range flows {
		sum.Flows = append(sum.Flows, flowOut{
			Flow:         fmt.Sprintf("%s:%d > %s:%d", fs.Src, fs.SrcPort, fs.Dst, fs.DstPort),
			Datagrams:    fs.Datagrams,
			PayloadBytes: fs.PayloadBytes,
		})
	}
	sort.Slice(sum.Flows, func(i, j int) bool { return sum.Flows[i].Flow < sum.Flows[j].Flow })
	if sum.Anomalies == nil {
		sum.Anomalies = []anomalyOut{}
	}
	if sum.Flows == nil {
		sum.Flows = []flowOut{}
	}
	se := json.NewEncoder(smW)
	se.SetIndent("", "  ")
	return se.Encode(sum)
}
