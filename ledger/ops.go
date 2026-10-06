package ledger

import "fmt"

// Advance moves the explicit monotonic clock forward.
func (l *Ledger) Advance(seconds int64) error {
	if seconds < 0 {
		return fmt.Errorf("%w: negative advance", ErrInvalidRequest)
	}
	l.Now += seconds
	return nil
}

// CreateSession starts a new authorization session with an initial token.
func (l *Ledger) CreateSession(sessionID, clientID, tokenID string, scopes []string, expiresAt int64) error {
	if sessionID == "" || clientID == "" || tokenID == "" {
		return fmt.Errorf("%w: empty id", ErrInvalidRequest)
	}
	if _, ok := l.Sessions[sessionID]; ok {
		return ErrSessionExists
	}
	if _, ok := l.Tokens[tokenID]; ok {
		return ErrDuplicateID
	}
	l.Sessions[sessionID] = &Session{
		ID:       sessionID,
		ClientID: clientID,
		Scopes:   append([]string(nil), scopes...),
	}
	l.Tokens[tokenID] = &Token{
		ID:        tokenID,
		SessionID: sessionID,
		Scopes:    append([]string(nil), scopes...),
		ExpiresAt: expiresAt,
	}
	l.record("create", sessionID, tokenID, "session created")
	return nil
}

// Rotate consumes an active token and creates a unique successor that
// inherits the absolute deadline and may only keep or narrow scopes.
//
// If the presented token was already consumed and the caller is the bound
// client, this is a replay: the entire token family is revoked and the
// trigger is recorded. Ordinary denials leave the ledger untouched.
func (l *Ledger) Rotate(tokenID, clientID, newTokenID string, scopes []string) error {
	tok, ok := l.Tokens[tokenID]
	if !ok {
		return ErrUnknownToken
	}
	sess := l.Sessions[tok.SessionID]
	if sess.ClientID != clientID {
		return ErrClientMismatch
	}
	if sess.Revoked {
		return ErrSessionRevoked
	}
	if tok.Consumed {
		// Replay of a successfully consumed token by the bound client:
		// revoke the whole family regardless of how many successors exist.
		l.revokeFamily(sess, "replay_revoke", tokenID,
			fmt.Sprintf("replay of consumed token %s by client %s", tokenID, clientID))
		return ErrTokenConsumed
	}
	if l.Now >= tok.ExpiresAt {
		return ErrTokenExpired
	}
	if newTokenID == "" {
		return fmt.Errorf("%w: empty new token id", ErrInvalidRequest)
	}
	if _, exists := l.Tokens[newTokenID]; exists {
		return ErrDuplicateID
	}
	if scopes == nil {
		scopes = tok.Scopes
	}
	if !subsetOf(scopes, tok.Scopes) {
		return ErrScopeWidening
	}
	tok.Consumed = true
	l.Tokens[newTokenID] = &Token{
		ID:        newTokenID,
		SessionID: sess.ID,
		ParentID:  tokenID,
		Scopes:    append([]string(nil), scopes...),
		ExpiresAt: tok.ExpiresAt,
	}
	l.record("rotate", sess.ID, newTokenID, "rotated from "+tokenID)
	return nil
}

// Revoke explicitly revokes the whole token family of a session.
func (l *Ledger) Revoke(sessionID string) error {
	sess, ok := l.Sessions[sessionID]
	if !ok {
		return ErrUnknownSession
	}
	if sess.Revoked {
		return nil
	}
	l.revokeFamily(sess, "revoke", "", "explicit revocation")
	return nil
}

func (l *Ledger) revokeFamily(sess *Session, kind, tokenID, detail string) {
	sess.Revoked = true
	for _, t := range l.Tokens {
		if t.SessionID == sess.ID {
			t.Consumed = true
		}
	}
	l.record(kind, sess.ID, tokenID, detail)
}
