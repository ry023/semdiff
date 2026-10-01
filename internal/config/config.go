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

// Load reads an explicitly selected configuration file, or semdiff.yaml from
// the repository root when path is empty. An explicit path is required to
// exist and replaces, rather than merges with, the repository configuration.
func Load(repositoryRoot, path string) (ReviewStore, error) {
	required := path != ""
	if !required {
		path = filepath.Join(repositoryRoot, ProjectFile)
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) && !required {
		return Normalize(ReviewStore{})
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
	return Normalize(f.ReviewStore)
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
