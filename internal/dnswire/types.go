package dnswire

// Name is a domain name expressed as a sequence of ASCII labels.
// The root name is an empty slice.
type Name []string

type Header struct {
	ID     uint16 `json:"id"`
	QR     bool   `json:"qr"`
	Opcode uint8  `json:"opcode"`
	AA     bool   `json:"aa"`
	TC     bool   `json:"tc"`
	RD     bool   `json:"rd"`
	RA     bool   `json:"ra"`
	Z      uint8  `json:"z"`
	Rcode  uint8  `json:"rcode"`
}

type Question struct {
	Name  Name   `json:"name"`
	Type  string `json:"type"`
	Class string `json:"class"`
}

// RData is the type-specific payload of a resource record.
// Exactly one field is set for supported types; unknown types carry Hex.
type RData struct {
	A     string   `json:"a,omitempty"`
	CNAME Name     `json:"cname,omitempty"`
	MX    *MXData  `json:"mx,omitempty"`
	TXT   []string `json:"txt,omitempty"`
	Hex   string   `json:"hex,omitempty"`
}

type MXData struct {
	Preference uint16 `json:"preference"`
	Exchange   Name   `json:"exchange"`
}

type Record struct {
	Name  Name   `json:"name"`
	Type  string `json:"type"`
	Class string `json:"class"`
	TTL   uint32 `json:"ttl"`
	RData RData  `json:"rdata"`
}

// Message is a full DNS message. Counts are derived from the slices when
// encoding, so the JSON document never carries redundant count fields.
type Message struct {
	Header      Header     `json:"header"`
	Questions   []Question `json:"questions"`
	Answers     []Record   `json:"answers"`
	Authorities []Record   `json:"authorities"`
	Additionals []Record   `json:"additionals"`
}

var typeCodes = map[string]uint16{
	"A": 1, "CNAME": 5, "MX": 15, "TXT": 16,
}

var codeTypes = map[uint16]string{
	1: "A", 5: "CNAME", 15: "MX", 16: "TXT",
}

const classIN uint16 = 1
