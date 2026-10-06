package ntp

import "fmt"

// RejectReason describes why a packet failed validation.
type RejectReason struct {
	Kind     string // machine-readable category
	Detail   string // human-readable explanation
	KissCode string // set only when Kind == "kiss-of-death"
}

func (r *RejectReason) Error() string {
	if r.KissCode != "" {
		return fmt.Sprintf("%s: %s (kiss code %q)", r.Kind, r.Detail, r.KissCode)
	}
	return fmt.Sprintf("%s: %s", r.Kind, r.Detail)
}

// ValidateRequest checks that p is a well-formed NTPv4 client request.
func ValidateRequest(p *Packet) *RejectReason {
	if p.Version != Version {
		return &RejectReason{Kind: "bad-version", Detail: fmt.Sprintf("request version %d, want %d", p.Version, Version)}
	}
	if p.Mode != ModeClient {
		return &RejectReason{Kind: "bad-mode", Detail: fmt.Sprintf("request mode %d, want %d (client)", p.Mode, ModeClient)}
	}
	return nil
}

// ValidateResponse checks that p is a well-formed NTPv4 server response
// matching the given request's transmit timestamp. Stratum-0 kiss-of-death
// replies are reported with their control code, not as generic failures.
func ValidateResponse(p, req *Packet) *RejectReason {
	if p.Version != Version {
		return &RejectReason{Kind: "bad-version", Detail: fmt.Sprintf("response version %d, want %d", p.Version, Version)}
	}
	if p.Mode != ModeServer {
		return &RejectReason{Kind: "bad-mode", Detail: fmt.Sprintf("response mode %d, want %d (server)", p.Mode, ModeServer)}
	}
	if p.Leap == 3 {
		return &RejectReason{Kind: "unsynchronized", Detail: "leap indicator 3 (clock unsynchronized)"}
	}
	if p.Stratum == 0 {
		code := string(p.RefID[:])
		return &RejectReason{Kind: "kiss-of-death", Detail: "stratum 0 control message", KissCode: code}
	}
	if p.Stratum >= 16 {
		return &RejectReason{Kind: "bad-stratum", Detail: fmt.Sprintf("stratum %d is reserved/invalid", p.Stratum)}
	}
	if p.Origin != req.Transmit {
		return &RejectReason{Kind: "origin-mismatch", Detail: "response origin timestamp does not match request transmit timestamp"}
	}
	return nil
}
