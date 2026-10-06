package audit

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

type entry struct {
	name     string
	typeflag byte
	body     string
	link     string
}

func reg(name, body string) entry  { return entry{name, tar.TypeReg, body, ""} }
func dir(name string) entry        { return entry{name, tar.TypeDir, "", ""} }
func sym(name, link string) entry  { return entry{name, tar.TypeSymlink, "", link} }
func wh(name string) entry         { return entry{name, tar.TypeReg, "", ""} }

func buildLayer(t *testing.T, entries ...entry) *parsedLayer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Typeflag: e.typeflag, Mode: 0o644, Linkname: e.link}
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
	pl, err := ParseLayer(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return pl
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func apply(t *testing.T, eng *Engine, idx int, pl *parsedLayer) {
	t.Helper()
	if err := eng.ApplyLayer(idx, "layer.tar", pl); err != nil {
		t.Fatalf("layer %d: %v", idx, err)
	}
}

func TestNormalizePath(t *testing.T) {
	bad := []string{"/abs", "a/../b", "..", "a\\b", "", "."}
	for _, p := range bad {
		if _, err := NormalizePath(p); err == nil {
			t.Errorf("expected rejection of %q", p)
		}
	}
	p, err := NormalizePath("a//b/./c")
	if err != nil || p != "a/b/c" {
		t.Errorf("got %q, %v", p, err)
	}
}

func TestBasicOverlayAndWhiteout(t *testing.T) {
	eng := NewEngine()
	apply(t, eng, 0, buildLayer(t,
		dir("etc"), reg("etc/keep.conf", "v1"), reg("etc/drop.conf", "x"),
	))
	apply(t, eng, 1, buildLayer(t,
		reg("etc/keep.conf", "v2"), wh("etc/.wh.drop.conf"),
	))
	st := eng.State()
	if _, ok := st["etc/drop.conf"]; ok {
		t.Error("whiteout did not delete etc/drop.conf")
	}
	n := st["etc/keep.conf"]
	if n.SHA256 != sha("v2") || n.Layer != 1 {
		t.Errorf("keep.conf = %+v", n)
	}
	rep := eng.Reports[1]
	if len(rep.Deleted) != 1 || rep.Deleted[0] != "etc/drop.conf" {
		t.Errorf("deleted = %v", rep.Deleted)
	}
	if len(rep.Changed) != 1 || rep.Changed[0] != "etc/keep.conf" {
		t.Errorf("changed = %v", rep.Changed)
	}
}

func TestSameContentRewriteNotAChange(t *testing.T) {
	eng := NewEngine()
	apply(t, eng, 0, buildLayer(t, reg("a.txt", "same")))
	apply(t, eng, 1, buildLayer(t, reg("a.txt", "same")))
	n := eng.State()["a.txt"]
	if n.Layer != 1 {
		t.Errorf("source layer not updated: %+v", n)
	}
	rep := eng.Reports[1]
	if len(rep.Changed) != 0 || len(rep.Rewritten) != 1 {
		t.Errorf("changed=%v rewritten=%v", rep.Changed, rep.Rewritten)
	}
}

func TestOpaqueDirectory(t *testing.T) {
	eng := NewEngine()
	apply(t, eng, 0, buildLayer(t,
		dir("d"), reg("d/old1", "1"), dir("d/sub"), reg("d/sub/old2", "2"), reg("top", "t"),
	))
	// Opaque marker clears lower descendants of d; the layer's own new
	// member under d must survive. Tar order deliberately scrambled.
	apply(t, eng, 1, buildLayer(t,
		reg("d/new", "n"), wh("d/.wh..wh..opq"),
	))
	st := eng.State()
	for _, p := range []string{"d/old1", "d/sub", "d/sub/old2"} {
		if _, ok := st[p]; ok {
			t.Errorf("%s should have been cleared by opaque marker", p)
		}
	}
	if _, ok := st["d/new"]; !ok {
		t.Error("d/new from the same layer must survive the opaque marker")
	}
	if _, ok := st["top"]; !ok {
		t.Error("opaque marker must not affect siblings outside d")
	}
	if _, ok := st["d/.wh..wh..opq"]; ok {
		t.Error("whiteout marker leaked into the manifest")
	}
}

func TestTypeReplacement(t *testing.T) {
	eng := NewEngine()
	apply(t, eng, 0, buildLayer(t,
		dir("d"), reg("d/child", "c"), reg("f", "x"),
	))
	// Directory d becomes a symlink: descendants must be removed.
	// File f becomes a directory with a child.
	apply(t, eng, 1, buildLayer(t,
		sym("d", "/elsewhere"), dir("f"), reg("f/inner", "y"),
	))
	st := eng.State()
	if _, ok := st["d/child"]; ok {
		t.Error("descendant of replaced directory must be removed")
	}
	if n := st["d"]; n.Kind != KindSymlink || n.Target != "/elsewhere" {
		t.Errorf("d = %+v", n)
	}
	if n := st["f"]; n.Kind != KindDir {
		t.Errorf("f = %+v", n)
	}
	if _, ok := st["f/inner"]; !ok {
		t.Error("f/inner missing")
	}
	rep := eng.Reports[1]
	if len(rep.Deleted) != 1 || rep.Deleted[0] != "d/child" {
		t.Errorf("deleted = %v", rep.Deleted)
	}
}

func TestSymlinkParentPath(t *testing.T) {
	eng := NewEngine()
	apply(t, eng, 0, buildLayer(t, sym("bin", "/usr/bin")))
	// A file under a symlink parent, without an explicit directory
	// replacing the symlink, must be rejected and leave state intact.
	bad := buildLayer(t, reg("bin/sh", "x"))
	if err := eng.ApplyLayer(1, "bad.tar", bad); err == nil {
		t.Fatal("expected rejection of file under symlink parent")
	}
	if len(eng.Reports) != 1 {
		t.Fatal("failed layer must not leave partial state")
	}
	if n := eng.State()["bin"]; n.Kind != KindSymlink {
		t.Errorf("state corrupted after failed layer: %+v", n)
	}
	// An explicit directory in the same layer replaces the symlink.
	apply(t, eng, 1, buildLayer(t, dir("bin"), reg("bin/sh", "x")))
	st := eng.State()
	if n := st["bin"]; n.Kind != KindDir {
		t.Errorf("bin = %+v", n)
	}
	if _, ok := st["bin/sh"]; !ok {
		t.Error("bin/sh missing after explicit dir replacement")
	}
}

func TestLayerValidation(t *testing.T) {
	// Duplicate normalized names in one layer.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, n := range []string{"a/b", "a//b"} {
		if err := tw.WriteHeader(&tar.Header{Name: n, Typeflag: tar.TypeReg, Mode: 0o644}); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	if _, err := ParseLayer(bytes.NewReader(buf.Bytes())); err == nil {
		t.Error("expected duplicate-name rejection")
	}
	// Unsupported member type.
	buf.Reset()
	tw = tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "dev/null", Typeflag: tar.TypeChar, Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	if _, err := ParseLayer(bytes.NewReader(buf.Bytes())); err == nil {
		t.Error("expected unsupported-type rejection")
	}
	// Non-directory member with a descendant in the same layer.
	eng := NewEngine()
	pl := buildLayer(t, reg("x", "f"), reg("x/y", "z"))
	if err := eng.ApplyLayer(0, "l.tar", pl); err == nil {
		t.Error("expected non-directory-with-descendant rejection")
	}
	if len(eng.State()) != 0 {
		t.Error("failed layer must not modify state")
	}
}

func TestImplicitParentDirectories(t *testing.T) {
	eng := NewEngine()
	apply(t, eng, 0, buildLayer(t, reg("a/b/c.txt", "deep")))
	st := eng.State()
	for _, p := range []string{"a", "a/b"} {
		if n, ok := st[p]; !ok || n.Kind != KindDir {
			t.Errorf("implicit dir %s missing: %v", p, ok)
		}
	}
	rep := eng.Reports[0]
	if len(rep.Added) != 3 {
		t.Errorf("added = %v", rep.Added)
	}
}

func TestHistoryQuery(t *testing.T) {
	eng := NewEngine()
	apply(t, eng, 0, buildLayer(t, reg("f", "1")))
	apply(t, eng, 1, buildLayer(t, reg("f", "2")))
	apply(t, eng, 2, buildLayer(t, wh(".wh.f")))
	evs := eng.History["f"]
	if len(evs) != 3 {
		t.Fatalf("events = %v", evs)
	}
	want := []string{"added", "changed", "deleted"}
	for i, w := range want {
		if evs[i].Action != w || evs[i].Layer != i {
			t.Errorf("event %d = %+v, want %s", i, evs[i], w)
		}
	}
}
