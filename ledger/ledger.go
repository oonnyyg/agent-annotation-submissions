// Package ledger implements an offline-verifiable authorization session
// ledger that simulates refresh token rotation with replay detection.
//
// This is a local simulation only; it is not a complete OAuth service.
package ledger

import (
	"encoding/json"
	"errors"
)

// Token state machine:
//
//	ACTIVE --rotate--> CONSUMED (a unique successor is created atomically)
//	ACTIVE --expire--> EXPIRED  (clock reaches the absolute deadline)
//	ANY --replay of a CONSUMED token by the bound client--> family REVOKED
//	ANY --explicit revoke--> family REVOKED
//
// Ordinary denials (unknown token, client mismatch, expired token, scope
// widening, duplicate successor id) never mutate the ledger.
type Token struct {
	ID        string   `json:"id"`
	SessionID string   `json:"session_id"`
	ParentID  string   `json:"parent_id,omitempty"`
	Scopes    []string `json:"scopes"`
	ExpiresAt int64    `json:"expires_at"`
	Consumed  bool     `json:"consumed"`
}

type Session struct {
	ID       string   `json:"id"`
	ClientID string   `json:"client_id"`
	Scopes   []string `json:"scopes"`
	Revoked  bool     `json:"revoked"`
}

type Event struct {
	Seq       int64  `json:"seq"`
	Time      int64  `json:"time"`
	Kind      string `json:"kind"` // create, rotate, revoke, replay_revoke
	SessionID string `json:"session_id"`
	TokenID   string `json:"token_id,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type Ledger struct {
	Now      int64              `json:"now"`
	Seq      int64              `json:"seq"`
	Sessions map[string]*Session `json:"sessions"`
	Tokens   map[string]*Token  `json:"tokens"`
	Events   []Event            `json:"events"`
}

func New() *Ledger {
	return &Ledger{
		Sessions: map[string]*Session{},
		Tokens:   map[string]*Token{},
	}
}

// Clone returns a deep copy of the ledger.
func (l *Ledger) Clone() *Ledger {
	data, err := json.Marshal(l)
	if err != nil {
		panic(err)
	}
	var cp Ledger
	if err := json.Unmarshal(data, &cp); err != nil {
		panic(err)
	}
	return &cp
}

// Restore replaces the ledger contents with a previously cloned snapshot.
func (l *Ledger) Restore(snapshot *Ledger) {
	*l = *snapshot
}

var (
	ErrUnknownSession = errors.New("unknown session")
	ErrUnknownToken   = errors.New("unknown token")
	ErrClientMismatch = errors.New("client mismatch")
	ErrTokenConsumed  = errors.New("token already consumed")
	ErrTokenExpired   = errors.New("token expired")
	ErrSessionRevoked = errors.New("session revoked")
	ErrDuplicateID    = errors.New("duplicate identifier")
	ErrScopeWidening  = errors.New("scope widening not allowed")
	ErrSessionExists  = errors.New("session already exists")
	ErrInvalidRequest = errors.New("invalid request")
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

func (l *Ledger) record(kind, sessionID, tokenID, detail string) {
	l.Seq++
	l.Events = append(l.Events, Event{
		Seq:       l.Seq,
		Time:      l.Now,
		Kind:      kind,
		SessionID: sessionID,
		TokenID:   tokenID,
		Detail:    detail,
	})
}
