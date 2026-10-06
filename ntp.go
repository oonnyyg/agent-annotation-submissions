// Package ntpaudit implements an offline NTP exchange auditor. It parses
// recorded NTPv4 client/server exchanges and reconstructs clock offset
// samples without sending packets or touching the system clock.
package ntpaudit

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// PacketLen is the exact size of an NTPv4 packet without extensions.
const PacketLen = 48

// Leap indicator values.
const (
	LeapNone    = 0
	LeapUnknown = 3 // unsynchronized
)

// Modes.
const (
	ModeClient = 3
	ModeServer = 4
)

// Stratum limits.
const (
	StratumKissOfDeath = 0
	StratumMax         = 15
)

// Packet is a parsed 48-byte NTPv4 packet without extension fields.
type Packet struct {
	Leap        uint8
	Version     uint8
	Mode        uint8
	Stratum     uint8
	Poll        int8
	Precision   int8
	ReferenceID [4]byte
	Origin      Timestamp
	Receive     Timestamp
	Transmit    Timestamp
}

// Timestamp is a raw 64-bit NTP timestamp (32-bit era-relative seconds and
// 32-bit fraction). It is expanded to an absolute time only relative to an
// approximate UTC base; see Expand.
type Timestamp struct {
	Seconds  uint32
	Fraction uint32
}

// IsZero reports whether the timestamp is all zero bits.
func (t Timestamp) IsZero() bool { return t.Seconds == 0 && t.Fraction == 0 }

// ParsePacket decodes a 48-byte NTPv4 packet. Any other length is rejected
// so that packets carrying extension fields never pass as plain ones.
func ParsePacket(b []byte) (*Packet, error) {
	if len(b) != PacketLen {
		return nil, fmt.Errorf("packet length %d, want exactly %d bytes (extensions not accepted)", len(b), PacketLen)
	}
	p := &Packet{
		Leap:      b[0] >> 6,
		Version:   (b[0] >> 3) & 0x7,
		Mode:      b[0] & 0x7,
		Stratum:   b[1],
		Poll:      int8(b[2]),
		Precision: int8(b[3]),
	}
	copy(p.ReferenceID[:], b[12:16])
	p.Origin = Timestamp{Seconds: binary.BigEndian.Uint32(b[24:28]), Fraction: binary.BigEndian.Uint32(b[28:32])}
	p.Receive = Timestamp{Seconds: binary.BigEndian.Uint32(b[32:36]), Fraction: binary.BigEndian.Uint32(b[36:40])}
	p.Transmit = Timestamp{Seconds: binary.BigEndian.Uint32(b[40:44]), Fraction: binary.BigEndian.Uint32(b[44:48])}
	return p, nil
}

// ValidateRequest checks that p is a well-formed NTPv4 client request.
func (p *Packet) ValidateRequest() error {
	if p.Version != 4 {
		return fmt.Errorf("request version %d, want 4", p.Version)
	}
	if p.Mode != ModeClient {
		return fmt.Errorf("request mode %d, want client (%d)", p.Mode, ModeClient)
	}
	return nil
}

// ValidateResponse checks that p is a usable NTPv4 server response matching
// the given request. A zero stratum is a Kiss-o'-Death control message and
// is reported separately with its reference code.
func (p *Packet) ValidateResponse(req *Packet) error {
	if p.Version != 4 {
		return fmt.Errorf("response version %d, want 4", p.Version)
	}
	if p.Mode != ModeServer {
		return fmt.Errorf("response mode %d, want server (%d)", p.Mode, ModeServer)
	}
	if p.Stratum == StratumKissOfDeath {
		return &KissOfDeathError{Code: string(p.ReferenceID[:])}
	}
	if p.Stratum > StratumMax {
		return fmt.Errorf("invalid stratum %d", p.Stratum)
	}
	if p.Leap == LeapUnknown {
		return errors.New("server not synchronized (LI=3)")
	}
	if p.Origin != req.Transmit {
		return errors.New("response origin timestamp does not match request transmit timestamp")
	}
	return nil
}

// KissOfDeathError reports a stratum-0 control response. Code is the
// four-byte ASCII reference identifier (e.g. "RATE", "DENY").
type KissOfDeathError struct{ Code string }

func (e *KissOfDeathError) Error() string {
	return fmt.Sprintf("kiss-of-death control response, code %q", e.Code)
}

// eraSeconds is one NTP era: 2^32 seconds.
const eraSeconds int64 = 1 << 32

// ErrEraAmbiguous is returned when the approximate base sits exactly half an
// era away from the candidate timestamp, so two eras are equally near.
var ErrEraAmbiguous = errors.New("ambiguous era: base is equidistant from two era expansions")

// Expand resolves the era-relative 32-bit seconds to an absolute UTC time
// using the era nearest to base. The computation is exact integer
// arithmetic; the 32-bit fraction is converted to nanoseconds without
// floating point so sub-nanosecond distinctions never collapse.
func (t Timestamp) Expand(base time.Time) (time.Time, error) {
	baseSec := base.Unix()
	// Nearest era: round((baseSec - t.Seconds) / 2^32), done with integer
	// arithmetic that is exact for negative values too.
	diff := baseSec - int64(t.Seconds)
	q := diff / eraSeconds
	r := diff % eraSeconds
	switch {
	case 2*r > eraSeconds:
		q++
	case 2*r < -eraSeconds:
		q--
	case 2*r == eraSeconds || 2*r == -eraSeconds:
		return time.Time{}, ErrEraAmbiguous
	}
	sec := q*eraSeconds + int64(t.Seconds)
	// fraction * 1e9 / 2^32 in integer arithmetic; fraction < 2^32 so the
	// product fits in 62 bits.
	nsec := int64(t.Fraction) * int64(time.Second) / eraSeconds
	return time.Unix(sec, nsec).UTC(), nil
}
