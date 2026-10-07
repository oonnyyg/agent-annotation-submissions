package hpack

// Field is a single header field with its raw name and value.
type Field struct {
	Name  string
	Value string
	// NeverIndex marks the field as sensitive. Sensitive fields are always
	// encoded with the never-indexed literal representation and are never
	// inserted into the dynamic table.
	NeverIndex bool
}

// EntrySize returns the RFC 7541 §4.1 size of an entry: the sum of the
// name and value byte lengths plus 32 bytes of per-entry overhead.
func (f Field) EntrySize() int {
	return len(f.Name) + len(f.Value) + 32
}

// staticEntries is the static table defined in RFC 7541 Appendix A.
// Indexing is 1-based; element 0 corresponds to static index 1.
var staticEntries = []Field{
	{Name: ":authority", Value: ""},
	{Name: ":method", Value: "GET"},
	{Name: ":method", Value: "POST"},
	{Name: ":path", Value: "/"},
	{Name: ":path", Value: "/index.html"},
	{Name: ":scheme", Value: "http"},
	{Name: ":scheme", Value: "https"},
	{Name: ":status", Value: "200"},
	{Name: ":status", Value: "204"},
	{Name: ":status", Value: "206"},
	{Name: ":status", Value: "304"},
	{Name: ":status", Value: "400"},
	{Name: ":status", Value: "404"},
	{Name: ":status", Value: "500"},
	{Name: "accept-charset", Value: ""},
	{Name: "accept-encoding", Value: "gzip, deflate"},
	{Name: "accept-language", Value: ""},
	{Name: "accept-ranges", Value: ""},
	{Name: "accept", Value: ""},
	{Name: "access-control-allow-origin", Value: ""},
	{Name: "age", Value: ""},
	{Name: "allow", Value: ""},
	{Name: "authorization", Value: ""},
	{Name: "cache-control", Value: ""},
	{Name: "content-disposition", Value: ""},
	{Name: "content-encoding", Value: ""},
	{Name: "content-language", Value: ""},
	{Name: "content-length", Value: ""},
	{Name: "content-location", Value: ""},
	{Name: "content-range", Value: ""},
	{Name: "content-type", Value: ""},
	{Name: "cookie", Value: ""},
	{Name: "date", Value: ""},
	{Name: "etag", Value: ""},
	{Name: "expect", Value: ""},
	{Name: "expires", Value: ""},
	{Name: "from", Value: ""},
	{Name: "host", Value: ""},
	{Name: "if-match", Value: ""},
	{Name: "if-modified-since", Value: ""},
	{Name: "if-none-match", Value: ""},
	{Name: "if-range", Value: ""},
	{Name: "if-unmodified-since", Value: ""},
	{Name: "last-modified", Value: ""},
	{Name: "link", Value: ""},
	{Name: "location", Value: ""},
	{Name: "max-forwards", Value: ""},
	{Name: "proxy-authenticate", Value: ""},
	{Name: "proxy-authorization", Value: ""},
	{Name: "range", Value: ""},
	{Name: "referer", Value: ""},
	{Name: "refresh", Value: ""},
	{Name: "retry-after", Value: ""},
	{Name: "server", Value: ""},
	{Name: "set-cookie", Value: ""},
	{Name: "strict-transport-security", Value: ""},
	{Name: "transfer-encoding", Value: ""},
	{Name: "user-agent", Value: ""},
	{Name: "vary", Value: ""},
	{Name: "via", Value: ""},
	{Name: "www-authenticate", Value: ""},
}

const staticLen = len(staticEntries)

// Eviction records one entry dropped from a dynamic table.
type Eviction struct {
	Name  string
	Value string
	// Reason is "capacity" for eviction due to insertion or a capacity
	// shrinkage, and "oversize" when an entry too large for the current
	// capacity empties the table without being inserted.
	Reason string
}

// DynamicTable is an RFC 7541 dynamic table with a negotiated capacity
// ceiling. Entries are stored newest-first, so position 0 corresponds to
// dynamic index 1 and absolute static-table index staticLen+1.
type DynamicTable struct {
	entries  []Field
	size     int
	capacity int
	maxCap   int
}

