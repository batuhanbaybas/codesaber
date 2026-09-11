package fswatch

import (
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

type Event struct {
	Path string `json:"path"`
	Op   string `json:"op"` // create|write|remove|rename
}

type Watcher struct{ w *fsnotify.Watcher }

// New watches root plus its first-level subdirectories. Recursive watching
// arrives with the git engine in Phase 2.
func New(root string) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := w.Add(root); err != nil {
		w.Close()
		return nil, err
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() {
			_ = w.Add(filepath.Join(root, e.Name()))
		}
	}
	return &Watcher{w: w}, nil
}

func (w *Watcher) Events() <-chan Event {
	out := make(chan Event, 64)
	go func() {
		for ev := range w.w.Events {
			if ev.Op&fsnotify.Write != 0 {
				out <- Event{Path: ev.Name, Op: "write"}
			}
			if ev.Op&fsnotify.Create != 0 {
				out <- Event{Path: ev.Name, Op: "create"}
			}
			if ev.Op&fsnotify.Remove != 0 {
				out <- Event{Path: ev.Name, Op: "remove"}
			}
			if ev.Op&fsnotify.Rename != 0 {
				out <- Event{Path: ev.Name, Op: "rename"}
			}
		}
		close(out)
	}()
	return out
}

func (w *Watcher) Close() error { return w.w.Close() }
