package po

// Entry is a single message entry in a PO/POT file.
type Entry struct {
	Comments  []string
	Flags     []string
	Context   string
	HasCtx    bool
	MsgID     string
	MsgIDPl   string
	HasPlural bool
	MsgStr    string
	MsgStrPl  []string
	Obsolete  bool
}

// Key identifies an entry by context + singular id.
type Key struct {
	Ctx    string
	HasCtx bool
	MsgID  string
}

func (e *Entry) Key() Key {
	return Key{Ctx: e.Context, HasCtx: e.HasCtx, MsgID: e.MsgID}
}

func (e *Entry) IsFuzzy() bool {
	for _, f := range e.Flags {
		if f == "fuzzy" {
			return true
		}
	}
	return false
}

// File is a parsed PO/POT document.
type File struct {
	HeaderComments []string
	Header         string
	Entries        []*Entry
}
