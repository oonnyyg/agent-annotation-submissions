// Package catalog merges an old PO catalog with a new POT template
// and validates the result for release.
package catalog

import (
	"fmt"
	"sort"

	"example.com/gettext-catalog-release/internal/po"
)

// Config is the JSON configuration for the tool.
type Config struct {
	Language   string `json:"language"`    // informational only
	PluralSlots int   `json:"plural_slots"` // number of plural forms for the target language
}

// IssueKind classifies audit findings.
type IssueKind string

const (
	IssueEmpty      IssueKind = "empty_translation"
	IssueFuzzy      IssueKind = "fuzzy"
	IssuePlaceholder IssueKind = "placeholder_mismatch"
	IssueDuplicate  IssueKind = "duplicate_key"
)

// Issue is one audit finding for an entry.
type Issue struct {
	Kind    IssueKind `json:"kind"`
	Context *string   `json:"context"`
	Msgid   string    `json:"msgid"`
	Detail  string    `json:"detail,omitempty"`
}

// Audit is the review report written next to the updated PO.
type Audit struct {
	ReleaseReady bool      `json:"release_ready"`
	Stats        Stats     `json:"stats"`
	Issues       []Issue   `json:"issues"`
}

// Stats summarizes publishable (non-obsolete) entries.
type Stats struct {
	Total       int `json:"total"`
	Translated  int `json:"translated"`
	Empty       int `json:"empty"`
	Fuzzy       int `json:"fuzzy"`
	Placeholder int `json:"placeholder_errors"`
	Obsolete    int `json:"obsolete"`
}

// Merge combines the old translated catalog with the new template.
// Old translations are kept only when the key (context+msgid) matches
// exactly and, for plurals, the old msgid_plural equals the new one.
// Removed entries become obsolete records. Duplicate active keys are an error.
func Merge(old, tmpl *po.File, cfg Config) (*po.File, error) {
	out := &po.File{}
	if old.Header != nil {
		h := *old.Header
		out.Header = &h // keep old header (language info) untouched
	} else if tmpl.Header != nil {
		h := *tmpl.Header
		out.Header = &h
	}

	oldByKey := map[po.Key]*po.Entry{}
	for _, e := range old.Entries {
		if e.Obsolete {
			continue
		}
		k := e.Key()
		if _, dup := oldByKey[k]; dup {
			return nil, fmt.Errorf("duplicate active key in old catalog: ctxt=%q msgid=%q", k.Ctxt, k.Msgid)
		}
		oldByKey[k] = e
	}

	seen := map[po.Key]bool{}
	used := map[*po.Entry]bool{}
	for _, t := range tmpl.Entries {
		if t.Obsolete {
			continue
		}
		k := t.Key()
		if seen[k] {
			return nil, fmt.Errorf("duplicate active key in template: ctxt=%q msgid=%q", k.Ctxt, k.Msgid)
		}
		seen[k] = true
		merged := *t
		merged.MsgstrPlural = map[int]string{}
		if o, ok := oldByKey[k]; ok {
			used[o] = true
			// Keep old translation only if plural source text also matches.
			if o.HasPlural == t.HasPlural && (!t.HasPlural || o.MsgidPlural == t.MsgidPlural) {
				merged.Msgstr = o.Msgstr
				for i, s := range o.MsgstrPlural {
					merged.MsgstrPlural[i] = s
				}
				merged.Fuzzy = o.Fuzzy
			}
			// Carry translator comments forward.
			merged.Comments = mergeComments(t.Comments, o.Comments)
		}
		out.Entries = append(out.Entries, &merged)
	}

	// Entries present in old but absent from the template become obsolete.
	for _, e := range old.Entries {
		if e.Obsolete || used[e] {
			continue
		}
		ob := *e
		ob.Obsolete = true
		out.Entries = append(out.Entries, &ob)
	}
	return out, nil
}

func mergeComments(newer, older []string) []string {
	set := map[string]bool{}
	var out []string
	for _, c := range append(append([]string{}, newer...), older...) {
		if !set[c] {
			set[c] = true
			out = append(out, c)
		}
	}
	return out
}

// Validate checks the merged catalog and builds the audit report.
// Obsolete entries are excluded from release statistics.
func Validate(f *po.File, cfg Config) Audit {
	a := Audit{ReleaseReady: true}
	ctxPtr := func(e *po.Entry) *string {
		if !e.HasCtxt {
			return nil
		}
		s := e.Msgctxt
		return &s
	}
	add := func(e *po.Entry, kind IssueKind, detail string) {
		a.Issues = append(a.Issues, Issue{Kind: kind, Context: ctxPtr(e), Msgid: e.Msgid, Detail: detail})
		a.ReleaseReady = false
	}

	for _, e := range f.Entries {
		if e.Obsolete {
			a.Stats.Obsolete++
			continue
		}
		a.Stats.Total++
		if e.Fuzzy {
			a.Stats.Fuzzy++
			add(e, IssueFuzzy, "entry marked fuzzy")
		}

		// Plural slot coverage.
		if e.HasPlural {
			if cfg.PluralSlots < 1 {
				add(e, IssuePlaceholder, "plural_slots must be >= 1")
			}
			for i := 0; i < cfg.PluralSlots; i++ {
				if _, ok := e.MsgstrPlural[i]; !ok {
					add(e, IssueEmpty, fmt.Sprintf("missing plural slot %d", i))
				}
			}
			for i := range e.MsgstrPlural {
				if i < 0 || i >= cfg.PluralSlots {
					add(e, IssuePlaceholder, fmt.Sprintf("plural slot %d outside configured range 0..%d", i, cfg.PluralSlots-1))
				}
			}
		} else if len(e.MsgstrPlural) > 0 {
			add(e, IssuePlaceholder, "singular entry must not have plural translations")
		}

		// Empty translation check.
		empty := false
		if e.HasPlural {
			for i := 0; i < cfg.PluralSlots; i++ {
				if e.MsgstrPlural[i] == "" {
					empty = true
				}
			}
		} else if e.Msgstr == "" {
			empty = true
		}
		if empty {
			a.Stats.Empty++
			add(e, IssueEmpty, "empty translation")
		} else {
			a.Stats.Translated++
		}

		// Placeholder check: each non-empty translation must carry the same
		// placeholders (with counts) as its source text.
		if e.HasPlural {
			src := placeholders(e.MsgidPlural)
			for i := 0; i < cfg.PluralSlots; i++ {
				t := e.MsgstrPlural[i]
				if t == "" {
					continue
				}
				if d := comparePlaceholders(src, placeholders(t)); d != "" {
					a.Stats.Placeholder++
					add(e, IssuePlaceholder, fmt.Sprintf("slot %d: %s", i, d))
				}
			}
		} else if e.Msgstr != "" {
			src := placeholders(e.Msgid)
			if d := comparePlaceholders(src, placeholders(e.Msgstr)); d != "" {
				a.Stats.Placeholder++
				add(e, IssuePlaceholder, d)
			}
		}
	}
	sort.SliceStable(a.Issues, func(i, j int) bool {
		if a.Issues[i].Msgid != a.Issues[j].Msgid {
			return a.Issues[i].Msgid < a.Issues[j].Msgid
		}
		return a.Issues[i].Kind < a.Issues[j].Kind
	})
	return a
}