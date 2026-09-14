package backend

import (
	"path/filepath"

	"codesaber/backend/editor"
)

// BufferRead returns the project's open text buffer, reading disk only on
// first open. Like ReadFile, paths may refer to definitions outside the root.
func (a *App) BufferRead(projectID, path string) (editor.Buffer, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return editor.Buffer{}, err
	}
	a.mu.Lock()
	_, err = a.reg.Get(projectID)
	b, ok := a.buf.Buffer(projectID, path)
	a.mu.Unlock()
	if err != nil {
		return editor.Buffer{}, err
	}
	if ok {
		return b, nil
	}
	content, err := a.ReadFile(path)
	if err != nil {
		return editor.Buffer{}, err
	}
	// RemoveProject shares this lock: a read finishing after project removal
	// must not recreate that project's buffers.
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.reg.Get(projectID); err != nil {
		return editor.Buffer{}, err
	}
	return a.buf.OpenBuffer(projectID, path, content), nil
}

// BufferUpdate retains a text tab's latest snapshot in memory. The frontend
// advances Version for edits, saves and explicit reloads.
func (a *App) BufferUpdate(projectID, path string, buffer editor.Buffer) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.reg.Get(projectID); err != nil {
		return err
	}
	return a.buf.UpdateBuffer(projectID, path, buffer)
}

// BufferClose releases a closed tab's snapshot; switching projects does not
// close buffers. The ID prevents a late close from removing a reopened tab.
func (a *App) BufferClose(projectID, path, id string) {
	if abs, err := filepath.Abs(path); err == nil {
		a.buf.CloseBuffer(projectID, abs, id)
	}
}
