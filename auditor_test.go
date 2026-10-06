package auditor

import (
	"archive/tar"
	"os"
	"testing"
)

type tarEntry struct {
	name     string
	typeflag byte
	body     string
	linkname string
}

func writeLayer(t *testing.T, entries ...tarEntry) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "layer-*.tar")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Mode:     0o644,
			Linkname: e.linkname,
		}
		if e.typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Size > 0 {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func reg(name, body string) tarEntry { return tarEntry{name: name, typeflag: tar.TypeReg, body: body} }
func dir(name string) tarEntry       { return tarEntry{name: name, typeflag: tar.TypeDir} }
func sym(name, target string) tarEntry {
	return tarEntry{name: name, typeflag: tar.TypeSymlink, linkname: target}
}
func whiteout(name string) tarEntry { return tarEntry{name: name, typeflag: tar.TypeReg} }

func entry(t *testing.T, a *Auditor, p string) *Entry {
	t.Helper()
	e, ok := a.state[p]
	if !ok {
		t.Fatalf("expected %q in state, have %v", p, a.Manifest().Paths)
	}
	return e
}

func TestBasicFileAndImplicitDirs(t *testing.T) {
	l1 := writeLayer(t, reg("app/bin/run", "hello"))
	a := New()
	a.ApplyLayers([]string{l1})
	if a.FailedLayer >= 0 {
		t.Fatal(a.Err)
	}
	e := entry(t, a, "app/bin/run")
	if e.Kind != KindFile || e.Layer != 0 || len(e.SHA256) != 64 {
		t.Fatalf("bad file entry: %+v", e)
	}
	if d := entry(t, a, "app"); !d.Implicit || d.Kind != KindDir {
		t.Fatalf("expected implicit dir, got %+v", d)
	}
}

func TestWhiteoutDelete(t *testing.T) {
	l1 := writeLayer(t, reg("etc/keep", "k"), reg("etc/drop", "d"), reg("etc/sub/x", "x"))
	l2 := writeLayer(t, whiteout("etc/.wh.drop"), whiteout("etc/.wh.sub"))
	a := New()
	a.ApplyLayers([]string{l1, l2})
	if a.FailedLayer >= 0 {
		t.Fatal(a.Err)
	}
	m := a.Manifest()
	for _, p := range []string{"etc/drop", "etc/sub", "etc/sub/x"} {
		if _, ok := m.Entries[p]; ok {
			t.Fatalf("%q should be deleted", p)
		}
	}
	if _, ok := m.Entries["etc/keep"]; !ok {
		t.Fatal("etc/keep should survive")
	}
	rep := a.Reports[1]
	if len(rep.Deleted) != 3 {
		t.Fatalf("expected 3 deletions, got %v", rep.Deleted)
	}
}

func TestOpaqueDirectory(t *testing.T) {
	l1 := writeLayer(t, reg("d/old1", "1"), reg("d/sub/old2", "2"), reg("other/f", "o"))
	l2 := writeLayer(t, whiteout("d/.wh..wh..opq"), reg("d/new", "n"))
	a := New()
	a.ApplyLayers([]string{l1, l2})
	if a.FailedLayer >= 0 {
		t.Fatal(a.Err)
	}
	m := a.Manifest()
	for _, p := range []string{"d/old1", "d/sub", "d/sub/old2"} {
		if _, ok := m.Entries[p]; ok {
			t.Fatalf("%q should be cleared by opaque marker", p)
		}
	}
	if _, ok := m.Entries["d/new"]; !ok {
		t.Fatal("same-layer addition must survive opaque marker")
	}
	if _, ok := m.Entries["other/f"]; !ok {
		t.Fatal("sibling tree must be untouched")
	}
}

func TestDirReplacedByFileRemovesDescendants(t *testing.T) {
	l1 := writeLayer(t, dir("d"), reg("d/a", "a"), reg("d/b/c", "c"))
	l2 := writeLayer(t, reg("d", "now a file"))
	a := New()
	a.ApplyLayers([]string{l1, l2})
	if a.FailedLayer >= 0 {
		t.Fatal(a.Err)
	}
	m := a.Manifest()
	for _, p := range []string{"d/a", "d/b", "d/b/c"} {
		if _, ok := m.Entries[p]; ok {
			t.Fatalf("%q should be removed with replaced dir", p)
		}
	}
	if e := entry(t, a, "d"); e.Kind != KindFile || e.Layer != 1 {
		t.Fatalf("bad replacement entry: %+v", e)
	}
	if len(a.Reports[1].Changed) != 1 || a.Reports[1].Changed[0] != "d" {
		t.Fatalf("expected type change for d, got %+v", a.Reports[1])
	}
}

func TestSymlinkParentRequiresExplicitDir(t *testing.T) {
	l1 := writeLayer(t, sym("etc/conf", "/elsewhere"))
	// Layer 2 writes under the symlink without replacing it: reject.
	l2 := writeLayer(t, reg("etc/conf/app.cfg", "v1"))
	a := New()
	a.ApplyLayers([]string{l1, l2})
	if a.FailedLayer != 1 {
		t.Fatalf("expected layer 1 to fail, got %+v", a)
	}
	// Previous good state is preserved.
	if e := entry(t, a, "etc/conf"); e.Kind != KindSymlink || e.Target != "/elsewhere" {
		t.Fatalf("state corrupted after failed layer: %+v", e)
	}

	// Replacing the symlink with an explicit dir in the same layer works.
	l3 := writeLayer(t, dir("etc/conf"), reg("etc/conf/app.cfg", "v1"))
	b := New()
	b.ApplyLayers([]string{l1, l3})
	if b.FailedLayer >= 0 {
		t.Fatal(b.Err)
	}
	if e := entry(t, b, "etc/conf"); e.Kind != KindDir || e.Implicit {
		t.Fatalf("expected explicit dir, got %+v", e)
	}
	if e := entry(t, b, "etc/conf/app.cfg"); e.Kind != KindFile {
		t.Fatalf("bad child entry: %+v", e)
	}
}

func TestIdenticalRewriteOnlyUpdatesSource(t *testing.T) {
	l1 := writeLayer(t, reg("f", "same"))
	l2 := writeLayer(t, reg("f", "same"))
	l3 := writeLayer(t, reg("f", "different"))
	a := New()
	a.ApplyLayers([]string{l1, l2, l3})
	if a.FailedLayer >= 0 {
		t.Fatal(a.Err)
	}
	if len(a.Reports[1].Changed) != 0 || len(a.Reports[1].Added) != 0 {
		t.Fatalf("identical rewrite must not count as change: %+v", a.Reports[1])
	}
	if e := entry(t, a, "f"); e.Layer != 2 {
		t.Fatalf("source layer should advance on rewrite, got %+v", e)
	}
	if len(a.Reports[2].Changed) != 1 {
		t.Fatalf("content change not reported: %+v", a.Reports[2])
	}
	hist := a.History("f")
	if len(hist) != 3 || hist[1].Action != "rewritten" {
		t.Fatalf("unexpected history: %+v", hist)
	}
}

func TestRejectBadPaths(t *testing.T) {
	cases := [][]tarEntry{
		{reg("/abs/path", "x")},
		{reg("../escape", "x")},
		{reg("a/../../escape", "x")},
		{{name: `win\style`, typeflag: tar.TypeReg, body: "x"}},
		{reg("a/./b", "x"), reg("a/b", "y")},             // duplicate after normalize
		{reg("f", "x"), reg("f/child", "y")},             // file with descendant
		{{name: "dev/null", typeflag: tar.TypeChar}},     // unsupported type
		{{name: "pipe", typeflag: tar.TypeFifo}},         // unsupported type
		{whiteout("d/.wh.x"), reg("d/x", "x")},           // whiteout vs same-layer add
		{{name: "d/.wh.x", typeflag: tar.TypeDir}},       // non-empty whiteout kind
	}
	for i, entries := range cases {
		l := writeLayer(t, entries...)
		a := New()
		a.ApplyLayers([]string{l})
		if a.FailedLayer != 0 {
			t.Errorf("case %d: expected failure, got %+v", i, a)
		}
		if len(a.state) != 0 {
			t.Errorf("case %d: failed layer left state behind", i)
		}
	}
}

func TestFailedLayerStopsProcessing(t *testing.T) {
	l1 := writeLayer(t, reg("ok", "1"))
	bad := writeLayer(t, reg("../nope", "x"))
	l3 := writeLayer(t, reg("later", "3"))
	a := New()
	a.ApplyLayers([]string{l1, bad, l3})
	if a.FailedLayer != 1 {
		t.Fatalf("expected failure at layer 1, got %d", a.FailedLayer)
	}
	if len(a.Reports) != 1 {
		t.Fatalf("later layers must not be applied: %+v", a.Reports)
	}
	if _, ok := a.state["later"]; ok {
		t.Fatal("layer after failure must not be applied")
	}
}

func TestSymlinkTargetRecordedVerbatim(t *testing.T) {
	l1 := writeLayer(t, sym("rel", "../up/../../etc/passwd"))
	a := New()
	a.ApplyLayers([]string{l1})
	if a.FailedLayer >= 0 {
		t.Fatal(a.Err)
	}
	if e := entry(t, a, "rel"); e.Target != "../up/../../etc/passwd" {
		t.Fatalf("symlink target must be recorded verbatim, got %q", e.Target)
	}
}
