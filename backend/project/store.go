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
	if base, err := os.UserConfigDir(); err == nil {
		return filepath.Join(base, "codesaber", "recents.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "codesaber", "recents.json")
	}
	return filepath.Join(home, ".config", "codesaber", "recents.json")
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
	for i, r := range recents {
		if r.Root == root {
			recents[i].ID = id
			recents[i].Branch = branch
			recents[i].LastUsed = time.Now()
			sort.Slice(recents, func(x, y int) bool { return recents[x].LastUsed.After(recents[y].LastUsed) })
			s.save(recents)
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

// Forget drops the entry identified by root.
func (s *Store) Forget(root string) {
	recents := s.load()
	out := recents[:0]
	for _, r := range recents {
		if r.Root != root {
			out = append(out, r)
		}
	}
	s.save(out)
}

func (s *Store) List() []Recent { return s.load() }
