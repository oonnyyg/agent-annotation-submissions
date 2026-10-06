package po

import "testing"

func TestParseMultilineEscapesAndContext(t *testing.T) {
	src := `msgid ""
msgstr "Project-Id-Version: x\n"
"Language: zh\n"

# translator note
#, fuzzy
msgctxt "menu"
msgid "File \"Open\""
msgstr "文件\n换行"

msgctxt ""
msgid "File \"Open\""
msgstr "空上下文"

msgid "File \"Open\""
msgstr "无上下文"

msgid "cat"
msgid_plural "cats"
msgstr[0] "猫"
msgstr[1] "猫们"
`
	f, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if f.Header == nil || f.Header.Msgstr != "Project-Id-Version: x\nLanguage: zh\n" {
		t.Fatalf("bad header: %+v", f.Header)
	}
	if len(f.Entries) != 4 {
		t.Fatalf("want 4 entries, got %d", len(f.Entries))
	}
	// context isolation: three distinct keys
	keys := map[Key]bool{}
	for _, e := range f.Entries {
		keys[e.Key()] = true
	}
	if len(keys) != 4 {
		t.Fatalf("keys not isolated: %v", keys)
	}
	e0 := f.Entries[0]
	if !e0.Fuzzy || e0.Msgctxt != "menu" || e0.Msgid != `File "Open"` || e0.Msgstr != "文件\n换行" {
		t.Fatalf("bad entry 0: %+v", e0)
	}
	if f.Entries[1].Msgctxt != "" || !f.Entries[1].HasCtxt {
		t.Fatal("empty context must have HasCtxt")
	}
	if f.Entries[2].HasCtxt {
		t.Fatal("no-context entry must not have HasCtxt")
	}
	pl := f.Entries[3]
	if !pl.HasPlural || pl.MsgidPlural != "cats" || pl.MsgstrPlural[1] != "猫们" {
		t.Fatalf("bad plural: %+v", pl)
	}
}

func TestRoundTrip(t *testing.T) {
	src := "msgid \"\"\nmsgstr \"Language: zh\\n\"\n\nmsgid \"a\\nb\"\nmsgstr \"c\\nd\"\n\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	f2, err := Parse(f.String())
	if err != nil {
		t.Fatal(err)
	}
	if f2.Entries[0].Msgid != "a\nb" || f2.Entries[0].Msgstr != "c\nd" {
		t.Fatalf("round trip failed: %+v", f2.Entries[0])
	}
}

func TestParseObsolete(t *testing.T) {
	src := "#~ msgid \"gone\"\n#~ msgstr \"走了\"\n"
	f, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 1 || !f.Entries[0].Obsolete || f.Entries[0].Msgid != "gone" {
		t.Fatalf("bad obsolete: %+v", f.Entries)
	}
}