// NewDynamicTable creates a dynamic table whose negotiated capacity may
// never exceed maxCapacity (the SETTINGS_HEADER_TABLE_SIZE limit).
func NewDynamicTable(maxCapacity int) *DynamicTable {
	if maxCapacity < 0 {
		maxCapacity = 0
	}
	return &DynamicTable{capacity: maxCapacity, maxCap: maxCapacity}
}

// Size returns the sum of the sizes of the current entries.
func (t *DynamicTable) Size() int { return t.size }

// Capacity returns the current capacity.
func (t *DynamicTable) Capacity() int { return t.capacity }

// MaxCapacity returns the negotiated upper bound on capacity updates.
func (t *DynamicTable) MaxCapacity() int { return t.maxCap }

// Entries returns the stored entries, newest first.
func (t *DynamicTable) Entries() []Field {
	out := make([]Field, len(t.entries))
	copy(out, t.entries)
	return out
}

// lookupAbsolute returns the field at the RFC 7541 absolute (1-based) index.
func (t *DynamicTable) lookupAbsolute(index int) (Field, bool) {
	if index < 1 {
		return Field{}, false
	}
	if index <= staticLen {
		return staticEntries[index-1], true
	}
	pos := index - staticLen - 1
	if pos >= len(t.entries) {
		return Field{}, false
	}
	return t.entries[pos], true
}

// lookupName returns the name at absolute index, used for indexed-name
// literal representations.
func (t *DynamicTable) lookupName(index int) (string, bool) {
	f, ok := t.lookupAbsolute(index)
	if !ok {
		return "", false
	}
	return f.Name, true
}

// SetCapacity applies a dynamic table size update (RFC 7541 §4.3.1),
// evicting oldest entries until the table fits newCap. It returns the
// evicted entries. newCap must not exceed the negotiated maximum.
func (t *DynamicTable) SetCapacity(newCap int) ([]Eviction, error) {
	if newCap < 0 || newCap > t.maxCap {
		return nil, ErrTableSizeUpdate
	}
	t.capacity = newCap
	var evicted []Eviction
	for t.size > t.capacity && len(t.entries) > 0 {
		oldest := t.entries[len(t.entries)-1]
		t.entries = t.entries[:len(t.entries)-1]
		t.size -= oldest.EntrySize()
		evicted = append(evicted, Eviction{
			Name: oldest.Name, Value: oldest.Value, Reason: "capacity",
		})
	}
	return evicted, nil
}

// add inserts a new entry at the beginning of the table, evicting oldest
// entries first. Per RFC 7541 §4.4, an entry larger than the current
// capacity empties the table and is not inserted.
func (t *DynamicTable) add(f Field) []Eviction {
	var evicted []Eviction
	entrySize := f.EntrySize()
	for t.size+entrySize > t.capacity && len(t.entries) > 0 {
		oldest := t.entries[len(t.entries)-1]
		t.entries = t.entries[:len(t.entries)-1]
		t.size -= oldest.EntrySize()
		evicted = append(evicted, Eviction{
			Name: oldest.Name, Value: oldest.Value, Reason: "capacity",
		})
	}
	if entrySize <= t.capacity {
		t.entries = append(t.entries, Field{})
		copy(t.entries[1:], t.entries[:len(t.entries)-1])
		t.entries[0] = f
		t.size += entrySize
	} else {
		evicted = append(evicted, Eviction{
			Name: f.Name, Value: f.Value, Reason: "oversize",
		})
	}
	return evicted
}

// findExact returns the absolute index of the first (most recent) entry
// matching both name and value, or 0 when there is no match.
func (t *DynamicTable) findExact(name, value string) int {
	for i, e := range t.entries {
		if e.Name == name && e.Value == value {
			return staticLen + i + 1
		}
	}
	return 0
}

// findName returns the absolute index of the first entry matching name.
func (t *DynamicTable) findName(name string) int {
	for i, e := range t.entries {
		if e.Name == name {
			return staticLen + i + 1
		}
	}
	return 0
}

// Reset drops all entries without changing the capacity.
func (t *DynamicTable) Reset() {
	t.entries = nil
	t.size = 0
}
