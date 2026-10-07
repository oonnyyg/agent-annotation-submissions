package dnswire

import (
	"encoding/json"
	"fmt"
)

// Document is the JSON representation accepted by the encoder. Names are
// arrays of ASCII labels; the root name is an empty array.
type Document struct {
	Header     Header        `json:"header"`
	Questions  []DocQuestion `json:"questions"`
	Answers    []DocRecord   `json:"answers"`
	Authority  []DocRecord   `json:"authority"`
	Additional []DocRecord   `json:"additional"`
}

type DocQuestion struct {
	Name   json.RawMessage `json:"name"`
	QType  uint16          `json:"qtype"`
	QClass uint16          `json:"qclass"`
}

// DocRecord carries typed data: A uses a string, CNAME a label array,
// MX an object, TXT a string array. Raw marks an unknown record and is
// rejected on encode.
type DocRecord struct {
	Name  json.RawMessage `json:"name"`
	Type  uint16          `json:"type"`
	Class uint16          `json:"class"`
	TTL   uint32          `json:"ttl"`
	Data  json.RawMessage `json:"data,omitempty"`
	Raw   string          `json:"raw,omitempty"`
}

type docMX struct {
	Preference uint16          `json:"preference"`
	Exchange   json.RawMessage `json:"exchange"`
}

// parseLabels accepts either a plain label array (["www","example","com"])
// or a decoded name object ({"labels":[...],"occupiedBytes":...,"expandedLen":...}).
func parseLabels(raw json.RawMessage, what string) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s: missing name", what)
	}
	var labels []string
	if err := json.Unmarshal(raw, &labels); err == nil {
		return labels, nil
	}
	var obj struct {
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("%s: name must be a label array or a decoded name object", what)
	}
	return obj.Labels, nil
}

// ToMessage converts a JSON document into an encodable message.
func (d *Document) ToMessage() (*Message, error) {
	m := &Message{Header: d.Header}
	for i, q := range d.Questions {
		labels, err := parseLabels(q.Name, fmt.Sprintf("question %d", i))
		if err != nil {
			return nil, err
		}
		m.Questions = append(m.Questions, Question{
			Name:   NameResult{Labels: labels},
			QType:  q.QType,
			QClass: q.QClass,
		})
	}
	convert := func(recs []DocRecord) ([]Record, error) {
		var out []Record
		for i, dr := range recs {
			labels, err := parseLabels(dr.Name, fmt.Sprintf("record %d", i))
			if err != nil {
				return nil, err
			}
			r := Record{
				Name:  NameResult{Labels: labels},
				Type:  dr.Type,
				Class: dr.Class,
				TTL:   dr.TTL,
				Raw:   dr.Raw,
			}
			if dr.Raw != "" {
				out = append(out, r)
				continue
			}
			if len(dr.Data) == 0 {
				return nil, fmt.Errorf("record %d: missing data", i)
			}
			switch dr.Type {
			case 1:
				var s string
				if err := json.Unmarshal(dr.Data, &s); err != nil {
					return nil, fmt.Errorf("record %d: A data: %w", i, err)
				}
				r.Data = s
			case 5:
				cname, err := parseLabels(dr.Data, fmt.Sprintf("record %d CNAME", i))
				if err != nil {
					return nil, fmt.Errorf("record %d: CNAME data: %w", i, err)
				}
				r.Data = NameResult{Labels: cname}
			case 15:
				var mx docMX
				if err := json.Unmarshal(dr.Data, &mx); err != nil {
					return nil, fmt.Errorf("record %d: MX data: %w", i, err)
				}
				exch, err := parseLabels(mx.Exchange, fmt.Sprintf("record %d MX exchange", i))
				if err != nil {
					return nil, fmt.Errorf("record %d: %w", i, err)
				}
				r.Data = MXData{Preference: mx.Preference, Exchange: NameResult{Labels: exch}}
			case 16:
				var txts []string
				if err := json.Unmarshal(dr.Data, &txts); err != nil {
					return nil, fmt.Errorf("record %d: TXT data: %w", i, err)
				}
				r.Data = txts
			default:
				return nil, fmt.Errorf("record %d: unsupported type %d", i, dr.Type)
			}
			out = append(out, r)
		}
		return out, nil
	}
	var err error
	if m.Answers, err = convert(d.Answers); err != nil {
		return nil, fmt.Errorf("answers: %w", err)
	}
	if m.Authority, err = convert(d.Authority); err != nil {
		return nil, fmt.Errorf("authority: %w", err)
	}
	if m.Additional, err = convert(d.Additional); err != nil {
		return nil, fmt.Errorf("additional: %w", err)
	}
	return m, nil
}
