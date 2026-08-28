package questions

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestAddWaitAnswer(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "questions.json"), BaseSHA: "base", HeadSHA: "head"}
	waiting := make(chan Question, 1)
	errs := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		question, err := store.Wait(ctx)
		if err != nil {
			errs <- err
			return
		}
		waiting <- question
	}()

	created, err := store.Add(Anchor{Type: "fragment", GroupID: "group", FragmentID: "fragment"}, "Why?")
	if err != nil {
		t.Fatal(err)
	}
	var claimed Question
	select {
	case err := <-errs:
		t.Fatal(err)
	case claimed = <-waiting:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if claimed.ID != created.ID || claimed.Status != StatusClaimed {
		t.Fatalf("unexpected claimed question: %+v", claimed)
	}
	answered, err := store.Answer(created.ID, "Because it owns the contract.")
	if err != nil {
		t.Fatal(err)
	}
	if answered.Status != StatusAnswered || answered.Answer == "" {
		t.Fatalf("unexpected answer: %+v", answered)
	}
	questions, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(questions) != 1 || questions[0].Status != StatusAnswered {
		t.Fatalf("unexpected questions: %+v", questions)
	}
}

func TestWaitReturnsExistingPendingQuestion(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "questions.json"), BaseSHA: "base", HeadSHA: "head"}
	created, err := store.Add(Anchor{Type: "group", GroupID: "group"}, "Explain this group")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	claimed, err := store.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != created.ID {
		t.Fatalf("claimed %q, want %q", claimed.ID, created.ID)
	}
}
