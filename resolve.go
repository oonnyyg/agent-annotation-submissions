package pack

import (
	"crypto/sha1"
	"fmt"
)

// Limits configures safety bounds for object reconstruction.
type Limits struct {
	MaxObjectSize int64 // maximum reconstructed object size in bytes
	MaxDeltaDepth int   // maximum delta chain depth
}

// DefaultLimits are conservative defaults suitable for typical packs.
var DefaultLimits = Limits{MaxObjectSize: 1 << 30, MaxDeltaDepth: 64}

// Object is a fully reconstructed Git object.
type Object struct {
	Entry    *Entry // pack entry this object came from
	Type     int    // final (non-delta) object type
	Data     []byte // reconstructed object body
	ID       string // hex SHA-1 object id
	BaseFrom string // human-readable base reference for delta entries
	Depth    int    // delta chain depth (0 for plain objects)
}

// ObjectID computes the Git object id for a type and body.
func ObjectID(typ int, body []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "%s %d\x00", TypeName(typ), len(body))
	h.Write(body)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Resolve reconstructs every object in the pack. The pack must be
// self-contained: any missing or unresolvable base is reported with the
// dependent entry's offset.
func (f *File) Resolve(lim Limits) ([]*Object, error) {
	if lim.MaxObjectSize <= 0 {
		lim.MaxObjectSize = DefaultLimits.MaxObjectSize
	}
	if lim.MaxDeltaDepth <= 0 {
		lim.MaxDeltaDepth = DefaultLimits.MaxDeltaDepth
	}

	byOffset := make(map[int64]*Entry, len(f.Entries))
	for _, e := range f.Entries {
		byOffset[e.Offset] = e
	}

	objects := make([]*Object, len(f.Entries))
	resolved := make([]*Object, len(f.Entries))
	byID := make(map[string]*Object)

	var resolve func(idx int, stack map[int]bool) (*Object, error)
	resolve = func(idx int, stack map[int]bool) (*Object, error) {
		if resolved[idx] != nil {
			return resolved[idx], nil
		}
		if stack[idx] {
			return nil, fmt.Errorf("cyclic delta dependency at entry offset %d", f.Entries[idx].Offset)
		}
		stack[idx] = true
		defer delete(stack, idx)

		e := f.Entries[idx]
		obj := &Object{Entry: e}
		switch e.Type {
		case TypeCommit, TypeTree, TypeBlob, TypeTag:
			obj.Type = e.Type
			obj.Data = e.Data
		case TypeOfsDelta, TypeRefDelta:
			var base *Object
			if e.Type == TypeOfsDelta {
				baseEntry, ok := byOffset[e.BaseOffset]
				if !ok {
					return nil, fmt.Errorf("entry at offset %d: ofs-delta base at offset %d not in pack", e.Offset, e.BaseOffset)
				}
				obj.BaseFrom = fmt.Sprintf("ofs:%d", e.BaseOffset)
				b, err := resolve(indexOf(f.Entries, baseEntry), stack)
				if err != nil {
					return nil, fmt.Errorf("entry at offset %d: %w", e.Offset, err)
				}
				base = b
			} else {
				b, err := resolveRefBase(f, e, resolved, byID, resolve, idx, stack)
				if err != nil {
					return nil, err
				}
				base = b
				obj.BaseFrom = "ref:" + e.BaseID
			}
			obj.Depth = base.Depth + 1
			if obj.Depth > lim.MaxDeltaDepth {
				return nil, fmt.Errorf("entry at offset %d: delta depth %d exceeds limit %d", e.Offset, obj.Depth, lim.MaxDeltaDepth)
			}
			data, err := ApplyDelta(base.Data, e.Data)
			if err != nil {
				return nil, fmt.Errorf("entry at offset %d: %w", e.Offset, err)
			}
			obj.Type = base.Type
			obj.Data = data
		default:
			return nil, fmt.Errorf("entry at offset %d: unknown type %d", e.Offset, e.Type)
		}
		if int64(len(obj.Data)) > lim.MaxObjectSize {
			return nil, fmt.Errorf("entry at offset %d: object size %d exceeds limit %d", e.Offset, len(obj.Data), lim.MaxObjectSize)
		}
		obj.ID = ObjectID(obj.Type, obj.Data)
		resolved[idx] = obj
		byID[obj.ID] = obj
		return obj, nil
	}

	for i := range f.Entries {
		obj, err := resolve(i, map[int]bool{})
		if err != nil {
			return nil, err
		}
		objects[i] = obj
	}
	return objects, nil
}

func indexOf(entries []*Entry, e *Entry) int {
	for i, x := range entries {
		if x == e {
			return i
		}
	}
	return -1
}

// resolveRefBase finds the base for a REF_DELTA entry. The base object may
// appear after the delta in the pack, so we scan all entries and resolve the
// matching one on demand.
func resolveRefBase(f *File, e *Entry, resolved []*Object, byID map[string]*Object,
	resolve func(int, map[int]bool) (*Object, error), idx int, stack map[int]bool) (*Object, error) {
	if obj, ok := byID[e.BaseID]; ok {
		return obj, nil
	}
	// The base may not be resolved yet; try resolving every other entry
	// until its id shows up. Resolution is memoized, so this is cheap.
	var firstErr error
	for i := range f.Entries {
		if i == idx || stack[i] {
			continue
		}
		if _, err := resolve(i, stack); err != nil {
			// Remember the failure: the base may exist but be
			// unresolvable, which is a better diagnostic than
			// "not found". Keep scanning either way.
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if obj, ok := byID[e.BaseID]; ok {
			return obj, nil
		}
	}
	if firstErr != nil {
		return nil, fmt.Errorf("entry at offset %d: cannot resolve ref-delta base %s: %w", e.Offset, e.BaseID, firstErr)
	}
	return nil, fmt.Errorf("entry at offset %d: ref-delta base %s not found in pack (not self-contained)", e.Offset, e.BaseID)
}
