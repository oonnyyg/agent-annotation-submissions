package hpack

import "fmt"

// HeaderField is a single name/value pair.
type HeaderField struct {
	Name      string
	Value     string
	Sensitive bool
}

func (f HeaderField) size() int {
	return len(f.Name) + len(f.Value) + 32
}

// staticTable is the RFC 7541 Appendix A static table, index 1-based.
var staticTable = []HeaderField{
	{":authority", "", false},
	{":method", "GET", false},
	{":method", "POST", false},
	{":path", "/", false},
	{":path", "/index.html", false},
	{":scheme", "http", false},
	{":scheme", "https", false},
	{":status", "200", false},
	{":status", "204", false},
	{":status", "206", false},
	{":status", "304", false},
	{":status", "400", false},
	{":status", "404", false},
	{":status", "500", false},
	{"accept-charset", "", false},
	{"accept-encoding", "gzip, deflate", false},
	{"accept-language", "", false},
	{"accept-ranges", "", false},
	{"accept", "", false},
	{"access-control-allow-origin", "", false},
	{"age", "", false},
	{"allow", "", false},
	{"authorization", "", false},
	{"cache-control", "", false},
	{"content-disposition", "", false},
	{"content-encoding", "", false},
	{"content-language", "", false},
	{"content-length", "", false},
	{"content-location", "", false},
	{"content-range", "", false},
	{"content-type", "", false},
	{"cookie", "", false},
	{"date", "", false},
	{"etag", "", false},
	{"expect", "", false},
	{"expires", "", false},
	{"from", "", false},
	{"host", "", false},
	{"if-match", "", false},
	{"if-modified-since", "", false},
	{"if-none-match", "", false},
	{"if-range", "", false},
	{"if-unmodified-since", "", false},
	{"last-modified", "", false},
	{"link", "", false},
	{"location", "", false},
	{"max-forwards", "", false},
	{"proxy-authenticate", "", false},
	{"proxy-authorization", "", false},
	{"range", "", false},
	{"referer", "", false},
	{"refresh", "", false},
	{"retry-after", "", false},
	{"server", "", false},
	{"set-cookie", "", false},
	{"strict-transport-security", "", false},
	{"transfer-encoding", "", false},
	{"user-agent", "", false},
	{"vary", "", false},
	{"via", "", false},
	{"www-authenticate", "", false},
}

// EvictRecord describes a dynamic table change for reporting.
type EvictRecord struct {
	Kind  string // "insert" or "evict"
	Field HeaderField
}

// dynamicTable is the encoder/decoder dynamic table. Newest entry first.
type dynamicTable struct {
	entries []HeaderField
	size    int
	maxSize int
}

func newDynamicTable(maxSize int) *dynamicTable {
	return &dynamicTable{maxSize: maxSize}
}

// setMaxSize updates the capacity, evicting as needed.
func (t *dynamicTable) setMaxSize(n int, log *[]EvictRecord) {
	t.maxSize = n
	t.evictToLimit(log)
}

func (t *dynamicTable) evictToLimit(log *[]EvictRecord) {
	for t.size > t.maxSize && len(t.entries) > 0 {
		last := t.entries[len(t.entries)-1]
		t.entries = t.entries[:len(t.entries)-1]
		t.size -= last.size()
		if log != nil {
			*log = append(*log, EvictRecord{Kind: "evict", Field: last})
		}
	}
}

// insert adds f as the newest entry. Oversized entries clear the table
// and are not inserted (RFC 7541 section 4.4).
func (t *dynamicTable) insert(f HeaderField, log *[]EvictRecord) {
	sz := f.size()
	if sz > t.maxSize {
		for len(t.entries) > 0 {
			last := t.entries[len(t.entries)-1]
			t.entries = t.entries[:len(t.entries)-1]
			t.size -= last.size()
			if log != nil {
				*log = append(*log, EvictRecord{Kind: "evict", Field: last})
			}
		}
		return
	}
	t.entries = append([]HeaderField{f}, t.entries...)
	t.size += sz
	if log != nil {
		*log = append(*log, EvictRecord{Kind: "insert", Field: f})
	}
	t.evictToLimit(log)
}

// lookup returns the 1-based HPACK index of an exact name+value match,
// searching the dynamic table first (newest entries get the lowest
// dynamic index), then the static table. Returns 0 if not found.
func (t *dynamicTable) lookup(name, value string) int {
	for i, e := range t.entries {
		if e.Name == name && e.Value == value {
			return len(staticTable) + 1 + i
		}
	}
	for i, e := range staticTable {
		if e.Name == name && e.Value == value {
			return i + 1
		}
	}
	return 0
}

// lookupName returns the 1-based index of any entry with the given
// name, or 0.
func (t *dynamicTable) lookupName(name string) int {
	for i, e := range t.entries {
		if e.Name == name {
			return len(staticTable) + 1 + i
		}
	}
	for i, e := range staticTable {
		if e.Name == name {
			return i + 1
		}
	}
	return 0
}

// at resolves a 1-based HPACK index to a header field.
func (t *dynamicTable) at(idx int) (HeaderField, error) {
	if idx <= 0 {
		return HeaderField{}, fmt.Errorf("invalid index %d", idx)
	}
	if idx <= len(staticTable) {
		return staticTable[idx-1], nil
	}
	d := idx - len(staticTable) - 1
	if d < len(t.entries) {
		return t.entries[d], nil
	}
	return HeaderField{}, fmt.Errorf("index %d out of range (table has %d dynamic entries)", idx, len(t.entries))
}
