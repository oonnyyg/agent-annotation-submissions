package catalog

import (
	"fmt"

	"example.com/gettext-catalog-release/internal/po"
)

// Issue describes one problem found in an active entry.
type Issue struct {
	Type    string `json:"type"` // "empty", "fuzzy", "placeholder"
	Context string `json:"context,omitempty"`
	MsgID   string `json:"msgid"`
	Detail  string `json:"detail"`
}

// Stats summarizes active entries; obsolete entries are excluded.
type Stats struct {
	Active            int `json:"active"`
	Obsolete          int `json:"obsolete"`
	Translated        int `json:"translated"`
	Empty             int `json:"empty"`
	Fuzzy             int `json:"fuzzy"`
	PlaceholderErrors int `json:"placeholder_errors"`
}

// Audit is the review report for a merged catalog.
type Audit struct {
	Language     string `json:"language"`
	ReleaseReady bool   `json:"release_ready"`
	Stats        Stats  `json:"stats"`
	Issues       []Issue `json:"issues"`
}

// Review validates the merged catalog against the config.
func Review(f *po.File, cfg Config) *Audit {
	a := &Audit{Language: cfg.Language, ReleaseReady: true, Issues: []Issue{}}
	for _, e := range f.Entries {
		if e.Obsolete {
			a.Stats.Obsolete++
			continue
		}
		a.Stats.Active++
		base := Issue{Context: ctxLabel(e), MsgID: e.MsgID}
		empty := false
		if e.HasPlural {
			if len(e.MsgStrPl) != cfg.PluralSlots {
				a.add(&base, "placeholder", fmt.Sprintf("plural slots %d, want %d", len(e.MsgStrPl), cfg.PluralSlots))
			}
			for i, s := range e.MsgStrPl {
				if s == "" {
					empty = true
					a.add(&base, "empty", fmt.Sprintf("msgstr[%d] is empty", i))
					continue
				}
				if !EqualCounts(Placeholders(s), Placeholders(e.MsgIDPl)) {
					a.add(&base, "placeholder", fmt.Sprintf("msgstr[%d] placeholders {%s} != msgid_plural {%s}",
						i, sortedKeys(Placeholders(s)), sortedKeys(Placeholders(e.MsgIDPl))))
				}
			}
		} else {
			if e.MsgStr == "" {
				empty = true
				a.add(&base, "empty", "msgstr is empty")
			} else if !EqualCounts(Placeholders(e.MsgStr), Placeholders(e.MsgID)) {
				a.add(&base, "placeholder", fmt.Sprintf("msgstr placeholders {%s} != msgid {%s}",
					sortedKeys(Placeholders(e.MsgStr)), sortedKeys(Placeholders(e.MsgID))))
			}
		}
		if empty {
			a.Stats.Empty++
		} else {
			a.Stats.Translated++
		}
		if e.IsFuzzy() {
			a.Stats.Fuzzy++
			a.add(&base, "fuzzy", "entry marked fuzzy")
		}
	}
	for _, is := range a.Issues {
		if is.Type == "placeholder" {
			a.Stats.PlaceholderErrors++
		}
	}
	if len(a.Issues) > 0 {
		a.ReleaseReady = false
	}
	return a
}

func (a *Audit) add(base *Issue, typ, detail string) {
	a.Issues = append(a.Issues, Issue{Type: typ, Context: base.Context, MsgID: base.MsgID, Detail: detail})
}

func ctxLabel(e *po.Entry) string {
	if !e.HasCtx {
		return ""
	}
	return e.Context
}
