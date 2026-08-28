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
}

type Question struct {
	ID         string     `json:"id"`
	BaseSHA    string     `json:"base_sha"`
	HeadSHA    string     `json:"head_sha"`
	Anchor     Anchor     `json:"anchor"`
	Question   string     `json:"question"`
	Status     Status     `json:"status"`
	Answer     string     `json:"answer,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ClaimedAt  *time.Time `json:"claimed_at,omitempty"`
	AnsweredAt *time.Time `json:"answered_at,omitempty"`
}

type File struct {
	Version   int        `json:"version"`
	BaseSHA   string     `json:"base_sha"`
	HeadSHA   string     `json:"head_sha"`
	Questions []Question `json:"questions"`
}

type Store struct {
	Path    string
	BaseSHA string
	HeadSHA string
}

func DefaultPath(groupsPath, baseSHA, headSHA string) string {
	name := shortSHA(baseSHA) + "-" + shortSHA(headSHA) + ".json"
	return filepath.Join(filepath.Dir(groupsPath), ".semdiff", "questions", name)
}

func (s Store) List() ([]Question, error) {
	var result []Question
	err := s.withLock(context.Background(), func() error {
		file, err := s.load()
		if err != nil {
			return err
		}
		result = append(result, file.Questions...)
		return nil
	})
	return result, err
}

func (s Store) Add(anchor Anchor, body string) (Question, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Question{}, errors.New("question is required")
	}
	if anchor.Type != "group" && anchor.Type != "fragment" {
		return Question{}, errors.New("anchor type must be group or fragment")
	}
	if anchor.GroupID == "" || (anchor.Type == "fragment" && anchor.FragmentID == "") {
		return Question{}, errors.New("anchor is incomplete")
	}
	now := time.Now().UTC()
	question := Question{
		ID: fmt.Sprintf("Q-%x", now.UnixNano()), BaseSHA: s.BaseSHA, HeadSHA: s.HeadSHA,
		Anchor: anchor, Question: body, Status: StatusPending, CreatedAt: now,
	}
	err := s.update(context.Background(), func(file *File) error {
		file.Questions = append(file.Questions, question)
		return nil
	})
	return question, err
}

func (s Store) Wait(ctx context.Context) (Question, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		var claimed Question
		err := s.update(ctx, func(file *File) error {
			for i := range file.Questions {
				if file.Questions[i].Status != StatusPending {
					continue
				}
				now := time.Now().UTC()
				file.Questions[i].Status = StatusClaimed
				file.Questions[i].ClaimedAt = &now
				claimed = file.Questions[i]
				return nil
			}
			return nil
		})
		if err != nil {
			return Question{}, err
		}
		if claimed.ID != "" {
			return claimed, nil
		}
		select {
		case <-ctx.Done():
			return Question{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s Store) Answer(id, answer string) (Question, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return Question{}, errors.New("answer is required")
	}
	var result Question
	err := s.update(context.Background(), func(file *File) error {
		for i := range file.Questions {
			if file.Questions[i].ID != id {
				continue
			}
			if file.Questions[i].Status == StatusAnswered {
				return fmt.Errorf("question %s is already answered", id)
			}
			now := time.Now().UTC()
			file.Questions[i].Status = StatusAnswered
			file.Questions[i].Answer = answer
			file.Questions[i].AnsweredAt = &now
			result = file.Questions[i]
			return nil
		}
		return fmt.Errorf("question %s not found", id)
	})
	return result, err
}

func (s Store) update(ctx context.Context, mutate func(*File) error) error {
	return s.withLock(ctx, func() error {
		file, err := s.load()
		if err != nil {
			return err
		}
		if err := mutate(&file); err != nil {
			return err
		}
		return s.save(file)
	})
}

func (s Store) load() (File, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return File{Version: 1, BaseSHA: s.BaseSHA, HeadSHA: s.HeadSHA}, nil
	}
	if err != nil {
		return File{}, err
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return File{}, fmt.Errorf("read questions: %w", err)
	}
	if file.Version != 1 || file.BaseSHA != s.BaseSHA || file.HeadSHA != s.HeadSHA {
		return File{}, errors.New("questions file does not match the groups Git range")
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
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, s.Path)
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
