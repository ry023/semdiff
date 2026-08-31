package questions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type SessionStatus string

const (
	SessionActive  SessionStatus = "active"
	SessionStopped SessionStatus = "stopped"
)

var ErrNoActiveSession = errors.New("no active answer session")

type Session struct {
	Version   int           `json:"version"`
	ID        string        `json:"session_id"`
	Status    SessionStatus `json:"status"`
	StartedAt time.Time     `json:"started_at"`
	StoppedAt *time.Time    `json:"stopped_at,omitempty"`
}

type SessionStore struct {
	Path string
}

func DefaultSessionPath(groupsPath, baseSHA, headSHA string) string {
	return filepath.Join(filepath.Dir(groupsPath), ".semdiff", "sessions", shortSHA(baseSHA)+"-"+shortSHA(headSHA)+".json")
}

func (s Store) Sessions() SessionStore {
	if s.SessionPath != "" {
		return SessionStore{Path: s.SessionPath}
	}
	questionsDir := filepath.Dir(s.Path)
	sessionsDir := filepath.Join(questionsDir, "sessions")
	if filepath.Base(questionsDir) == "questions" {
		sessionsDir = filepath.Join(filepath.Dir(questionsDir), "sessions")
	}
	return SessionStore{Path: filepath.Join(sessionsDir, filepath.Base(s.Path))}
}

func (s SessionStore) Start() (Session, error) {
	var result Session
	err := s.withLock(context.Background(), func() error {
		current, found, err := s.load()
		if err != nil {
			return err
		}
		if found && current.Status == SessionActive {
			return fmt.Errorf("answer session %s is already active", current.ID)
		}
		now := time.Now().UTC()
		result = Session{Version: 1, ID: fmt.Sprintf("S-%x", now.UnixNano()), Status: SessionActive, StartedAt: now}
		return s.save(result)
	})
	return result, err
}

func (s SessionStore) Get() (Session, bool, error) {
	var result Session
	var found bool
	err := s.withLock(context.Background(), func() error {
		var err error
		result, found, err = s.load()
		return err
	})
	return result, found, err
}

func (s SessionStore) Stop() (Session, error) {
	var result Session
	err := s.withLock(context.Background(), func() error {
		current, found, err := s.load()
		if err != nil {
			return err
		}
		if !found || current.Status != SessionActive {
			return ErrNoActiveSession
		}
		now := time.Now().UTC()
		current.Status = SessionStopped
		current.StoppedAt = &now
		result = current
		return s.save(current)
	})
	return result, err
}

// WithActive keeps the session lock while fn commits related state, so a
// viewer submission cannot cross the boundary of a concurrent Stop.
func (s SessionStore) WithActive(fn func() error) error {
	return s.withLock(context.Background(), func() error {
		current, found, err := s.load()
		if err != nil {
			return err
		}
		if !found || current.Status != SessionActive {
			return ErrNoActiveSession
		}
		return fn()
	})
}

func (s SessionStore) IsActive() (bool, error) {
	session, found, err := s.Get()
	return found && session.Status == SessionActive, err
}

func (s SessionStore) load() (Session, bool, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, false, fmt.Errorf("read answer session: %w", err)
	}
	if session.Version != 1 || session.ID == "" || (session.Status != SessionActive && session.Status != SessionStopped) {
		return Session{}, false, errors.New("invalid answer session file")
	}
	return session, true, nil
}

func (s SessionStore) save(session Session) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.Path), ".session-*.tmp")
	if err != nil {
		return err
	}
	path := temporary.Name()
	defer os.Remove(path)
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(path, s.Path)
}

func (s SessionStore) withLock(ctx context.Context, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0755); err != nil {
		return err
	}
	lockPath := s.Path + ".lock"
	for {
		lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			lock.Close()
			defer os.Remove(lockPath)
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}
