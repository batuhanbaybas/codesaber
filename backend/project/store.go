package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Recent struct {
	ID       string    `json:"id"`
	Root     string    `json:"root"`
	Branch   string    `json:"branch"`
	LastUsed time.Time `json:"lastUsed"`
}

type Store struct{ path string }

func NewStore(path string) *Store { return &Store{path: path} }

func DefaultStorePath() string {
	base, _ := os.UserConfigDir()
	return filepath.Join(base, "aide", "recents.json")
}

func (s *Store) load() []Recent {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil
	}
	var recents []Recent
	if err := json.Unmarshal(data, &recents); err != nil {
		return nil
	}
	return recents
}

func (s *Store) save(recents []Recent) {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(recents, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.path, data, 0o644)
}

func (s *Store) Remember(id, root, branch string) {
	recents := s.load()
	for _, r := range recents {
		if r.Root == root {
			return
		}
	}
	recents = append(recents, Recent{
		ID:       id,
		Root:     root,
		Branch:   branch,
		LastUsed: time.Now(),
	})
	sort.Slice(recents, func(i, j int) bool { return recents[i].LastUsed.After(recents[j].LastUsed) })
	s.save(recents)
}

func (s *Store) Forget(id string) {
	recents := s.load()
	out := recents[:0]
	for _, r := range recents {
		if r.ID != id {
			out = append(out, r)
		}
	}
	s.save(out)
}

func (s *Store) List() []Recent { return s.load() }
