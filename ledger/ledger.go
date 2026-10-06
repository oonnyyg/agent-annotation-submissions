// Package ledger implements an offline-verifiable authorization session
// ledger that simulates refresh-token rotation with replay detection.
// It is a local simulation only, not a real OAuth service.
package ledger

import (
	"errors"
	"fmt"
	"sort"
)

// TokenStatus describes the lifecycle state of a single token.
type TokenStatus string

const (
	StatusActive   TokenStatus = "active"   // usable for rotation
	StatusConsumed TokenStatus = "consumed" // rotated away; replay of it kills the family
	StatusExpired  TokenStatus = "expired"  // past absolute expiry
	StatusRevoked  TokenStatus = "revoked"  // family revoked (replay or explicit revoke)
)

// SessionState is the externally visible state of a session.
type SessionState string

const (
	SessionActive  SessionState = "active"
	SessionExpired SessionState = "expired"
	SessionRevoked SessionState = "revoked"
)

// Token is one generation of a refresh token family.
type Token struct {
	ID        string      `json:"id"`
	Session   string      `json:"session"`
	Client    string      `json:"client"`
	Scopes    []string    `json:"scopes"`
	Parent    string      `json:"parent,omitempty"`
	Child     string      `json:"child,omitempty"`
	ExpiresAt int64       `json:"expires_at"`
	CreatedAt int64       `json:"created_at"`
	Status    TokenStatus `json:"status"`
}

// Revocation records why and how a token family was revoked.
type Revocation struct {
	Session string `json:"session"`
	Reason  string `json:"reason"` // "replay" or "explicit"
	Trigger string `json:"trigger"`
	Token   string `json:"token"`
	Client  string `json:"client"`
	At      int64  `json:"at"`
}

// Event is one entry of a session's history.
type Event struct {
	At     int64  `json:"at"`
	Kind   string `json:"kind"` // create, rotate, replay-revoke, revoke, expire
	Detail string `json:"detail"`
}

// Session binds a client and a token family.
type Session struct {
	ID      string            `json:"id"`
	Client  string            `json:"client"`
	Tokens  map[string]*Token `json:"tokens"`
	Head    string            `json:"head"`
	Revoked *Revocation       `json:"revoked,omitempty"`
	History []Event           `json:"history"`
}

// Ledger is the in-memory state; persistence lives in store.go.
type Ledger struct {
	Now      int64               `json:"now"`
	Sessions map[string]*Session `json:"sessions"`
	byToken  map[string]*Token
}

// NewLedger returns an empty ledger at logical time zero.
func NewLedger() *Ledger {
	return &Ledger{Sessions: map[string]*Session{}, byToken: map[string]*Token{}}
}

func (l *Ledger) reindex() {
	l.byToken = map[string]*Token{}
	for _, s := range l.Sessions {
		for _, t := range s.Tokens {
			l.byToken[t.ID] = t
		}
	}
}

// Errors returned as ordinary rejections; none of them mutate the ledger.
var (
	ErrSessionExists  = errors.New("session already exists")
	ErrSessionUnknown = errors.New("unknown session")
	ErrTokenUnknown   = errors.New("unknown token")
	ErrClientMismatch = errors.New("client mismatch")
	ErrTokenNotActive = errors.New("token not active")
	ErrTokenIDTaken   = errors.New("new token id already in use")
	ErrScopeExpansion = errors.New("scopes may only be kept or narrowed")
	ErrClockBackwards = errors.New("clock may not move backwards")
	ErrInvalidExpiry  = errors.New("expiry must be in the future")
)

func subsetOf(child, parent []string) bool {
	set := map[string]bool{}
	for _, s := range parent {
		set[s] = true
	}
	for _, s := range child {
		if !set[s] {
			return false
		}
	}
	return true
}

// Advance moves the explicit monotonic clock forward.
func (l *Ledger) Advance(now int64) error {
	if now < l.Now {
		return ErrClockBackwards
	}
	l.Now = now
	for _, s := range l.Sessions {
		l.expireIfDue(s)
	}
	return nil
}

func (l *Ledger) expireIfDue(s *Session) {
	if s.Revoked != nil {
		return
	}
	head := s.Tokens[s.Head]
	if head != nil && head.Status == StatusActive && l.Now >= head.ExpiresAt {
		head.Status = StatusExpired
		s.History = append(s.History, Event{At: l.Now, Kind: "expire",
			Detail: fmt.Sprintf("token %s expired at %d", head.ID, head.ExpiresAt)})
	}
}

// Create opens a new session with its initial token.
func (l *Ledger) Create(session, client string, scopes []string, tokenID string, expiresAt int64) error {
	if _, ok := l.Sessions[session]; ok {
		return ErrSessionExists
	}
	if _, ok := l.byToken[tokenID]; ok {
		return ErrTokenIDTaken
	}
	if expiresAt <= l.Now {
		return ErrInvalidExpiry
	}
	tok := &Token{ID: tokenID, Session: session, Client: client, Scopes: scopes,
		ExpiresAt: expiresAt, CreatedAt: l.Now, Status: StatusActive}
	s := &Session{ID: session, Client: client, Tokens: map[string]*Token{tokenID: tok},
		Head: tokenID}
	s.History = append(s.History, Event{At: l.Now, Kind: "create",
		Detail: fmt.Sprintf("session created for client %s with token %s", client, tokenID)})
	l.Sessions[session] = s
	l.byToken[tokenID] = tok
	return nil
}

