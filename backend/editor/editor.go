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

// Count returns the number of tracked buffer paths.
func (s *Service) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.saved)
}

func (s *Service) Dirty(path, current string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.saved[path]
	if !ok {
		return false
	}
	return s.saved[path] != current
}

// Save writes content atomically via a hidden temp file + rename, then records
// it as the last-saved content for path. The temp file is removed on any
// error path.
func (s *Service) Save(path, content string) error {
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".codesaber-tmp")
	defer func() {
		if _, statErr := os.Stat(tmp); statErr == nil {
			os.Remove(tmp)
		}
	}()
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	s.Track(path, content)
	return nil
}
