package editor

import (
	"os"
	"path/filepath"
	"sync"
)

// Service tracks last-saved buffer content per path. The webview holds the live
// content; backend truth reconciles on save and against external changes.
type Service struct {
	mu    sync.Mutex
	saved map[string]string
}

func New() *Service { return &Service{saved: map[string]string{}} }

func (s *Service) Track(path, savedContent string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved[path] = savedContent
}

func (s *Service) Dirty(path, current string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saved[path] != current
}

// Save writes content atomically via a hidden temp file + rename, then records
// it as the last-saved content for path.
func (s *Service) Save(path, content string) error {
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".aide-tmp")
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	s.Track(path, content)
	return nil
}
