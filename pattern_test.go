package main

import "testing"

func parse(t *testing.T, line string) Pattern {
	t.Helper()
	p, ok := parseLine(line)
	if !ok {
		t.Fatalf("line %q did not parse as a rule", line)
	}
	return p
}

func TestCommentsBlankAndEscapes(t *testing.T) {
	if _, ok := parseLine(""); ok {
		t.Fatal("empty line should not be a rule")
	}
	if _, ok := parseLine("# comment"); ok {
		t.Fatal("comment should not be a rule")
	}
	p := parse(t, "\\#hash")
	if p.Segments[0] != "#hash" || p.Negate {
		t.Fatalf("escaped # should be literal, got %+v", p)
	}
	p = parse(t, "\\!bang")
	if p.Segments[0] != "!bang" || p.Negate {
		t.Fatalf("escaped ! should be literal, got %+v", p)
	}
	p = parse(t, "!real")
	if !p.Negate {
		t.Fatal("unescaped ! should negate")
	}
}

func TestTrailingSpaces(t *testing.T) {
	p := parse(t, "foo   ")
	if p.Segments[0] != "foo" {
		t.Fatalf("unescaped trailing spaces must be trimmed, got %q", p.Segments[0])
	}
	p = parse(t, "foo\\ ")
	if p.Segments[0] != "foo " {
		t.Fatalf("escaped trailing space must be kept, got %q", p.Segments[0])
	}
}

func TestGlobs(t *testing.T) {
	cases := []struct {
		pat, name string
		want      bool
	}{
		{"*.log", "a.log", true},
		{"*.log", "a.txt", false},
		{"f?o", "foo", true},
		{"f?o", "fo", false},
		{"[a-c]x", "bx", true},
		{"[a-c]x", "dx", false},
		{"[!a-c]x", "dx", true},
		{"[!a-c]x", "bx", false},
		{"Foo", "foo", false}, // case-sensitive
		{"FOO", "FOO", true},
	}
	for _, c := range cases {
		if got := globMatch(c.pat, c.name); got != c.want {
			t.Errorf("globMatch(%q,%q)=%v want %v", c.pat, c.name, got, c.want)
		}
	}
}

func TestDoubleStar(t *testing.T) {
	cases := []struct {
		pat  string
		path string
		want bool
	}{
		{"**/foo", "foo", true},
		{"**/foo", "a/b/foo", true},
		{"a/**/z", "a/z", true},
		{"a/**/z", "a/b/c/z", true},
		{"a/**", "a/b/c", true},
		{"**/b/**", "x/b/y/z", true},
		{"a/**/z", "b/a/z", false},
	}
	for _, c := range cases {
		p := parse(t, c.pat)
		p.Anchored = true
		if got := p.Match(c.path, false); got != c.want {
			t.Errorf("Match(%q,%q)=%v want %v", c.pat, c.path, got, c.want)
		}
	}
}

func TestAnchoredAndDirOnly(t *testing.T) {
	p := parse(t, "/build")
	if !p.Anchored || p.Match("sub/build", true) {
		t.Fatal("leading slash must anchor to base dir")
	}
	if !p.Match("build", true) {
		t.Fatal("anchored rule should match at base")
	}
	p = parse(t, "doc/*.txt")
	if !p.Anchored || !p.Match("doc/a.txt", false) || p.Match("x/doc/a.txt", false) {
		t.Fatal("mid-pattern slash must anchor")
	}
	p = parse(t, "logs/")
	if !p.DirOnly || p.Match("logs", false) || !p.Match("logs", true) {
		t.Fatal("trailing slash must restrict to directories")
	}
	p = parse(t, "temp")
	if p.Anchored || !p.Match("a/b/temp", true) {
		t.Fatal("slashless rule must match at any depth")
	}
}