// RotateResult reports a rotation outcome.
type RotateResult struct {
	NewToken  string   `json:"new_token,omitempty"`
	ExpiresAt int64    `json:"expires_at,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	Replay    bool     `json:"replay,omitempty"`
	Revoked   bool     `json:"revoked,omitempty"`
}

// Rotate consumes an active token and installs its unique successor.
// Replaying an already-consumed token from its bound client revokes the
// whole family and reports Replay=true. Ordinary rejections mutate nothing.
func (l *Ledger) Rotate(tokenID, client, newID string, scopes []string) (*RotateResult, error) {
	tok, ok := l.byToken[tokenID]
	if !ok {
		return nil, ErrTokenUnknown
	}
	s := l.Sessions[tok.Session]
	if tok.Client != client {
		return nil, ErrClientMismatch
	}
	if tok.Status == StatusConsumed {
		l.revokeFamily(s, "replay", tokenID, client)
		return &RotateResult{Replay: true, Revoked: true}, nil
	}
	if tok.Status != StatusActive {
		return nil, ErrTokenNotActive
	}
	if l.Now >= tok.ExpiresAt {
		l.expireIfDue(s)
		return nil, ErrTokenNotActive
	}
	if _, taken := l.byToken[newID]; taken {
		return nil, ErrTokenIDTaken
	}
	if scopes == nil {
		scopes = tok.Scopes
	}
	if !subsetOf(scopes, tok.Scopes) {
		return nil, ErrScopeExpansion
	}
	next := &Token{ID: newID, Session: s.ID, Client: client, Scopes: scopes,
		Parent: tok.ID, ExpiresAt: tok.ExpiresAt, CreatedAt: l.Now, Status: StatusActive}
	tok.Status = StatusConsumed
	tok.Child = newID
	s.Tokens[newID] = next
	s.Head = newID
	l.byToken[newID] = next
	s.History = append(s.History, Event{At: l.Now, Kind: "rotate",
		Detail: fmt.Sprintf("token %s rotated to %s", tok.ID, newID)})
	return &RotateResult{NewToken: newID, ExpiresAt: next.ExpiresAt, Scopes: scopes}, nil
}

// Revoke explicitly revokes the whole family of the given token.
func (l *Ledger) Revoke(tokenID, client string) error {
	tok, ok := l.byToken[tokenID]
	if !ok {
		return ErrTokenUnknown
	}
	if tok.Client != client {
		return ErrClientMismatch
	}
	s := l.Sessions[tok.Session]
	if s.Revoked == nil {
		l.revokeFamily(s, "explicit", tokenID, client)
	}
	return nil
}

func (l *Ledger) revokeFamily(s *Session, reason, triggerToken, triggerClient string) {
	for _, t := range s.Tokens {
		if t.Status == StatusActive || t.Status == StatusConsumed {
			t.Status = StatusRevoked
		}
	}
	s.Revoked = &Revocation{Session: s.ID, Reason: reason, Trigger: triggerToken,
		Token: triggerToken, Client: triggerClient, At: l.Now}
	kind := "revoke"
	if reason == "replay" {
		kind = "replay-revoke"
	}
	s.History = append(s.History, Event{At: l.Now, Kind: kind,
		Detail: fmt.Sprintf("family revoked (%s) by token %s from client %s", reason, triggerToken, triggerClient)})
}

// SessionView is the query result for a session.
type SessionView struct {
	ID         string       `json:"id"`
	Client     string       `json:"client"`
	State      SessionState `json:"state"`
	Head       string       `json:"head"`
	Revocation *Revocation  `json:"revocation,omitempty"`
	History    []Event      `json:"history"`
	Tokens     []*Token     `json:"tokens"`
}

// Query returns the state and history of a session.
func (l *Ledger) Query(sessionID string) (*SessionView, error) {
	s, ok := l.Sessions[sessionID]
	if !ok {
		return nil, ErrSessionUnknown
	}
	l.expireIfDue(s)
	state := SessionActive
	switch {
	case s.Revoked != nil:
		state = SessionRevoked
	case s.Tokens[s.Head].Status == StatusExpired:
		state = SessionExpired
	}
	tokens := make([]*Token, 0, len(s.Tokens))
	for _, t := range s.Tokens {
		tokens = append(tokens, t)
	}
	sort.Slice(tokens, func(i, j int) bool { return tokens[i].CreatedAt < tokens[j].CreatedAt })
	return &SessionView{ID: s.ID, Client: s.Client, State: state, Head: s.Head,
		Revocation: s.Revoked, History: s.History, Tokens: tokens}, nil
}
