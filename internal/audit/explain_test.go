package audit

import "testing"

func TestExplainDirectMatch(t *testing.T) {
	root := tree(t, map[string]string{
		".gitignore":    "*.log\n!important.log\n",
		"a.log":         "x",
		"important.log": "x",
	})
	ex, err := Explain(root, "a.log")
	if err != nil {
		t.Fatal(err)
	}
	if !ex.Ignored || ex.Kind != "direct" || ex.File != ".gitignore" || ex.Line != 1 {
		t.Errorf("unexpected explanation: %+v", ex)
	}
	ex, err = Explain(root, "important.log")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Ignored || ex.Kind != "none" {
		t.Errorf("important.log should be kept: %+v", ex)
	}
}

func TestExplainAncestorBlocked(t *testing.T) {
	root := tree(t, map[string]string{
		".gitignore":        "vendor/\n",
		"vendor/.gitignore": "!keep.go\n",
		"vendor/keep.go":    "x",
	})
	ex, err := Explain(root, "vendor/keep.go")
	if err != nil {
		t.Fatal(err)
	}
	if !ex.Ignored || ex.Kind != "ancestor" || ex.BlockedBy != "vendor" {
		t.Errorf("expected ancestor-blocked explanation: %+v", ex)
	}
	if ex.File != ".gitignore" || ex.Line != 1 {
		t.Errorf("wrong rule trace: %+v", ex)
	}
}

func TestExplainRestoredDir(t *testing.T) {
	root := tree(t, map[string]string{
		".gitignore":  "dist/\n!dist/\n",
		"dist/app.js": "x",
	})
	ex, err := Explain(root, "dist/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Ignored {
		t.Errorf("restored dir child should be kept: %+v", ex)
	}
}

func TestExplainRejectsEscape(t *testing.T) {
	root := tree(t, map[string]string{"a.txt": "x"})
	for _, p := range []string{"..", "../outside", ""} {
		if _, err := Explain(root, p); err == nil {
			t.Errorf("Explain(%q) should fail", p)
		}
	}
}
