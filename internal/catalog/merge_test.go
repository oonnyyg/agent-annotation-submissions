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

const header = "msgid \"\"\nmsgstr \"Language: zh\\n\"\n\n"

func TestContextIsolation(t *testing.T) {
	old := mustParse(t, header+
		"msgctxt \"menu\"\nmsgid \"Open\"\nmsgstr \"菜单打开\"\n\n"+
		"msgid \"Open\"\nmsgstr \"打开\"\n\n"+
		"msgctxt \"\"\nmsgid \"Open\"\nmsgstr \"空上下文\"\n")
	pot := mustParse(t, header+
		"msgctxt \"menu\"\nmsgid \"Open\"\nmsgstr \"\"\n\n"+
		"msgid \"Open\"\nmsgstr \"\"\n\n"+
		"msgctxt \"\"\nmsgid \"Open\"\nmsgstr \"\"\n")
	merged, err := Merge(old, pot, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range merged.Entries {
		key := e.Context
		if !e.HasCtx {
			key = "<none>"
		}
		got[key] = e.MsgStr
	}
	if got["menu"] != "菜单打开" || got["<none>"] != "打开" || got[""] != "空上下文" {
		t.Errorf("context isolation failed: %v", got)
	}
}

func TestPluralChangeClearsTranslation(t *testing.T) {
	old := mustParse(t, header+
		"msgid \"{n} file\"\nmsgid_plural \"{n} files\"\n"+
		"msgstr[0] \"{n} 文件\"\nmsgstr[1] \"{n} 文件们\"\n")
	pot := mustParse(t, header+
		"msgid \"{n} file\"\nmsgid_plural \"{n} files total\"\nmsgstr[0] \"\"\nmsgstr[1] \"\"\n")
	merged, err := Merge(old, pot, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	e := merged.Entries[0]
	if e.MsgStrPl[0] != "" || e.MsgStrPl[1] != "" {
		t.Errorf("expected cleared translations, got %v", e.MsgStrPl)
	}
	if e.MsgIDPl != "{n} files total" {
		t.Errorf("msgid_plural=%q", e.MsgIDPl)
	}
}

func TestSingularToPluralClears(t *testing.T) {
	old := mustParse(t, header+"msgid \"File\"\nmsgstr \"文件\"\n")
	pot := mustParse(t, header+
		"msgid \"File\"\nmsgid_plural \"Files\"\nmsgstr[0] \"\"\nmsgstr[1] \"\"\n")
	merged, err := Merge(old, pot, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Entries[0].MsgStrPl[0] != "" {
		t.Errorf("expected cleared plural slots: %+v", merged.Entries[0])
	}
}

func TestNoFuzzyMatching(t *testing.T) {
	old := mustParse(t, header+"msgid \"Save file\"\nmsgstr \"保存文件\"\n")
	pot := mustParse(t, header+"msgid \"Save files\"\nmsgstr \"\"\n")
	merged, err := Merge(old, pot, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Entries[0].MsgStr != "" {
		t.Error("similar text must not be guessed")
	}
	if len(merged.Entries) != 2 || !merged.Entries[1].Obsolete {
		t.Errorf("old entry should become obsolete: %+v", merged.Entries)
	}
}

func TestDuplicateActiveKeyRejected(t *testing.T) {
	dup := "msgid \"A\"\nmsgstr \"1\"\n\nmsgid \"A\"\nmsgstr \"2\"\n"
	old := mustParse(t, header+dup)
	pot := mustParse(t, header+"msgid \"A\"\nmsgstr \"\"\n")
	if _, err := Merge(old, pot, cfg2); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate error, got %v", err)
	}
}

func TestObsoleteExcludedFromStats(t *testing.T) {
	old := mustParse(t, header+"msgid \"Gone\"\nmsgstr \"没了\"\n\nmsgid \"Stay\"\nmsgstr \"留下\"\n")
	pot := mustParse(t, header+"msgid \"Stay\"\nmsgstr \"\"\n")
	merged, err := Merge(old, pot, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	a := Review(merged, cfg2)
	if a.Stats.Obsolete != 1 || a.Stats.Active != 1 {
		t.Errorf("stats=%+v", a.Stats)
	}
	if !a.ReleaseReady {
		t.Errorf("obsolete entry must not block release: %+v", a.Issues)
	}
	if merged.Header != old.Header {
		t.Errorf("header not preserved: %q", merged.Header)
	}
}
