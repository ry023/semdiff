package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	ProjectFile = "semdiff.yaml"
	LocalFile   = ".semdiff/config.local.yaml"
)

type ReviewStore struct {
	Remote     string `yaml:"remote"`
	Repository string `yaml:"repository"`
	Branch     string `yaml:"branch"`
}

func (store ReviewStore) Endpoint() string {
	if store.Repository != "" {
		return store.Repository
	}
	return store.Remote
}

type File struct {
	ReviewStore ReviewStore `yaml:"review_store"`
}

// Load merges the optional repository and local configuration files. Local
// settings intentionally win, so a repository can share a branch convention
// without forcing a user's private artifact repository.
func Load(dir string) (ReviewStore, error) {
	var result ReviewStore
	for _, name := range []string{ProjectFile, LocalFile} {
		path := filepath.Join(dir, name)
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return ReviewStore{}, err
		}
		var f File
		decoder := yaml.NewDecoder(strings.NewReader(string(b)))
		decoder.KnownFields(true)
		if err := decoder.Decode(&f); err != nil {
			return ReviewStore{}, fmt.Errorf("decode %s: %w", path, err)
		}
		if f.ReviewStore.Remote != "" {
			result.Remote, result.Repository = f.ReviewStore.Remote, ""
		}
		if f.ReviewStore.Repository != "" {
			result.Repository, result.Remote = f.ReviewStore.Repository, ""
		}
		if f.ReviewStore.Branch != "" {
			result.Branch = f.ReviewStore.Branch
		}
	}
	return Normalize(result)
}

func Normalize(store ReviewStore) (ReviewStore, error) {
	if store.Repository != "" && store.Remote != "" {
		return ReviewStore{}, fmt.Errorf("review_store.remote and review_store.repository cannot both be set")
	}
	if store.Repository == "" && store.Remote == "" {
		store.Remote = "origin"
	}
	if store.Branch == "" {
		store.Branch = "semdiff/reviews"
	}
	return store, nil
}

func Override(store ReviewStore, remote, repository, branch string) (ReviewStore, error) {
	if remote != "" {
		store.Remote, store.Repository = remote, ""
	}
	if repository != "" {
		store.Repository, store.Remote = repository, ""
	}
	if branch != "" {
		store.Branch = branch
	}
	return Normalize(store)
}
