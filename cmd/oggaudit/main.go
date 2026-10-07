// Command oggaudit validates an Ogg container file (RFC 3533, version 0)
// and exports its logical packets plus a JSON index for archival review.
// It never decodes the audio or video payload.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"example.com/ogg-packet-audit/ogg"
)

type pageRefJSON struct {
	Offset   int64  `json:"offset"`
	Length   int64  `json:"length"`
	Sequence uint32 `json:"sequence"`
}

type packetJSON struct {
	Index   int           `json:"index"`
	File    string        `json:"file"`
	Length  int           `json:"length"`
	SHA256  string        `json:"sha256"`
	Granule uint64        `json:"granule"`
	Pages   []pageRefJSON `json:"pages"`
}

type streamJSON struct {
	Serial  uint32       `json:"serial"`
	Group   int          `json:"group"`
	Packets []packetJSON `json:"packets"`
}

type indexJSON struct {
	Source  string       `json:"source"`
	Size    int64        `json:"size"`
	Streams []streamJSON `json:"streams"`
}

func main() {
	outDir := flag.String("out", "", "output directory (default <file>.audit)")
	maxPkt := flag.Int64("max-packet-size", 64<<20, "maximum packet size in bytes; larger packets fail the audit")
	checkOnly := flag.Bool("check-only", false, "validate only, do not export packets")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: oggaudit [-out dir] [-max-packet-size n] [-check-only] <file.ogg>")
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *outDir, *maxPkt, *checkOnly); err != nil {
		fmt.Fprintln(os.Stderr, "oggaudit:", err)
		os.Exit(1)
	}
}

func run(src, outDir string, maxPkt int64, checkOnly bool) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if outDir == "" {
		outDir = src + ".audit"
	}

	// Stage results in a temporary directory; they are published only
	// after the entire file validates, so a failed run never clobbers
	// previously exported output.
	tmp, err := os.MkdirTemp(filepath.Dir(outDir), ".oggaudit-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	idx := indexJSON{Source: src, Size: st.Size()}
	streamDirs := map[int]string{}
	streamSlot := map[*ogg.StreamResult]int{}

	cb := func(s *ogg.StreamResult, p *ogg.Packet) error {
		key, ok := streamSlot[s]
		if !ok {
			key = len(idx.Streams)
			streamSlot[s] = key
			idx.Streams = append(idx.Streams, streamJSON{Serial: s.Serial, Group: s.Group})
		}
		sum := sha256.Sum256(p.Data)
		entry := packetJSON{
			Index:   p.Index,
			Length:  len(p.Data),
			SHA256:  hex.EncodeToString(sum[:]),
			Granule: p.Granule,
		}
		for _, ref := range p.Pages {
			entry.Pages = append(entry.Pages, pageRefJSON{Offset: ref.Offset, Length: ref.Length, Sequence: ref.Sequence})
		}
		if !checkOnly {
			dir, ok := streamDirs[key]
			if !ok {
				dir = fmt.Sprintf("stream-%08x-g%d", s.Serial, s.Group)
				streamDirs[key] = dir
				if err := os.MkdirAll(filepath.Join(tmp, dir), 0o755); err != nil {
					return err
				}
			}
			name := filepath.Join(dir, fmt.Sprintf("packet-%06d.bin", p.Index))
			if err := os.WriteFile(filepath.Join(tmp, name), p.Data, 0o644); err != nil {
				return err
			}
			entry.File = filepath.ToSlash(name)
		}
		idx.Streams[key].Packets = append(idx.Streams[key].Packets, entry)
		return nil
	}

	if _, err := ogg.Audit(f, maxPkt, cb); err != nil {
		return err
	}

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, "index.json"), data, 0o644); err != nil {
		return err
	}

	if checkOnly {
		fmt.Printf("ok: %s validated (%d streams)\n", src, len(idx.Streams))
		return nil
	}
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.Rename(tmp, outDir); err != nil {
		return err
	}
	fmt.Printf("ok: exported %d streams to %s\n", len(idx.Streams), outDir)
	return nil
}
