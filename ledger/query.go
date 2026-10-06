package ledger

import "sort"

// SessionStatus describes a session for status queries. RevokeReason is
// "replay" for replay-triggered revocation, "explicit" for active
// revocation, and "" while the session is live (tokens may still be
// individually expired or consumed through normal rotation).
type SessionStatus struct {
	SessionID    string   `json:"session_id"`
	ClientID     string   `json:"client_id"`
	Scopes       []string `json:"scopes"`
	Revoked      bool     `json:"revoked"`
	RevokeReason string   `json:"revoke_reason,omitempty"`
	ActiveToken  string   `json:"active_token,omitempty"`
	ExpiresAt    int64    `json:"expires_at"`
	Expired      bool     `json:"expired"`
}

func (l *Ledger) Status(sessionID string) (SessionStatus, error) {
	sess, ok := l.Sessions[sessionID]
	if !ok {
		return SessionStatus{}, ErrUnknownSession
	}
	st := SessionStatus{
		SessionID: sess.ID,
		ClientID:  sess.ClientID,
		Scopes:    append([]string(nil), sess.Scopes...),
		Revoked:   sess.Revoked,
	}
	for _, e := range l.Events {
		if e.SessionID == sessionID && e.Kind == "replay_revoke" {
			st.RevokeReason = "replay"
		} else if e.SessionID == sessionID && e.Kind == "revoke" {
			st.RevokeReason = "explicit"
		}
	}
	var latest *Token
	for _, t := range l.Tokens {
		if t.SessionID == sessionID && !t.Consumed {
			latest = t
		}
	}
	if latest != nil {
		st.ActiveToken = latest.ID
		st.ExpiresAt = latest.ExpiresAt
		st.Expired = l.Now >= latest.ExpiresAt
	}
	return st, nil
}

// History returns the recorded events for a session in order.
func (l *Ledger) History(sessionID string) ([]Event, error) {
	if _, ok := l.Sessions[sessionID]; !ok {
		return nil, ErrUnknownSession
	}
	var out []Event
	for _, e := range l.Events {
		if e.SessionID == sessionID {
			out = append(out, e)
		}
	}
	return out, nil
}

// TokenChain returns the token lineage of a session, root first.
func (l *Ledger) TokenChain(sessionID string) ([]Token, error) {
	if _, ok := l.Sessions[sessionID]; !ok {
		return nil, ErrUnknownSession
	}
	var out []Token
	for _, t := range l.Tokens {
		if t.SessionID == sessionID {
			out = append(out, *t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return depth(l, out[i].ID) < depth(l, out[j].ID)
	})
	return out, nil
}

func depth(l *Ledger, id string) int {
	d := 0
	for t := l.Tokens[id]; t != nil && t.ParentID != ""; t = l.Tokens[t.ParentID] {
		d++
	}
	return d
}
