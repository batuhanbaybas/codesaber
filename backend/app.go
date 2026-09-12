// Package backend is the Wails binding facade. Every exported method on App
// becomes an RPC the frontend can call; all state lives in engines
// (project/editor/fswatch) and is namespaced per project by Project.ID.
package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"aide/backend/adapter"
	"aide/backend/editor"
	"aide/backend/fswatch"
	"aide/backend/project"
)

// maxTreeDepth limits ListTree recursion (root children = depth 1).
const maxTreeDepth = 2

// maxFileSize guards ReadFile against dumping huge binaries into memory.
const maxFileSize = 10 << 20 // 10MB

// Entry is a node in a project tree listing. Path is absolute.
type Entry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

// EventEngineStatus is emitted when an engine's health changes for a project
// (currently: fswatch watcher startup success/failure).
const EventEngineStatus = "engine.status"

// App is the main service bound to the UI. Pure facade over engines;
// all engine state is per-project, namespaced by Project.ID.
type App struct {
	sink     adapter.EventSink
	reg      *project.Registry
	store    *project.Store
	buf      *editor.Service
	mu       sync.Mutex
	watchers map[string]*fswatch.Watcher
	closed   map[string]bool
}

// New wires the engines together. sink receives all backend→UI events.
func New(sink adapter.EventSink) *App {
	return NewWith(sink, project.DefaultStorePath())
}

// NewWith is New with an injectable recents store path (for tests).
func NewWith(sink adapter.EventSink, storePath string) *App {
	return &App{
		sink:     sink,
		reg:      project.NewRegistry(func() {}),
		store:    project.NewStore(storePath),
		buf:      editor.New(),
		watchers: map[string]*fswatch.Watcher{},
		closed:   map[string]bool{},
	}
}

// OpenProject registers the project, remembers it in recents, starts a
// filesystem watcher and emits "project.added". Watcher events are re-emitted
// as "fs.change" with {projectId, path, op}.
func (a *App) OpenProject(root string) (project.Project, error) {
	p, err := a.reg.Add(root)
	if err != nil {
		return project.Project{}, err
	}
	a.store.Remember(p.ID, p.Root, p.Branch)

	if w, werr := fswatch.New(p.Root); werr == nil {
		a.mu.Lock()
		a.watchers[p.ID] = w
		a.mu.Unlock()
		go a.forwardWatcher(p.ID, w)
		a.sink.Emit(EventEngineStatus, map[string]any{
			"engine": "fswatch", "ok": true, "projectId": p.ID,
		})
	} else {
		a.reg.SetEngineOK(p.ID, false)
		a.sink.Emit(EventEngineStatus, map[string]any{
			"engine": "fswatch", "ok": false, "projectId": p.ID,
		})
		a.sink.Emit("engine.error", map[string]string{"projectId": p.ID, "message": werr.Error()})
	}

	a.sink.Emit(project.EventAdded, p)
	return *p, nil
}

func (a *App) forwardWatcher(projectID string, w *fswatch.Watcher) {
	for ev, ok := <-w.Events(); ok; ev, ok = <-w.Events() {
		if a.isClosed(projectID) {
			continue
		}
		a.sink.Emit("fs.change", map[string]string{
			"projectId": projectID,
			"path":      ev.Path,
			"op":        ev.Op,
		})
	}
}

// PickFolder opens a native folder-picker and returns the chosen absolute
// path, or an empty string if the user cancelled the dialog.
func (a *App) PickFolder() (string, error) {
	return adapter.PickFolder("Choose Project Folder")
}

// ListProjects returns currently open projects, most recently used first.
func (a *App) ListProjects() []project.Project {
	return a.reg.List()
}

// RecentProjects returns the persisted recents list.
func (a *App) RecentProjects() []project.Recent {
	return a.store.List()
}

// ForgetRecent drops a recents entry by root path.
func (a *App) ForgetRecent(root string) {
	a.store.Forget(root)
}

// RemoveProject closes the project: registry removal, watcher shutdown and
// "project.removed" emission.
func (a *App) RemoveProject(id string) error {
	p, err := a.reg.Get(id)
	if err != nil {
		return err
	}
	if err := a.reg.Remove(id); err != nil {
		return err
	}
	a.CloseWatcher(id)
	a.sink.Emit(project.EventRemoved, map[string]any{"id": id, "root": p.Root})
	if len(a.reg.List()) == 0 {
		adapter.ShowWelcomeWindow()
	}
	return nil
}

// CloseWatcher stops and releases the watcher for projectID and marks the
// project closed so the forwarder suppresses any buffered events.
func (a *App) CloseWatcher(projectID string) {
	a.mu.Lock()
	w, ok := a.watchers[projectID]
	delete(a.watchers, projectID)
	a.closed[projectID] = true
	a.mu.Unlock()
	if ok {
		w.Close()
	}
}

func (a *App) isClosed(projectID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closed[projectID]
}

// ListTree walks root recursively to maxTreeDepth, dirs-first sorted, skipping
// dotfiles except .env, .gitignore and .github.
func (a *App) ListTree(root string) ([]Entry, error) {
	return walk("", root, 0)
}

func walk(projectRoot, dir string, depth int) ([]Entry, error) {
	if depth >= maxTreeDepth {
		return nil, nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var dirs, files []Entry
	for _, e := range ents {
		name := e.Name()
		if !visible(name) {
			continue
		}
		path := filepath.Join(dir, name)
		entry := Entry{
			Name: name,
			Path: path, // os.ReadDir on an absolute dir yields absolute paths already
			Dir:  e.IsDir(),
		}
		if e.IsDir() {
			dirs = append(dirs, entry)
		} else {
			files = append(files, entry)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })

	var out []Entry
	for _, entry := range dirs {
		out = append(out, entry)
		children, err := walk(projectRoot, entry.Path, depth+1)
		if err != nil {
			return nil, err
		}
		out = append(out, children...)
	}
	out = append(out, files...)
	return out, nil
}

var allowedDots = map[string]bool{".env": true, ".gitignore": true, ".github": true}

func visible(name string) bool {
	if !strings.HasPrefix(name, ".") {
		return true
	}
	return allowedDots[name]
}

// ReadFile returns file content, refusing files larger than maxFileSize.
func (a *App) ReadFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file %s is larger than %d bytes", path, maxFileSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

// SaveFile persists content via editor.Service (atomic write + dirty tracking).
func (a *App) SaveFile(path, content string) error {
	return a.buf.Save(path, content)
}

// EnsureWorkspaceWindow opens the workspace window if none exists (or shows
// the existing one). Called by the frontend after opening a project.
func (a *App) EnsureWorkspaceWindow() {
	adapter.EnsureWorkspaceWindow()
}

// CloseWelcome hides the welcome window after a project has been opened.
func (a *App) CloseWelcome() {
	adapter.CloseWelcomeWindow()
}
