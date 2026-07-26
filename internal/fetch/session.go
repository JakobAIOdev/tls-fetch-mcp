package fetch

import (
	"fmt"
	"sort"
	"time"

	tlsclient "github.com/bogdanfinn/tls-client"
)

type sessionState struct {
	jar       tlsclient.CookieJar
	createdAt time.Time
	lastUsed  time.Time
	expiresAt time.Time
}

type SessionInfoInput struct {
	SessionID string `json:"session_id" jsonschema:"Cookie-session identifier to inspect"`
}

type SessionInfoOutput struct {
	SessionID     string   `json:"session_id"`
	Exists        bool     `json:"exists"`
	CreatedAt     string   `json:"created_at,omitempty"`
	LastUsedAt    string   `json:"last_used_at,omitempty"`
	ExpiresAt     string   `json:"expires_at,omitempty"`
	CookiesStored int      `json:"cookies_stored,omitempty"`
	CookieNames   []string `json:"cookie_names,omitempty"`
}

func (f *Fetcher) ClearSession(sessionID string) (bool, error) {
	if err := validateSessionID(sessionID); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.purgeExpiredSessionsLocked(f.now())
	if _, exists := f.sessions[sessionID]; !exists {
		return false, nil
	}
	delete(f.sessions, sessionID)
	return true, nil
}

func (f *Fetcher) SessionInfo(sessionID string) (SessionInfoOutput, error) {
	if err := validateSessionID(sessionID); err != nil {
		return SessionInfoOutput{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.purgeExpiredSessionsLocked(f.now())
	state, exists := f.sessions[sessionID]
	if !exists {
		return SessionInfoOutput{SessionID: sessionID, Exists: false}, nil
	}
	count, names := cookieSummary(state.jar)
	return SessionInfoOutput{
		SessionID:     sessionID,
		Exists:        true,
		CreatedAt:     state.createdAt.UTC().Format(time.RFC3339),
		LastUsedAt:    state.lastUsed.UTC().Format(time.RFC3339),
		ExpiresAt:     state.expiresAt.UTC().Format(time.RFC3339),
		CookiesStored: count,
		CookieNames:   names,
	}, nil
}

func (f *Fetcher) session(sessionID string) (tlsclient.CookieJar, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, err
	}
	now := f.now()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.purgeExpiredSessionsLocked(now)
	if state, exists := f.sessions[sessionID]; exists {
		state.lastUsed = now
		state.expiresAt = now.Add(time.Duration(f.config.SessionTTL) * time.Second)
		return state.jar, nil
	}
	if len(f.sessions) >= f.config.MaxSessions {
		return nil, fmt.Errorf("cookie-session limit reached (%d); clear an existing session with tls_session_clear", f.config.MaxSessions)
	}
	jar := tlsclient.NewCookieJar()
	f.sessions[sessionID] = &sessionState{
		jar:       jar,
		createdAt: now,
		lastUsed:  now,
		expiresAt: now.Add(time.Duration(f.config.SessionTTL) * time.Second),
	}
	return jar, nil
}

func (f *Fetcher) touchSession(sessionID string) {
	if sessionID == "" {
		return
	}
	now := f.now()
	f.mu.Lock()
	defer f.mu.Unlock()
	if state, exists := f.sessions[sessionID]; exists {
		state.lastUsed = now
		state.expiresAt = now.Add(time.Duration(f.config.SessionTTL) * time.Second)
	}
}

func (f *Fetcher) purgeExpiredSessionsLocked(now time.Time) {
	for id, state := range f.sessions {
		if !state.expiresAt.After(now) {
			delete(f.sessions, id)
		}
	}
}

func cookieSummary(jar tlsclient.CookieJar) (int, []string) {
	if jar == nil {
		return 0, nil
	}
	names := make(map[string]struct{})
	count := 0
	for _, cookies := range jar.GetAllCookies() {
		for _, cookie := range cookies {
			count++
			if cookie != nil && cookie.Name != "" {
				names[cookie.Name] = struct{}{}
			}
		}
	}
	sortedNames := make([]string, 0, len(names))
	for name := range names {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)
	return count, sortedNames
}
