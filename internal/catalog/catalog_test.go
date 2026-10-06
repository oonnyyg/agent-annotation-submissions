package catalog

import (
	"strings"
	"testing"

	"example.com/gettext-catalog-release/internal/po"
)

func mustParse(t *testing.T, s string) *po.File {
	t.Helper()
	f, err := po.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

var cfg2 = Config{Language: "zh", PluralSlots: 2}

func TestContextIsolationAndKeep(t *testing.T) {
	old := mustParse(t, `msgid ""
msgstr "Language: zh\n"

msgctxt "menu"
msgid "Open"
msgstr "菜单打开"

msgid "Open"
msgstr "打开"
`)
	tmpl := mustParse(t, `msgctxt "menu"
msgid "Open"
msgstr ""

msgctxt ""
msgid "Open"
msgstr ""

msgid "Open"
msgstr ""
`)
	merged, err := Merge(old, tmpl, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(merged.Entries))
	}
	// "menu"/"Open" kept; empty-context and no-context are new (untranslated).
	if merged.Entries[0].Msgstr != "菜单打开" {
		t.Fatalf("ctx entry should keep translation: %q", merged.Entries[0].Msgstr)
	}
	if merged.Entries[1].Msgstr != "" {
		t.Fatal("empty-context key must not inherit the no-context translation")
	}
	if merged.Entries[2].Msgstr != "打开" {
		t.Fatal("no-context key must keep its own translation")
	}
	if merged.Header == nil || !strings.Contains(merged.Header.Msgstr, "Language: zh") {
		t.Fatal("old header language info must be preserved")
	}
}

