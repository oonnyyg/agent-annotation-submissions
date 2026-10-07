package ignore

import (
	"strings"
	"testing"
)

func parse(t *testing.T, line string) Pattern {
	t.Helper()
	p, ok := ParseLine(line, ".gitignore", 1)
	if !ok {
		t.Fatalf("ParseLine(%q) returned ok=false", line)
	}
	return p
}

func TestParseSkipsBlanksAndComments(t *testing.T) {
	for _, line := range []string{"", "   ", "# comment", "#!x"} {
		if _, ok := ParseLine(line, ".gitignore", 1); ok {
			t.Errorf("ParseLine(%q) should not produce a rule", line)
		}
	}
}

func TestEscapedLeadingCharsAreLiteral(t *testing.T) {
	p := parse(t, `\#hash`)
	if p.Negate || p.Body != "#hash" {
		t.Errorf(`\#hash parsed as %+v`, p)
	}
	p = parse(t, `\!bang`)
	if p.Negate || p.Body != "!bang" {
		t.Errorf(`\!bang parsed as %+v`, p)
	}
	p = parse(t, "!real-negate")
	if !p.Negate || p.Body != "real-negate" {
		t.Errorf("negation parsed as %+v", p)
	}
}

func TestTrailingSpaceHandling(t *testing.T) {
	p := parse(t, "foo   ")
	if p.Body != "foo" {
		t.Errorf("unescaped trailing spaces kept: %q", p.Body)
	}
	p = parse(t, `foo\ `)
	if p.Body != "foo " {
		t.Errorf("escaped trailing space lost: %q", p.Body)
	}
}

func TestMatchBasics(t *testing.T) {
	cases := []struct {
		pattern string
		rel     string
		isDir   bool
		want    bool
	}{
		{"*.log", "a/b/c.log", false, true},   // no slash: any depth
		{"*.log", "a/b/c.logx", false, false}, // case/format sensitive
		{"*.LOG", "c.log", false, false},      // case-sensitive
		{"/build", "build", true, true},
		{"/build", "sub/build", true, false}, // anchored to rule dir
		{"doc/*.txt", "doc/a.txt", false, true},
		{"doc/*.txt", "doc/sub/a.txt", false, false},
		{"foo/", "foo", true, true},
		{"foo/", "foo", false, false}, // dir-only
		{"a?c", "abc", false, true},
		{"a?c", "ac", false, false},
		{"[a-c]x", "bx", false, true},
		{"[a-c]x", "dx", false, false},
		{"[!a-c]x", "dx", false, true},
		{"**/temp", "a/b/temp", false, true},
		{"a/**/z", "a/z", false, true},
		{"a/**/z", "a/b/c/z", false, true},
		{"a/**/z", "a/b/c", false, false},
		{`\#file`, "#file", false, true},
		{`\!file`, "!file", false, true},
	}
	for _, c := range cases {
		p := parse(t, c.pattern)
		if got := p.Match(c.rel, c.isDir); got != c.want {
			t.Errorf("pattern %q vs %q (dir=%v): got %v, want %v",
				c.pattern, c.rel, c.isDir, got, c.want)
		}
	}
}

func TestLastMatchWinsWithinLevel(t *testing.T) {
	lv := ParseFile(mustReader("*.log\n!important.log\n"), "", ".gitignore")
	st := Stack{}.Push(lv)
	if d := st.Decide("a.log", false); !d.Ignored {
		t.Error("a.log should be ignored")
	}
	d := st.Decide("important.log", false)
	if d.Ignored || d.Pattern == nil || !d.Pattern.Negate {
		t.Errorf("important.log should be kept by negation, got %+v", d)
	}
}

func TestDeeperLevelOverridesAncestor(t *testing.T) {
	root := ParseFile(mustReader("*.tmp\n"), "", ".gitignore")
	sub := ParseFile(mustReader("!keep.tmp\n"), "sub", "sub/.gitignore")
	st := Stack{}.Push(root).Push(sub)
	if d := st.Decide("sub/keep.tmp", false); d.Ignored {
		t.Error("deeper negation should override ancestor rule")
	}
	if d := st.Decide("top.tmp", false); !d.Ignored {
		t.Error("root rule should still apply outside sub/")
	}
	// Rules in sub/ must not leak to siblings.
	if d := st.Decide("other/keep.tmp", false); !d.Ignored {
		t.Error("sub/ rules must not affect other/ paths")
	}
}

func mustReader(s string) *strings.Reader { return strings.NewReader(s) }
