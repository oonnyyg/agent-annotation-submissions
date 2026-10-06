package elfmap

import (
	"fmt"
	"io"
	"os"
)

// ReadQuery is one entry of the JSON config's "reads" array.
type ReadQuery struct {
	Name   string `json:"name"`
	Addr   uint64 `json:"addr"` // hex ("0x..") or decimal
	Length int    `json:"length"`
}

// Config is the JSON query configuration.
type Config struct {
	LoadBias uint64      `json:"load_bias"` // used only for ET_DYN
	Reads    []ReadQuery `json:"reads"`
}

// SegmentReport describes one mapped PT_LOAD segment.
type SegmentReport struct {
	Index    int    `json:"index"`
	Flags    string `json:"flags"`
	FileOff  uint64 `json:"file_off"`
	Vaddr    uint64 `json:"vaddr"`
	FileEnd  uint64 `json:"file_end"`
	MemEnd   uint64 `json:"mem_end"`
	Filesz   uint64 `json:"filesz"`
	Memsz    uint64 `json:"memsz"`
	ZeroFill uint64 `json:"zero_fill"`
	Align    uint64 `json:"align"`
}

// EntryReport is the entry-point audit outcome.
type EntryReport struct {
	Present bool   `json:"present"`
	Address uint64 `json:"address,omitempty"`
	Valid   bool   `json:"valid"`
	Segment int    `json:"segment"`
	Reason  string `json:"reason,omitempty"`
}

// ReadOutcome reports one configured read.
type ReadOutcome struct {
	Name   string      `json:"name"`
	Addr   uint64      `json:"addr"`
	Length int         `json:"length"`
	OK     bool        `json:"ok"`
	Error  string      `json:"error,omitempty"`
	Result *ReadResult `json:"result,omitempty"`
}

// Report is the full JSON output document.
type Report struct {
	Path     string          `json:"path"`
	FileType string          `json:"file_type"`
	LoadBias uint64          `json:"load_bias"`
	Segments []SegmentReport `json:"segments"`
	Pages    []PageRange     `json:"pages"`
	WXPages  []PageRange     `json:"wx_pages"`
	Entry    EntryReport     `json:"entry"`
	Reads    []ReadOutcome   `json:"reads"`
}

// Analyze opens the ELF file read-only, parses it, builds the map and runs all
// queries. The file is never modified or executed.
func Analyze(path string, cfg Config) (*Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return AnalyzeReader(f, st.Size(), path, cfg)
}

// AnalyzeReader is the test-friendly core of Analyze.
func AnalyzeReader(r io.ReaderAt, fileSize int64, path string, cfg Config) (*Report, error) {
	h, err := ParseHeader(r, fileSize)
	if err != nil {
		return nil, err
	}
	phdrs, err := ParseProgramHeaders(r, h)
	if err != nil {
		return nil, err
	}
	bias := uint64(0)
	if h.Type == etDyn {
		bias = cfg.LoadBias
	} else if cfg.LoadBias != 0 {
		return nil, fmt.Errorf("load_bias %#x is only valid for ET_DYN", cfg.LoadBias)
	}
	m, err := Build(h, phdrs, fileSize, bias)
	if err != nil {
		return nil, err
	}

	rep := &Report{
		Path:     path,
		LoadBias: bias,
		Segments: []SegmentReport{},
		Pages:    []PageRange{},
		WXPages:  []PageRange{},
		Reads:    []ReadOutcome{},
	}
	if h.Type == etExec {
		rep.FileType = "ET_EXEC"
	} else {
		rep.FileType = "ET_DYN"
	}
	for _, s := range m.Segments {
		fileEnd, _ := s.FileEnd()
		memEnd, _ := s.MemEnd()
		rep.Segments = append(rep.Segments, SegmentReport{
			Index:    s.Index,
			Flags:    flagString(s.Flags),
			FileOff:  s.FileOff,
			Vaddr:    s.Vaddr,
			FileEnd:  fileEnd,
			MemEnd:   memEnd,
			Filesz:   s.Filesz,
			Memsz:    s.Memsz,
			ZeroFill: s.Memsz - s.Filesz,
			Align:    s.Align,
		})
	}
	pages, err := m.Pages()
	if err != nil {
		return nil, err
	}
	rep.Pages = pages
	wx, err := m.WXPages()
	if err != nil {
		return nil, err
	}
	rep.WXPages = wx

	entry, err := m.AuditEntry()
	if err != nil {
		return nil, err
	}
	er := EntryReport{Segment: entry.Segment}
	switch {
	case entry.NoEntry:
		er.Reason = "e_entry is zero: no entry point"
	case entry.Valid:
		er.Present = true
		er.Address = entry.Address
		er.Valid = true
		er.Segment = entry.Segment
	default:
		er.Present = true
		er.Address = entry.Address
		er.Reason = "entry does not belong to a file-backed executable segment"
	}
	rep.Entry = er

	for _, q := range cfg.Reads {
		out := ReadOutcome{Name: q.Name, Addr: q.Addr, Length: q.Length}
		res, rerr := m.Read(r, q.Addr, q.Length)
		if rerr != nil {
			out.Error = rerr.Error()
		} else {
			out.OK = true
			out.Result = res
		}
		rep.Reads = append(rep.Reads, out)
	}
	return rep, nil
}

func flagString(f uint32) string {
	b := [3]byte{'-', '-', '-'}
	if f&pfR != 0 {
		b[0] = 'r'
	}
	if f&pfW != 0 {
		b[1] = 'w'
	}
	if f&pfX != 0 {
		b[2] = 'x'
	}
	return string(b[:])
}
