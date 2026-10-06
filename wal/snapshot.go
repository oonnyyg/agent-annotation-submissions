package wal

import (
	"encoding/binary"
	"fmt"
)

// PageSource records where each page of the snapshot came from.
type PageSource struct {
	Page       uint32 `json:"page"`
	Source     string `json:"source"` // "wal" or "database"
	FrameIndex int    `json:"frameIndex,omitempty"`
}

// Snapshot is the result of rebuilding a database image.
type Snapshot struct {
	Data     []byte
	PageSize uint32
	Pages    []PageSource
	NoCommit bool // true when the WAL had no valid commit; baseline kept
	Note     string
}

// DBPageSize reads the page size from a SQLite database header.
func DBPageSize(db []byte) (uint32, error) {
	if len(db) < 100 {
		return 0, fmt.Errorf("database file shorter than 100-byte header")
	}
	if string(db[0:16]) != "SQLite format 3\x00" {
		return 0, fmt.Errorf("database file lacks SQLite format 3 header")
	}
	ps := uint32(binary.BigEndian.Uint16(db[16:18]))
	if ps == 1 {
		ps = 65536
	}
	if ps < 512 || ps > 65536 || ps&(ps-1) != 0 {
		return 0, fmt.Errorf("invalid database page size %d", ps)
	}
	return ps, nil
}

// BuildSnapshot reconstructs the database image at the last valid commit.
// dbData is the baseline main database (pre-WAL, no frames applied).
func BuildSnapshot(dbData []byte, res *VerifyResult) (*Snapshot, error) {
	h := res.Header
	dbPS, err := DBPageSize(dbData)
	if err != nil {
		return nil, err
	}
	if dbPS != h.PageSize {
		return nil, fmt.Errorf("page size mismatch: database %d, WAL %d", dbPS, h.PageSize)
	}
	ps := int64(h.PageSize)

	if res.LastCommit < 0 {
		out := make([]byte, len(dbData))
		copy(out, dbData)
		return &Snapshot{
			Data: out, PageSize: h.PageSize, NoCommit: true,
			Note: "WAL contains no valid commit; snapshot is the unchanged baseline database",
		}, nil
	}

	commitFrame := res.Frames[res.LastCommit]
	dbSize := commitFrame.DBSize
	// Latest frame per page within the committed prefix only.
	chosen := map[uint32]*Frame{}
	for _, f := range res.Frames[:res.LastCommit+1] {
		chosen[f.PageNumber] = f
	}

	out := make([]byte, int64(dbSize)*ps)
	pages := make([]PageSource, 0, dbSize)
	for pg := uint32(1); pg <= dbSize; pg++ {
		dst := out[int64(pg-1)*ps : int64(pg)*ps]
		if f, ok := chosen[pg]; ok {
			copy(dst, f.Data)
			pages = append(pages, PageSource{Page: pg, Source: "wal", FrameIndex: f.Index})
			continue
		}
		start := int64(pg-1) * ps
		if start+ps > int64(len(dbData)) {
			return nil, fmt.Errorf("page %d missing: not in committed WAL frames and beyond baseline database (%d bytes)", pg, len(dbData))
		}
		copy(dst, dbData[start:start+ps])
		pages = append(pages, PageSource{Page: pg, Source: "database"})
	}
	return &Snapshot{
		Data: out, PageSize: h.PageSize, Pages: pages,
		Note: fmt.Sprintf("rebuilt at commit frame %d (database size %d pages)", commitFrame.Index, dbSize),
	}, nil
}
