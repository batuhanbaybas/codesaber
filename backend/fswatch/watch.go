package fswatch

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type Event struct {
	Path string `json:"path"`
	Op   string `json:"op"` // create|write|remove|rename
}

type Watcher struct {
	w     *fsnotify.Watcher
	events chan Event
	done   chan struct{}

	eventsClosed sync.Once
	closeOnce    sync.Once
}

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
	entries, err := os.ReadDir(root)
	if err != nil {
		w.Close()
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			if err := w.Add(filepath.Join(root, e.Name())); err != nil {
				w.Close()
				return nil, err
			}
		}
	}
	watcher := &Watcher{
		w:     w,
		events:   make(chan Event, 64),
		done:     make(chan struct{}),
	}
	go watcher.forward()
	return watcher, nil
}

func (w *Watcher) closeEvents() { w.eventsClosed.Do(func() { close(w.events) }) }

func (w *Watcher) forward() {
	for {
		select {
		case ev, ok := <-w.w.Events:
			if !ok {
				w.closeEvents()
				return
			}
			var op string
			switch {
			case ev.Op&fsnotify.Create != 0:
				op = "create"
			case ev.Op&fsnotify.Write != 0:
				op = "write"
			case ev.Op&fsnotify.Remove != 0:
				op = "remove"
			case ev.Op&fsnotify.Rename != 0:
				op = "rename"
			default:
				continue
			}
			select {
			case w.events <- Event{Path: ev.Name, Op: op}:
			case <-w.done:
				w.closeEvents()
				return
			}
		case <-w.done:
			w.closeEvents()
			return
		}
	}
}

// Events returns the event stream. It is idempotent: every call returns the
// same channel, which is closed after Close.
func (w *Watcher) Events() <-chan Event {
	return w.events
}

// internalDone exposes the done channel for tests only.
func (w *Watcher) internalDone() <-chan struct{} { return w.done }

// Close stops forwarding and releases the underlying watcher. After Close,
// the Events channel is eventually closed.
func (w *Watcher) Close() error {
	w.closeOnce.Do(func() { close(w.done) })
	return w.w.Close()
}
