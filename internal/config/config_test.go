package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMergesProjectAndLocalConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ProjectFile), []byte("review_store:\n  remote: team\n  branch: shared\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".semdiff"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, LocalFile), []byte("review_store:\n  repository: ../artifacts.git\n"), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if store.Repository != "../artifacts.git" || store.Remote != "" || store.Branch != "shared" {
		t.Fatalf("unexpected store: %+v", store)
	}
}

func TestLoadUsesZeroConfigDefaults(t *testing.T) {
	store, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if store.Remote != "origin" || store.Branch != "semdiff/reviews" {
		t.Fatalf("unexpected defaults: %+v", store)
	}
}
