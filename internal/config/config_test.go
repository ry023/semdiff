package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ProjectFile), []byte("review_store:\n  remote: team\n  branch: shared\n"), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if store.Remote != "team" || store.Branch != "shared" {
		t.Fatalf("unexpected store: %+v", store)
	}
}

func TestLoadUsesZeroConfigDefaults(t *testing.T) {
	store, err := Load(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if store.Remote != "origin" || store.Branch != "semdiff/reviews" {
		t.Fatalf("unexpected defaults: %+v", store)
	}
}

func TestLoadExplicitConfigIgnoresProjectConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ProjectFile), []byte("review_store:\n  remote: team\n  branch: shared\n"), 0644); err != nil {
		t.Fatal(err)
	}
	explicit := filepath.Join(t.TempDir(), "private.yaml")
	if err := os.WriteFile(explicit, []byte("review_store:\n  repository: ../artifacts.git\n"), 0644); err != nil {
		t.Fatal(err)
	}
	store, err := Load(dir, explicit)
	if err != nil {
		t.Fatal(err)
	}
	if store.Repository != "../artifacts.git" || store.Remote != "" || store.Branch != "semdiff/reviews" {
		t.Fatalf("unexpected store: %+v", store)
	}
}

func TestLoadExplicitConfigMustExist(t *testing.T) {
	_, err := Load(t.TempDir(), filepath.Join(t.TempDir(), "missing.yaml"))
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}
