package catalog

import "testing"

func TestPlaceholderValidation(t *testing.T) {
	old := mustParse(t, header+
		"msgid \"Hello {name}, you have {count} mails and {count} notices\"\n"+
		"msgstr \"你好 {name}，有 {count} 封邮件\"\n")
	pot := mustParse(t, header+
		"msgid \"Hello {name}, you have {count} mails and {count} notices\"\nmsgstr \"\"\n")
	merged, err := Merge(old, pot, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	a := Review(merged, cfg2)
	if a.ReleaseReady {
		t.Fatal("expected release blocked")
	}
	found := false
	for _, is := range a.Issues {
		if is.Type == "placeholder" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected placeholder issue: %+v", a.Issues)
	}
}

func TestPlaceholderLiteralBraces(t *testing.T) {
	old := mustParse(t, header+
		"msgid \"Use {{name}} literally with {value}\"\n"+
		"msgstr \"字面 {{name}} 加 {value}\"\n")
	pot := mustParse(t, header+
		"msgid \"Use {{name}} literally with {value}\"\nmsgstr \"\"\n")
	merged, err := Merge(old, pot, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	a := Review(merged, cfg2)
	if !a.ReleaseReady {
		t.Errorf("literal braces should pass: %+v", a.Issues)
	}
}

func TestEmptyAndFuzzyIssues(t *testing.T) {
	old := mustParse(t, header+
		"#, fuzzy\nmsgid \"A\"\nmsgstr \"甲\"\n\n"+
		"msgid \"B\"\nmsgstr \"\"\n")
	pot := mustParse(t, header+
		"msgid \"A\"\nmsgstr \"\"\n\nmsgid \"B\"\nmsgstr \"\"\n\nmsgid \"C\"\nmsgstr \"\"\n")
	merged, err := Merge(old, pot, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	a := Review(merged, cfg2)
	if a.ReleaseReady || a.Stats.Fuzzy != 1 || a.Stats.Empty != 2 {
		t.Errorf("audit=%+v issues=%+v", a.Stats, a.Issues)
	}
	types := map[string]bool{}
	for _, is := range a.Issues {
		types[is.Type] = true
	}
	if !types["fuzzy"] || !types["empty"] {
		t.Errorf("missing issue types: %+v", a.Issues)
	}
}

func TestPluralSlotCoverage(t *testing.T) {
	old := mustParse(t, header+
		"msgid \"{n} file\"\nmsgid_plural \"{n} files\"\n"+
		"msgstr[0] \"{n} 文件\"\nmsgstr[1] \"\"\n")
	pot := mustParse(t, header+
		"msgid \"{n} file\"\nmsgid_plural \"{n} files\"\nmsgstr[0] \"\"\nmsgstr[1] \"\"\n")
	merged, err := Merge(old, pot, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	a := Review(merged, cfg2)
	if a.ReleaseReady || a.Stats.Empty != 1 {
		t.Errorf("expected empty slot issue: %+v %+v", a.Stats, a.Issues)
	}
}
