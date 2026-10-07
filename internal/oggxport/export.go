// Package oggxport exports audited Ogg packets and a JSON index into an
// output directory, published atomically only after the whole input
// validates.
package oggxport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"example.com/ogg-packet-audit/internal/ogg"
)

// PacketEntry describes one exported packet in the JSON index.
type PacketEntry struct {
	Stream     string        `json:"stream"`      // stream directory name
	Serial     uint32        `json:"serial"`      // logical stream serial number
	Index      uint64        `json:"index"`       // packet index within the stream
	File       string        `json:"file"`        // packet file path, relative to the output dir
	Length     int           `json:"length"`      // packet length in bytes
	SHA256     string        `json:"sha256"`      // hex digest of the raw packet bytes
	GranulePos uint64        `json:"granule_pos"` // raw granule position, not a timestamp
	Pages      []ogg.PageRef `json:"pages"`       // contributing pages by file offset
	EndOffset  int64         `json:"end_offset"`
}

// Index is the top-level JSON document written to index.json.
type Index struct {
	Source  string        `json:"source"`
	Packets []PacketEntry `json:"packets"`
	Streams []uint32      `json:"streams"`
	PageCnt int64         `json:"page_count"`
}

// Report summarizes a successful export.
type Report struct {
	Streams int
	Packets int
	Pages   int64
}

// Export audits r and writes packet files plus index.json into outDir.
// Output is staged in a temporary sibling directory and only moved into
// place after the entire input validates; on failure outDir is untouched.
func Export(r io.Reader, sourceName, outDir string, opts ogg.AuditOptions) (Report, error) {
	var rep Report
	tmp := fmt.Sprintf("%s.tmp-%d", outDir, os.Getpid())
	if err := os.RemoveAll(tmp); err != nil {
		return rep, err
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return rep, err
	}
	defer os.RemoveAll(tmp) // no-op after a successful publish

	idx := &Index{Source: sourceName}
	seenStreams := map[uint32]bool{}

	opts.OnPage = func(*ogg.Page) { rep.Pages++ }
	err := ogg.Audit(ogg.NewReader(r), opts, func(pkt *ogg.Packet) error {
		streamDir := fmt.Sprintf("stream-%08x", pkt.Serial)
		if !seenStreams[pkt.Serial] {
			seenStreams[pkt.Serial] = true
			idx.Streams = append(idx.Streams, pkt.Serial)
			if err := os.MkdirAll(filepath.Join(tmp, streamDir), 0o755); err != nil {
				return err
			}
		}
		name := fmt.Sprintf("packet-%06d.bin", pkt.Index)
		rel := filepath.Join(streamDir, name)
		if err := os.WriteFile(filepath.Join(tmp, rel), pkt.Data, 0o644); err != nil {
			return err
		}
		sum := sha256.Sum256(pkt.Data)
		idx.Packets = append(idx.Packets, PacketEntry{
			Stream:     streamDir,
			Serial:     pkt.Serial,
			Index:      pkt.Index,
			File:       filepath.ToSlash(rel),
			Length:     len(pkt.Data),
			SHA256:     hex.EncodeToString(sum[:]),
			GranulePos: pkt.GranulePos,
			Pages:      pkt.Pages,
			EndOffset:  pkt.EndOffset,
		})
		rep.Packets++
		return nil
	})
	if err != nil {
		return rep, err // staged output discarded; outDir untouched
	}
	idx.PageCnt = rep.Pages

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return rep, err
	}
	if err := os.WriteFile(filepath.Join(tmp, "index.json"), append(data, '\n'), 0o644); err != nil {
		return rep, err
	}

	// Publish: swap the staged directory into place.
	backup := fmt.Sprintf("%s.old-%d", outDir, os.Getpid())
	if err := os.RemoveAll(backup); err != nil {
		return rep, err
	}
	if _, err := os.Stat(outDir); err == nil {
		if err := os.Rename(outDir, backup); err != nil {
			return rep, fmt.Errorf("backing up existing output: %w", err)
		}
	}
	if err := os.Rename(tmp, outDir); err != nil {
		return rep, fmt.Errorf("publishing output: %w", err)
	}
	os.RemoveAll(backup)
	rep.Streams = len(idx.Streams)
	return rep, nil
}
