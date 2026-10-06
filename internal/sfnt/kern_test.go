package sfnt

import (
	"encoding/binary"
	"testing"
)

// kernTable builds a version 0 kern table with one horizontal
// format 0 subtable holding the given pairs.
func kernTable(pairs [][3]uint16) []byte {
	body := make([]byte, 8, 8+len(pairs)*6)
	binary.BigEndian.PutUint16(body[0:2], uint16(len(pairs)))
	for _, p := range pairs {
		body = binary.BigEndian.AppendUint16(body, p[0])
		body = binary.BigEndian.AppendUint16(body, p[1])
		body = binary.BigEndian.AppendUint16(body, p[2])
	}
	sub := make([]byte, 6)
	binary.BigEndian.PutUint16(sub[2:4], uint16(6+len(body)))
	binary.BigEndian.PutUint16(sub[4:6], 0x0001) // format 0, horizontal
	sub = append(sub, body...)
	out := make([]byte, 4)
	binary.BigEndian.PutUint16(out[2:4], 1)
	return append(out, sub...)
}

func kernFont(t *testing.T, kern []byte) (*Font, *Metrics) {
	t.Helper()
	sub := cmapFormat12For([][3]uint32{{'A', 'V', 1}})
	cmap := cmapHeader([3]uint32{3, 10, 4 + 8})
	cmap = append(cmap, sub...)
	tables := baseTables(cmap, 3, []uint16{500, 600, 700})
	if kern != nil {
		tables["kern"] = kern
	}
	data := buildFont(t, tables, orderOf(tables))
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	m, err := f.LoadMetrics()
	if err != nil {
		t.Fatalf("LoadMetrics: %v", err)
	}
	return f, m
}

func TestKernAdjustment(t *testing.T) {
	f, m := kernFont(t, kernTable([][3]uint16{{1, 2, 0xFFCE}})) // -50
	if err := f.LoadKern(m); err != nil {
		t.Fatalf("LoadKern: %v", err)
	}
	if got := m.Kern(1, 2); got != -50 {
		t.Errorf("Kern(1,2) = %d; want -50", got)
	}
	if got := m.Kern(2, 1); got != 0 {
		t.Errorf("Kern(2,1) = %d; want 0", got)
	}
}

func TestKernMissingTableIsZero(t *testing.T) {
	f, m := kernFont(t, nil)
	if err := f.LoadKern(m); err != nil {
		t.Fatalf("LoadKern: %v", err)
	}
	if got := m.Kern(1, 2); got != 0 {
		t.Errorf("Kern without table = %d; want 0", got)
	}
}

func TestKernUnsupportedForms(t *testing.T) {
	// Two subtables.
	k := kernTable(nil)
	binary.BigEndian.PutUint16(k[2:4], 2)
	k = append(k, k[4:]...)
	if _, m := kernFont(t, nil); true {
		f, m2 := kernFont(t, k)
		if err := f.LoadKern(m2); err == nil {
			t.Error("expected error for multiple kern subtables")
		}
		_ = m
	}
	// Non-zero version.
	k = kernTable(nil)
	binary.BigEndian.PutUint16(k[0:2], 1)
	f, m2 := kernFont(t, k)
	if err := f.LoadKern(m2); err == nil {
		t.Error("expected error for kern version 1")
	}
	// Format 1 subtable.
	k = kernTable(nil)
	binary.BigEndian.PutUint16(k[8:10], 0x0101)
	f, m2 = kernFont(t, k)
	if err := f.LoadKern(m2); err == nil {
		t.Error("expected error for kern format 1")
	}
}
