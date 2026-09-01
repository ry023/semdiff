package questions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusClaimed  Status = "claimed"
	StatusAnswered Status = "answered"
)

type Anchor struct {
	Type       string `json:"type"`
	GroupID    string `json:"group_id"`
	FragmentID string `json:"fragment_id,omitempty"`
	StepID     string `json:"step_id,omitempty"`
}
type Turn struct {
	ID         string     `json:"id"`
	Question   string     `json:"question"`
	Status     Status     `json:"status"`
	Answer     string     `json:"answer,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ClaimedAt  *time.Time `json:"claimed_at,omitempty"`
	AnsweredAt *time.Time `json:"answered_at,omitempty"`
}
type Thread struct {
	ID        string    `json:"id"`
	BaseSHA   string    `json:"base_sha"`
	HeadSHA   string    `json:"head_sha"`
	Anchor    Anchor    `json:"anchor"`
	CreatedAt time.Time `json:"created_at"`
	Turns     []Turn    `json:"turns"`
}
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// WorkItem contains one pending turn. History includes answered turns from
// this thread only, so independent questions never share conversational state.
type WorkItem struct {
	ID       string    `json:"id"`
	ThreadID string    `json:"thread_id"`
	BaseSHA  string    `json:"base_sha"`
	HeadSHA  string    `json:"head_sha"`
	Anchor   Anchor    `json:"anchor"`
	History  []Message `json:"history"`
	Question string    `json:"question"`
}
type WaitEvent struct {
	Event     string    `json:"event"`
	SessionID string    `json:"session_id"`
	Question  *WorkItem `json:"question,omitempty"`
}
type File struct {
	Version int      `json:"version"`
	BaseSHA string   `json:"base_sha"`
	HeadSHA string   `json:"head_sha"`
	Threads []Thread `json:"threads"`
}
type Store struct {
	Path        string
	SessionPath string
	BaseSHA     string
	HeadSHA     string
}

func DefaultPath(groupsPath, baseSHA, headSHA string) string {
	return filepath.Join(filepath.Dir(groupsPath), ".semdiff", "questions", shortSHA(baseSHA)+"-"+shortSHA(headSHA)+".json")
}

func (s Store) List() ([]Thread, error) {
	var result []Thread
	err := s.withLock(context.Background(), func() error {
		file, _, err := s.load()
		if err == nil {
			result = append(result, file.Threads...)
		}
		return err
	})
	return result, err
}

func (s Store) Add(anchor Anchor, body string) (Thread, error) {
	if err := validateQuestion(anchor, body); err != nil {
		return Thread{}, err
	}
	now := time.Now().UTC()
	thread := Thread{ID: fmt.Sprintf("T-%x", now.UnixNano()), BaseSHA: s.BaseSHA, HeadSHA: s.HeadSHA, Anchor: anchor, CreatedAt: now, Turns: []Turn{{ID: fmt.Sprintf("Q-%x", now.UnixNano()), Question: strings.TrimSpace(body), Status: StatusPending, CreatedAt: now}}}
	err := s.update(context.Background(), func(file *File) error { file.Threads = append(file.Threads, thread); return nil })
	return thread, err
}

func (s Store) FollowUp(threadID, body string) (Thread, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Thread{}, errors.New("question is required")
	}
	var result Thread
	err := s.update(context.Background(), func(file *File) error {
		for i := range file.Threads {
			thread := &file.Threads[i]
			if thread.ID != threadID {
				continue
			}
			if len(thread.Turns) == 0 || thread.Turns[len(thread.Turns)-1].Status != StatusAnswered {
				return fmt.Errorf("thread %s is still waiting for an answer", threadID)
			}
			now := time.Now().UTC()
			thread.Turns = append(thread.Turns, Turn{ID: fmt.Sprintf("Q-%x", now.UnixNano()), Question: body, Status: StatusPending, CreatedAt: now})
			result = *thread
			return nil
		}
		return fmt.Errorf("thread %s not found", threadID)
	})
	return result, err
}

func (s Store) Wait(ctx context.Context) (WorkItem, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		item, err := s.claim(ctx)
		if err != nil {
			return WorkItem{}, err
		}
		if item.ID != "" {
			return item, nil
		}
		select {
		case <-ctx.Done():
			return WorkItem{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s Store) WaitSession(ctx context.Context, sessionID string) (WaitEvent, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		session, found, err := s.Sessions().Get()
		if err != nil {
			return WaitEvent{}, err
		}
		if !found || session.ID != sessionID {
			return WaitEvent{}, fmt.Errorf("answer session %s is not current", sessionID)
		}
		if session.Status == SessionStopped {
			return WaitEvent{Event: "stopped", SessionID: sessionID}, nil
		}
		item, err := s.claim(ctx)
		if err != nil {
			return WaitEvent{}, err
		}
		if item.ID != "" {
			return WaitEvent{Event: "question", SessionID: sessionID, Question: &item}, nil
		}
		select {
		case <-ctx.Done():
			return WaitEvent{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s Store) claim(ctx context.Context) (WorkItem, error) {
	var item WorkItem
	err := s.update(ctx, func(file *File) error {
		threadIndex, turnIndex := -1, -1
		var earliest time.Time
		for ti := range file.Threads {
			for qi := range file.Threads[ti].Turns {
				turn := file.Threads[ti].Turns[qi]
				if turn.Status == StatusPending && (turnIndex < 0 || turn.CreatedAt.Before(earliest)) {
					threadIndex, turnIndex, earliest = ti, qi, turn.CreatedAt
				}
			}
		}
		if turnIndex >= 0 {
			thread := &file.Threads[threadIndex]
			now := time.Now().UTC()
			thread.Turns[turnIndex].Status = StatusClaimed
			thread.Turns[turnIndex].ClaimedAt = &now
			item = workItem(*thread, turnIndex)
		}
		return nil
	})
	return item, err
}

func (s Store) Answer(id, answer string) (Thread, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return Thread{}, errors.New("answer is required")
	}
	var result Thread
	err := s.update(context.Background(), func(file *File) error {
		for ti := range file.Threads {
			thread := &file.Threads[ti]
			for qi := range thread.Turns {
				turn := &thread.Turns[qi]
				if turn.ID != id {
					continue
				}
				if turn.Status == StatusAnswered {
					return fmt.Errorf("question %s is already answered", id)
				}
				now := time.Now().UTC()
				turn.Status = StatusAnswered
				turn.Answer = answer
				turn.AnsweredAt = &now
				result = *thread
				return nil
			}
		}
		return fmt.Errorf("question %s not found", id)
	})
	return result, err
}

func workItem(thread Thread, turnIndex int) WorkItem {
	history := make([]Message, 0, turnIndex*2)
	for _, turn := range thread.Turns[:turnIndex] {
		if turn.Status == StatusAnswered {
			history = append(history, Message{Role: "user", Content: turn.Question}, Message{Role: "assistant", Content: turn.Answer})
		}
	}
	turn := thread.Turns[turnIndex]
	return WorkItem{ID: turn.ID, ThreadID: thread.ID, BaseSHA: thread.BaseSHA, HeadSHA: thread.HeadSHA, Anchor: thread.Anchor, History: history, Question: turn.Question}
}

func validateQuestion(anchor Anchor, body string) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("question is required")
	}
	if anchor.Type != "group" && anchor.Type != "fragment" && anchor.Type != "step" {
		return errors.New("anchor type must be group, step, or fragment")
	}
	if anchor.GroupID == "" || (anchor.Type == "fragment" && anchor.FragmentID == "") || (anchor.Type == "step" && anchor.StepID == "") {
		return errors.New("anchor is incomplete")
	}
	return nil
}

func (s Store) update(ctx context.Context, mutate func(*File) error) error {
	return s.withLock(ctx, func() error {
		file, _, err := s.load()
		if err != nil {
			return err
		}
		if err := mutate(&file); err != nil {
			return err
		}
		return s.save(file)
	})
}

func (s Store) load() (File, bool, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return File{Version: 2, BaseSHA: s.BaseSHA, HeadSHA: s.HeadSHA}, false, nil
	}
	if err != nil {
		return File{}, false, err
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return File{}, false, fmt.Errorf("read questions: %w", err)
	}
	if header.Version == 1 {
		file, err := migrateV1(data)
		if err != nil {
			return File{}, false, err
		}
		if file.BaseSHA != s.BaseSHA || file.HeadSHA != s.HeadSHA {
			return File{}, false, errors.New("questions file does not match the groups Git range")
		}
		return file, true, nil
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return File{}, false, fmt.Errorf("read questions: %w", err)
	}
	if file.Version != 2 || file.BaseSHA != s.BaseSHA || file.HeadSHA != s.HeadSHA {
		return File{}, false, errors.New("questions file does not match the groups Git range")
	}
	return file, false, nil
}

func migrateV1(data []byte) (File, error) {
	type oldQuestion struct {
		ID         string     `json:"id"`
		Anchor     Anchor     `json:"anchor"`
		Question   string     `json:"question"`
		Status     Status     `json:"status"`
		Answer     string     `json:"answer,omitempty"`
		CreatedAt  time.Time  `json:"created_at"`
		ClaimedAt  *time.Time `json:"claimed_at,omitempty"`
		AnsweredAt *time.Time `json:"answered_at,omitempty"`
	}
	var old struct {
		Version   int           `json:"version"`
		BaseSHA   string        `json:"base_sha"`
		HeadSHA   string        `json:"head_sha"`
		Questions []oldQuestion `json:"questions"`
	}
	if err := json.Unmarshal(data, &old); err != nil {
		return File{}, fmt.Errorf("migrate questions: %w", err)
	}
	file := File{Version: 2, BaseSHA: old.BaseSHA, HeadSHA: old.HeadSHA}
	for _, q := range old.Questions {
		file.Threads = append(file.Threads, Thread{ID: "T-" + strings.TrimPrefix(q.ID, "Q-"), BaseSHA: old.BaseSHA, HeadSHA: old.HeadSHA, Anchor: q.Anchor, CreatedAt: q.CreatedAt, Turns: []Turn{{ID: q.ID, Question: q.Question, Status: q.Status, Answer: q.Answer, CreatedAt: q.CreatedAt, ClaimedAt: q.ClaimedAt, AnsweredAt: q.AnsweredAt}}})
	}
	return file, nil
}

func (s Store) save(file File) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.Path), ".questions-*.tmp")
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

func (s Store) withLock(ctx context.Context, fn func() error) error {
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

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
