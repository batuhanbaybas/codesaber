package editor

import (
	"fmt"

	"github.com/google/uuid"
)

// Buffer is a text tab's in-memory snapshot. SavedContent is the baseline
// for dirty checks; Content may contain edits that have never reached disk.
// ID changes on close/reopen so a late update cannot resurrect an old tab.
type Buffer struct {
	ID           string `json:"id"`
	Content      string `json:"content"`
	SavedContent string `json:"savedContent"`
	Version      int    `json:"version"`
}

func (s *Service) Buffer(projectID, path string) (Buffer, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.buffers[projectID][path]
	return b, ok
}

// OpenBuffer installs the disk baseline once. A repeated open returns the
// existing snapshot, including unsaved edits.
func (s *Service) OpenBuffer(projectID, path, content string) Buffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.buffers[projectID][path]; ok {
		return b
	}
	if s.buffers[projectID] == nil {
		s.buffers[projectID] = map[string]Buffer{}
	}
	b := Buffer{ID: uuid.NewString(), Content: content, SavedContent: content}
	s.buffers[projectID][path] = b
	return b
}

// UpdateBuffer mirrors a frontend snapshot. Superseded updates are ignored;
// updates for closed/reopened tabs are rejected. No filesystem writes occur.
func (s *Service) UpdateBuffer(projectID, path string, next Buffer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.buffers[projectID][path]
	if !ok || b.ID != next.ID {
		return fmt.Errorf("editor: buffer is closed: %s", path)
	}
	if next.Version <= b.Version {
		return nil
	}
	s.buffers[projectID][path] = next
	return nil
}

// CloseBuffer forgets only the matching tab generation.
func (s *Service) CloseBuffer(projectID, path, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.buffers[projectID][path]; ok && b.ID == id {
		delete(s.buffers[projectID], path)
		if len(s.buffers[projectID]) == 0 {
			delete(s.buffers, projectID)
		}
	}
}

func (s *Service) RemoveProject(projectID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.buffers, projectID)
}