func TestPluralSourceChangeClearsTranslation(t *testing.T) {
	old := mustParse(t, `msgid "{n} file"
msgid_plural "{n} files"
msgstr[0] "{n} 个文件"
msgstr[1] "{n} 个文件们"
`)
	tmpl := mustParse(t, `msgid "{n} file"
msgid_plural "{n} files total"
msgstr[0] ""
msgstr[1] ""
`)
	merged, err := Merge(old, tmpl, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Entries[0].MsgstrPlural[0] != "" || merged.Entries[0].MsgstrPlural[1] != "" {
		t.Fatalf("plural change must clear translations: %+v", merged.Entries[0].MsgstrPlural)
	}
	a := Validate(merged, cfg2)
	if a.ReleaseReady || a.Stats.Empty != 1 {
		t.Fatalf("audit: %+v", a)
	}
}

func TestPluralKeptWhenIdentical(t *testing.T) {
	old := mustParse(t, `msgid "{n} file"
msgid_plural "{n} files"
msgstr[0] "{n} 文件"
msgstr[1] "{n} 文件们"
`)
	tmpl := mustParse(t, `msgid "{n} file"
msgid_plural "{n} files"
msgstr[0] ""
msgstr[1] ""
`)
	merged, err := Merge(old, tmpl, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Entries[0].MsgstrPlural[1] != "{n} 文件们" {
		t.Fatal("identical plural must keep translations")
	}
	a := Validate(merged, cfg2)
	if !a.ReleaseReady || a.Stats.Translated != 1 {
		t.Fatalf("audit: %+v issues=%v", a, a.Issues)
	}
}

func TestRemovedEntryBecomesObsolete(t *testing.T) {
	old := mustParse(t, "msgid \"old\"\nmsgstr \"旧的\"\n")
	tmpl := mustParse(t, "msgid \"new\"\nmsgstr \"\"\n")
	merged, err := Merge(old, tmpl, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Entries) != 2 || !merged.Entries[1].Obsolete {
		t.Fatalf("obsolete missing: %+v", merged.Entries)
	}
	a := Validate(merged, cfg2)
	if a.Stats.Obsolete != 1 || a.Stats.Total != 1 {
		t.Fatalf("obsolete must not count in release stats: %+v", a.Stats)
	}
	if !strings.Contains(merged.String(), "#~ msgid \"old\"") {
		t.Fatal("obsolete entry must be rendered with #~ prefix")
	}
}

func TestDuplicateActiveKeyRejected(t *testing.T) {
	old := mustParse(t, "msgid \"x\"\nmsgstr \"1\"\n\nmsgid \"x\"\nmsgstr \"2\"\n")
	tmpl := mustParse(t, "msgid \"x\"\nmsgstr \"\"\n")
	if _, err := Merge(old, tmpl, cfg2); err == nil {
		t.Fatal("duplicate active key must be rejected")
	}
}

func TestPlaceholderMismatch(t *testing.T) {
	old := mustParse(t, "msgid \"Hello {name}, you have {count} mails\"\nmsgstr \"你好 {name}\"\n")
	tmpl := mustParse(t, "msgid \"Hello {name}, you have {count} mails\"\nmsgstr \"\"\n")
	merged, err := Merge(old, tmpl, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	a := Validate(merged, cfg2)
	if a.ReleaseReady || a.Stats.Placeholder != 1 {
		t.Fatalf("want placeholder issue: %+v", a)
	}
	found := false
	for _, i := range a.Issues {
		if i.Kind == IssuePlaceholder && strings.Contains(i.Detail, "{count}") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing {count} detail: %+v", a.Issues)
	}
}

func TestPlaceholderDoubleBracesLiteral(t *testing.T) {
	old := mustParse(t, "msgid \"use {{name}} literally\"\nmsgstr \"按字面使用\"\n")
	tmpl := mustParse(t, "msgid \"use {{name}} literally\"\nmsgstr \"\"\n")
	merged, err := Merge(old, tmpl, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	a := Validate(merged, cfg2)
	if !a.ReleaseReady {
		t.Fatalf("double braces are literal, no placeholder expected: %+v", a.Issues)
	}
}

func TestFuzzyAndEmptyIssues(t *testing.T) {
	old := mustParse(t, "#, fuzzy\nmsgid \"a\"\nmsgstr \"甲\"\n\nmsgid \"b\"\nmsgstr \"\"\n")
	tmpl := mustParse(t, "msgid \"a\"\nmsgstr \"\"\n\nmsgid \"b\"\nmsgstr \"\"\n")
	merged, err := Merge(old, tmpl, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	a := Validate(merged, cfg2)
	if a.ReleaseReady || a.Stats.Fuzzy != 1 || a.Stats.Empty != 1 {
		t.Fatalf("audit: %+v", a)
	}
}

func TestPluralSlotCoverage(t *testing.T) {
	old := mustParse(t, "msgid \"f\"\nmsgid_plural \"fs\"\nmsgstr[0] \"单\"\nmsgstr[1] \"复\"\n")
	tmpl := mustParse(t, "msgid \"f\"\nmsgid_plural \"fs\"\nmsgstr[0] \"\"\nmsgstr[1] \"\"\n")
	merged, err := Merge(old, tmpl, Config{PluralSlots: 3})
	if err != nil {
		t.Fatal(err)
	}
	a := Validate(merged, Config{PluralSlots: 3})
	if a.ReleaseReady {
		t.Fatal("missing slot 2 must block release")
	}
	hasEmpty := false
	for _, i := range a.Issues {
		if i.Kind == IssueEmpty && strings.Contains(i.Detail, "slot 2") {
			hasEmpty = true
		}
	}
	if !hasEmpty {
		t.Fatalf("want missing slot 2 issue: %+v", a.Issues)
	}
}

func TestNoFuzzyMatching(t *testing.T) {
	old := mustParse(t, "msgid \"Hello world\"\nmsgstr \"你好世界\"\n")
	tmpl := mustParse(t, "msgid \"Hello worlds\"\nmsgstr \"\"\n")
	merged, err := Merge(old, tmpl, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Entries[0].Msgstr != "" {
		t.Fatal("similar text must not be guessed/matched")
	}
	if !merged.Entries[1].Obsolete {
		t.Fatal("old entry must become obsolete")
	}
}