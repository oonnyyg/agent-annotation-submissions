package catalog

import (
	"fmt"

	"example.com/gettext-catalog-release/internal/po"
)

// Config is the JSON configuration for a target language.
type Config struct {
	Language    string `json:"language"`
	PluralSlots int    `json:"plural_slots"`
}

// Merge combines the old PO with the new POT using exact key matching only.
func Merge(oldPO, pot *po.File, cfg Config) (*po.File, error) {
	if cfg.PluralSlots < 1 {
		return nil, fmt.Errorf("plural_slots must be >= 1")
	}
	oldByKey, err := indexActive(oldPO, "old PO")
	if err != nil {
		return nil, err
	}
	if _, err := indexActive(pot, "POT"); err != nil {
		return nil, err
	}

	out := &po.File{Header: oldPO.Header, HeaderComments: oldPO.HeaderComments}
	matched := map[*po.Entry]bool{}

	for _, pe := range pot.Entries {
		if pe.Obsolete {
			continue
		}
		ne := &po.Entry{
			Context:   pe.Context,
			HasCtx:    pe.HasCtx,
			MsgID:     pe.MsgID,
			MsgIDPl:   pe.MsgIDPl,
			HasPlural: pe.HasPlural,
			Comments:  pe.Comments,
		}
		if oe, ok := oldByKey[pe.Key()]; ok {
			matched[oe] = true
			ne.Comments = oe.Comments
			ne.Flags = oe.Flags
			if oe.HasPlural == pe.HasPlural && (!pe.HasPlural || oe.MsgIDPl == pe.MsgIDPl) {
				ne.MsgStr = oe.MsgStr
				ne.MsgStrPl = append([]string(nil), oe.MsgStrPl...)
			}
		}
		if ne.HasPlural {
			slots := make([]string, cfg.PluralSlots)
			copy(slots, ne.MsgStrPl)
			ne.MsgStrPl = slots
			ne.MsgStr = ""
		} else {
			ne.MsgStrPl = nil
		}
		out.Entries = append(out.Entries, ne)
	}

	for _, oe := range oldPO.Entries {
		if oe.Obsolete || matched[oe] {
			continue
		}
		ob := *oe
		ob.Obsolete = true
		out.Entries = append(out.Entries, &ob)
	}
	return out, nil
}

func indexActive(f *po.File, name string) (map[po.Key]*po.Entry, error) {
	m := map[po.Key]*po.Entry{}
	for _, e := range f.Entries {
		if e.Obsolete {
			continue
		}
		k := e.Key()
		if _, dup := m[k]; dup {
			return nil, fmt.Errorf("%s: duplicate active key (context=%q msgid=%q)", name, k.Ctx, k.MsgID)
		}
		m[k] = e
	}
	return m, nil
}
