package pack

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Limits bound resource use during recovery.
type Limits struct {
	MaxObjectSize int64
	MaxDeltaDepth int
}

// DefaultLimits are conservative defaults suitable for typical packs.
var DefaultLimits = Limits{MaxObjectSize: 1 << 30, MaxDeltaDepth: 64}

// Record is one row of the recovery manifest.
type Record struct {
	Offset   int64
	Type     ObjectType
	EntryTyp ObjectType
	Size     int
	ID       [20]byte
	BaseDesc string
}

// Result holds the recovered objects and the manifest.
type Result struct {
	Records []Record
	Objects map[[20]byte][]byte
}

// ObjectID computes the Git object identity for a type and body.
func ObjectID(typ ObjectType, body []byte) [20]byte {
	h := sha1.New()
	fmt.Fprintf(h, "%s %d\x00", typ, len(body))
	h.Write(body)
	var id [20]byte
	copy(id[:], h.Sum(nil))
	return id
}

var errDeferred = fmt.Errorf("deferred")

// Resolve recovers every object in the pack, resolving delta chains.
// Ref-delta bases may appear later in the pack; entries whose base identity
// is not yet known are deferred and retried until no progress is made.
func Resolve(p *Pack, lim Limits) (*Result, error) {
	if lim.MaxObjectSize <= 0 {
		lim.MaxObjectSize = DefaultLimits.MaxObjectSize
	}
	if lim.MaxDeltaDepth <= 0 {
		lim.MaxDeltaDepth = DefaultLimits.MaxDeltaDepth
	}
	n := len(p.Entries)
	bodies := make([][]byte, n)
	types := make([]ObjectType, n)
	ids := make([][20]byte, n)
	done := make([]bool, n)

	resolveOne := func(i int) error {
		e := &p.Entries[i]
		switch e.Type {
		case ObjCommit, ObjTree, ObjBlob, ObjTag:
			bodies[i], types[i] = e.Data, e.Type
		case ObjOfsDelta, ObjRefDelta:
			var baseIdx int
			var ok bool
			if e.Type == ObjOfsDelta {
				baseIdx, ok = p.IndexByOffset(e.BaseOff)
				if !ok {
					return fmt.Errorf("entry %d (offset %d): ofs-delta base at offset %d not present in pack",
						i, e.Offset, e.BaseOff)
				}
			} else {
				baseIdx, ok = p.IndexByID(e.BaseID)
				if !ok {
					return errDeferred
				}
			}
			if !done[baseIdx] {
				return errDeferred
			}
			depth := 1
			for b := baseIdx; p.Entries[b].Type == ObjOfsDelta || p.Entries[b].Type == ObjRefDelta; {
				depth++
				if p.Entries[b].Type == ObjOfsDelta {
					b, _ = p.IndexByOffset(p.Entries[b].BaseOff)
				} else {
					b, _ = p.IndexByID(p.Entries[b].BaseID)
				}
			}
			if depth > lim.MaxDeltaDepth {
				return fmt.Errorf("entry %d (offset %d): delta depth %d exceeds limit %d",
					i, e.Offset, depth, lim.MaxDeltaDepth)
			}
			body, err := ApplyDelta(bodies[baseIdx], e.Data)
			if err != nil {
				return fmt.Errorf("entry %d (offset %d): %w", i, e.Offset, err)
			}
			bodies[i], types[i] = body, types[baseIdx]
		default:
			return fmt.Errorf("entry %d (offset %d): unsupported type %s", i, e.Offset, e.Type)
		}
		if int64(len(bodies[i])) > lim.MaxObjectSize {
			return fmt.Errorf("entry %d (offset %d): object size %d exceeds limit %d",
				i, e.Offset, len(bodies[i]), lim.MaxObjectSize)
		}
		ids[i] = ObjectID(types[i], bodies[i])
		done[i] = true
		p.RegisterID(i, ids[i])
		return nil
	}

	remaining := n
	for remaining > 0 {
		progress := false
		for i := 0; i < n; i++ {
			if done[i] {
				continue
			}
			err := resolveOne(i)
			if err == errDeferred {
				continue
			}
			if err != nil {
				return nil, err
			}
			progress = true
			remaining--
		}
		if !progress {
			for i := 0; i < n; i++ {
				if !done[i] {
					e := &p.Entries[i]
					return nil, fmt.Errorf("entry %d (offset %d): ref-delta base %x not found in pack (self-contained packs only)",
						i, e.Offset, e.BaseID)
				}
			}
		}
	}

	res := &Result{Objects: make(map[[20]byte][]byte, n)}
	for i := 0; i < n; i++ {
		e := &p.Entries[i]
		rec := Record{
			Offset:   e.Offset,
			Type:     types[i],
			EntryTyp: e.Type,
			Size:     len(bodies[i]),
			ID:       ids[i],
		}
		switch e.Type {
		case ObjOfsDelta:
			rec.BaseDesc = fmt.Sprintf("ofs-delta base @%d", e.BaseOff)
		case ObjRefDelta:
			rec.BaseDesc = fmt.Sprintf("ref-delta base %x", e.BaseID)
		}
		res.Records = append(res.Records, rec)
		res.Objects[ids[i]] = bodies[i]
	}
	return res, nil
}

// Manifest renders the checklist as text.
func (r *Result) Manifest() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s %-9s %-10s %-10s %-40s %s\n", "OFFSET", "TYPE", "ENTRY", "SIZE", "OBJECT-ID", "DELTA-SOURCE")
	for _, rec := range r.Records {
		fmt.Fprintf(&b, "%-10d %-9s %-10s %-10d %x %s\n",
			rec.Offset, rec.Type, rec.EntryTyp, rec.Size, rec.ID, rec.BaseDesc)
	}
	return b.String()
}

// Export writes object files (named by hex identity, raw bytes) and a
// MANIFEST.txt into outDir. The directory is built under a temporary sibling
// and renamed into place only after every write succeeds; an existing
// outDir is never overwritten, and failures leave no partial output.
func (r *Result) Export(outDir string) error {
	if _, err := os.Lstat(outDir); err == nil {
		return fmt.Errorf("output directory %q already exists", outDir)
	} else if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(outDir)
	tmp, err := os.MkdirTemp(parent, ".pack-export-*")
	if err != nil {
		return err
	}
	cleanup := func(err error) error {
		os.RemoveAll(tmp)
		return err
	}

	ids := make([][20]byte, 0, len(r.Objects))
	for id := range r.Objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return hex.EncodeToString(ids[i][:]) < hex.EncodeToString(ids[j][:])
	})
	objDir := filepath.Join(tmp, "objects")
	if err := os.Mkdir(objDir, 0o755); err != nil {
		return cleanup(err)
	}
	for _, id := range ids {
		name := filepath.Join(objDir, hex.EncodeToString(id[:]))
		if err := os.WriteFile(name, r.Objects[id], 0o644); err != nil {
			return cleanup(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, "MANIFEST.txt"), []byte(r.Manifest()), 0o644); err != nil {
		return cleanup(err)
	}
	if err := os.Rename(tmp, outDir); err != nil {
		return cleanup(fmt.Errorf("publishing output: %w", err))
	}
	return nil
}
