package questions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFollowUpCarriesOnlyItsThreadHistory(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "questions.json"), BaseSHA: "base", HeadSHA: "head"}
	first, err := store.Add(Anchor{Type: "fragment", GroupID: "group", FragmentID: "fragment"}, "Why?")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	item, err := store.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if item.ThreadID != first.ID || len(item.History) != 0 {
		t.Fatalf("unexpected initial work item: %+v", item)
	}
	if _, err := store.Answer(item.ID, "Because it owns the contract."); err != nil {
		t.Fatal(err)
	}

	other, err := store.Add(Anchor{Type: "group", GroupID: "other"}, "Independent?")
	if err != nil {
		t.Fatal(err)
	}
	followed, err := store.FollowUp(first.ID, "What calls it?")
	if err != nil {
		t.Fatal(err)
	}
	if len(followed.Turns) != 2 || followed.Turns[1].Status != StatusPending {
		t.Fatalf("unexpected follow-up: %+v", followed)
	}

	// The independent thread was queued first and must have no history.
	independent, err := store.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if independent.ThreadID != other.ID || len(independent.History) != 0 {
		t.Fatalf("threads leaked context: %+v", independent)
	}
	if _, err := store.Answer(independent.ID, "Yes."); err != nil {
		t.Fatal(err)
	}

	followUp, err := store.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if followUp.ThreadID != first.ID || followUp.Question != "What calls it?" || len(followUp.History) != 2 {
		t.Fatalf("missing thread history: %+v", followUp)
	}
	if followUp.History[0].Role != "user" || followUp.History[0].Content != "Why?" || followUp.History[1].Role != "assistant" {
		t.Fatalf("unexpected history: %+v", followUp.History)
	}
}

func TestFollowUpRequiresPreviousAnswer(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "questions.json"), BaseSHA: "base", HeadSHA: "head"}
	thread, err := store.Add(Anchor{Type: "group", GroupID: "group"}, "First?")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.FollowUp(thread.ID, "Too soon?"); err == nil {
		t.Fatal("follow-up succeeded before the previous answer")
	}
}

func TestVersionOneQuestionsMigrateToIndependentThreads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	data := `{"version":1,"base_sha":"base","head_sha":"head","questions":[{"id":"Q-old","anchor":{"type":"group","group_id":"g"},"question":"Old?","status":"answered","answer":"Old answer","created_at":"2026-01-01T00:00:00Z"}]}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path, BaseSHA: "base", HeadSHA: "head"}
	threads, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].Turns[0].ID != "Q-old" || threads[0].Turns[0].Answer != "Old answer" {
		t.Fatalf("unexpected migration: %+v", threads)
	}
	if _, err := store.FollowUp(threads[0].ID, "Follow-up?"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 || string(raw[:14]) != "{\n  \"version\":" {
		t.Fatalf("migration was not persisted: %s", raw)
	}
}
