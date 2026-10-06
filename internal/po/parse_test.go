package po

import "testing"

func TestParseMultilineEscapesAndContext(t *testing.T) {
	src := "msgid \"\"\nmsgstr \"Content-Type: text/plain; charset=UTF-8\\n\"\n" +
		"\n# a comment\n#, fuzzy\n" +
		"msgctxt \"menu\"\n" +
		"msgid \"Hel\"\n\"lo \\\"world\\\"\\n\"\n" +
		"msgstr \"你好\"\n\"世界\"\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("entries=%d", len(f.Entries))
	}
	e := f.Entries[0]
	if !e.HasCtx || e.Context != "menu" {
		t.Errorf("ctx=%q", e.Context)
	}
	if e.MsgID != "Hello \"world\"\n" {
		t.Errorf("msgid=%q", e.MsgID)
	}
	if e.MsgStr != "你好世界" {
		t.Errorf("msgstr=%q", e.MsgStr)
	}
	if !e.IsFuzzy() {
		t.Error("expected fuzzy")
	}
	if len(e.Comments) != 2 {
		t.Errorf("comments=%v", e.Comments)
	}
}

func TestParsePluralAndRoundTrip(t *testing.T) {
	src := "msgid \"\"\nmsgstr \"hdr\"\n\n" +
		"msgid \"{n} file\"\nmsgid_plural \"{n} files\"\n" +
		"msgstr[0] \"{n} 个文件\"\nmsgstr[1] \"{n} 个文件\"\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	e := f.Entries[0]
	if !e.HasPlural || e.MsgIDPl != "{n} files" || len(e.MsgStrPl) != 2 {
		t.Fatalf("bad plural: %+v", e)
	}
	out := f.String()
	f2, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if f2.Entries[0].MsgStrPl[1] != "{n} 个文件" {
		t.Errorf("round trip mismatch: %q", f2.Entries[0].MsgStrPl)
	}
}

func TestParseObsolete(t *testing.T) {
	src := "msgid \"\"\nmsgstr \"\"\n\n#~ msgid \"gone\"\n#~ msgstr \"走了\"\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 1 || !f.Entries[0].Obsolete || f.Entries[0].MsgStr != "走了" {
		t.Fatalf("obsolete parse: %+v", f.Entries)
	}
}
