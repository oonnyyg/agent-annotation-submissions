// Package ntp parses and validates 48-byte NTPv4 client requests and
// server responses, and expands 32-bit NTP era timestamps against an
// approximate UTC base without going through floating point.
package ntp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

const (
	PacketSize = 48

	ModeClient = 3
	ModeServer = 4

	Version = 4

	// eraSecs is the length of one NTP era: 2^32 seconds.
	eraSecs = int64(1) << 32
	// halfEraSecs is the maximum unambiguous distance from the base.
	halfEraSecs = eraSecs / 2

	ntpEpochOffset = 2208988800 // seconds from 1900-01-01 to 1970-01-01
)

// Packet is a parsed 48-byte NTPv4 message.
type Packet struct {
	Leap      uint8
	Version   uint8
	Mode      uint8
	Stratum   uint8
	Poll      int8
	Precision int8
	RefID     [4]byte
	Origin    Timestamp
	Receive   Timestamp
	Transmit  Timestamp
}

// Timestamp is an NTP short-era timestamp expanded to full 64-bit seconds.
type Timestamp struct {
	Sec  int64  // seconds since 1900-01-01T00:00:00Z, era-expanded
	Frac uint32 // fractional second, units of 2^-32 s
}

// Parse decodes a 48-byte packet. Any other length (including packets
// carrying extension fields) is rejected.
func Parse(b []byte) (*Packet, error) {
	if len(b) != PacketSize {
		return nil, fmt.Errorf("packet length %d, want exactly %d bytes (no extensions)", len(b), PacketSize)
	}
	p := &Packet{
		Leap:      b[0] >> 6,
		Version:   (b[0] >> 3) & 0x7,
		Mode:      b[0] & 0x7,
		Stratum:   b[1],
		Poll:      int8(b[2]),
		Precision: int8(b[3]),
	}
	copy(p.RefID[:], b[12:16])
	p.Origin = Timestamp{Sec: int64(binary.BigEndian.Uint32(b[24:28])), Frac: binary.BigEndian.Uint32(b[28:32])}
	p.Receive = Timestamp{Sec: int64(binary.BigEndian.Uint32(b[32:36])), Frac: binary.BigEndian.Uint32(b[36:40])}
	p.Transmit = Timestamp{Sec: int64(binary.BigEndian.Uint32(b[40:44])), Frac: binary.BigEndian.Uint32(b[44:48])}
	return p, nil
}

// Expand resolves the 32-bit seconds field of t into the NTP era closest
// to base (an approximate UTC time). It returns an error when t sits
// exactly half an era away from base, where both eras are equidistant.
func Expand(t Timestamp, base time.Time) (Timestamp, error) {
	baseNTP := base.Unix() + ntpEpochOffset
	era := baseNTP / eraSecs
	if baseNTP < 0 && baseNTP%eraSecs != 0 {
		era--
	}
	candidate := era*eraSecs + t.Sec
	switch d := candidate - baseNTP; {
	case d > halfEraSecs:
		candidate -= eraSecs
	case d < -halfEraSecs:
		candidate += eraSecs
	case d == halfEraSecs || d == -halfEraSecs:
		return Timestamp{}, fmt.Errorf("timestamp %d is exactly half an era from base, ambiguous era", t.Sec)
	}
	t.Sec = candidate
	return t, nil
}

// sub returns a-b in units of 2^-32 seconds, using integer math only.
func sub(a, b Timestamp) int64 {
	return (a.Sec-b.Sec)<<32 + int64(a.Frac) - int64(b.Frac)
}

// Sub returns a-b as a Duration (rounded to nanoseconds).
func Sub(a, b Timestamp) time.Duration {
	return unitsToDuration(sub(a, b))
}

func unitsToDuration(u int64) time.Duration {
	// Split into whole seconds and a 2^-32 fraction so the
	// multiplication by 1e9 cannot overflow int64.
	sec := u >> 32
	frac := u - (sec << 32) // 0 .. 2^32-1
	return time.Duration(sec)*time.Second + time.Duration(frac*1_000_000_000>>32)
}

// OffsetDelay computes clock offset and round-trip delay from the four
// timestamps of an exchange: t1 client send, t2 server receive,
// t3 server transmit, t4 client receive. All arithmetic is integer.
func OffsetDelay(t1, t2, t3, t4 Timestamp) (offset, delay time.Duration) {
	offUnits := (sub(t2, t1) + sub(t3, t4)) / 2
	delUnits := sub(t4, t1) - sub(t3, t2)
	return unitsToDuration(offUnits), unitsToDuration(delUnits)
}

var ErrZeroTimestamp = errors.New("timestamp is zero")

// Time converts an expanded timestamp to time.Time in UTC.
func (t Timestamp) Time() time.Time {
	return time.Unix(t.Sec-ntpEpochOffset, int64(unitsToDuration(int64(t.Frac)))).UTC()
}
