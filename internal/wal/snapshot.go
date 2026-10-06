package wal

import "fmt"

// PageSource describes where one page of the snapshot came from.
type PageSource struct {
	Page   uint32 `json:"page"`
	Source string `json:"source"` // "wal" or "main"
	Frame  int    `json:"frame"`  // -1 when Source is "main"
}

// Snapshot is the result of rebuilding the last committed state.
type Snapshot struct {
	Image       []byte
	Pages       []PageSource
	CommitIndex int // index of the chosen commit frame, -1 if none
	DBSize      uint32
	NoCommit    bool // true when the WAL has no commit in the valid prefix
}

// BuildSnapshot reconstructs the database image at the last commit of the
// valid WAL prefix. Baseline is the main database file as it was before any
// of these WAL frames were checkpointed into it.
//
// Pages come from the last frame (within the committed range) that wrote
// them, falling back to the baseline. Missing baseline pages are an error;
// they are never zero-filled.
func BuildSnapshot(res *ScanResult, baseline []byte) (*Snapshot, error) {
	ps := int64(res.Header.PageSize)
	last := res.LastCommit()
	snap := &Snapshot{CommitIndex: last}

	if last < 0 {
		// No commit in the valid prefix: keep the baseline verbatim.
		snap.NoCommit = true
		snap.DBSize = uint32((int64(len(baseline)) + ps - 1) / ps)
		snap.Image = make([]byte, len(baseline))
		copy(snap.Image, baseline)
		for p := uint32(1); p <= snap.DBSize; p++ {
			snap.Pages = append(snap.Pages, PageSource{Page: p, Source: "main", Frame: -1})
		}
		return snap, nil
	}

	commit := res.Frames[last]
	dbSize := commit.DBSize
	if dbSize == 0 {
		return nil, fmt.Errorf("内部错误: 提交帧 %d 的数据库页数为 0", last)
	}
	snap.DBSize = dbSize

	// Last write wins, considering only frames up to and including the
	// chosen commit. Frames after the commit never enter the snapshot.
	walPage := make(map[uint32]int)
	for i := 0; i <= last; i++ {
		walPage[res.Frames[i].PageNumber] = i
	}

	img := make([]byte, int64(dbSize)*ps)
	for p := uint32(1); p <= dbSize; p++ {
		dst := img[int64(p-1)*ps : int64(p)*ps]
		if fi, ok := walPage[p]; ok {
			copy(dst, res.Frames[fi].Data)
			snap.Pages = append(snap.Pages, PageSource{Page: p, Source: "wal", Frame: fi})
			continue
		}
		start := int64(p-1) * ps
		if start+ps > int64(len(baseline)) {
			return nil, fmt.Errorf("第 %d 页既不在已提交 WAL 帧中，也超出主库基线范围（需要偏移 %d..%d，主库仅 %d 字节）；拒绝补零",
				p, start, start+ps, len(baseline))
		}
		copy(dst, baseline[start:start+ps])
		snap.Pages = append(snap.Pages, PageSource{Page: p, Source: "main", Frame: -1})
	}
	snap.Image = img
	return snap, nil
}
