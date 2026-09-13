// Package search implements parallel project-wide text search. It walks the
// project tree applying the same skip rules as the file index (dotfiles except
// .env/.gitignore/.github, vendored/build dirs, binary extensions), filters
// files through Query.Include/Query.Exclude comma-separated doublestar globs
// ('/'-normalized rel paths), then scans remaining files with a bounded
// worker pool. Literal mode is raw-byte contains when Query.CaseSensitive,
// else case-folded. Results are capped (matches and files) with a Truncated
// flag; cancellation flows through context.Context.
package search

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Match is one hit inside a file. Line is 1-based; Col is the 1-based byte
// offset of the match start within the line (approximate for multi-byte
// unicode when case folding shifts byte offsets — acceptable MVP). Text is
// the full line capped at 200 bytes.
type Match struct {
	Line int    `json:"line"`
	Col  int    `json:"col"`
	Text string `json:"text"`
}

// FileMatches groups the matches found in one file (absolute path).
type FileMatches struct {
	Path    string  `json:"path"`
	Matches []Match `json:"matches"`
}

// Result is the aggregate outcome of one search.
type Result struct {
	Files        int           `json:"files"`
	Matches      int           `json:"matches"`
	FilesMatches []FileMatches `json:"filesMatches"`
	Truncated    bool          `json:"truncated"`
}

// Query describes one search. MaxSizeMB/MaxMatches/MaxFiles are knobs for
// tests and callers; zero means the documented default.
type Query struct {
	Term          string
	Regex         bool
	CaseSensitive bool   // literal mode: raw contains; regex mode: not case-insensitive
	Include       string // comma-separated globs to keep, e.g. "*.ts, src/**"
	Exclude       string // comma-separated globs to drop, e.g. "*.md, docs/**"
	MaxSizeMB     int    // per-file size limit; 0 = 10MB
	MaxMatches    int    // total match cap;    0 = 2000
	MaxFiles      int    // file result cap;    0 = 200
}

const (
	defaultMaxSizeMB = 10
	defaultMaxMatch  = 2000
	defaultMaxFiles  = 200
	maxLineBytes     = 200
	maxWorkers       = 16
)

// skipDirs are directories never descended into (mirrors backend/app.go).
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "dist": true, "build": true,
	"vendor": true, "target": true, ".next": true,
}

// allowedDots are the dot-named entries still searched (mirrors backend/app.go;
// .github is a directory).
var allowedDots = map[string]bool{".env": true, ".gitignore": true, ".github": true}

// binaryExts: files with these extensions are never text-scanned.
var binaryExts = map[string]bool{
	"png": true, "jpg": true, "jpeg": true, "gif": true, "bmp": true,
	"ico": true, "webp": true, "icns": true, "pdf": true,
	"zip": true, "gz": true, "tar": true, "bz2": true, "xz": true, "7z": true,
	"bin": true, "exe": true, "dll": true, "so": true, "dylib": true,
	"o": true, "a": true, "obj": true, "lib": true, "wasm": true,
	"woff": true, "woff2": true, "ttf": true, "otf": true, "eot": true,
	"mp3": true, "mp4": true, "mov": true, "avi": true, "mkv": true,
	"flac": true, "ogg": true, "wav": true, "class": true, "jar": true,
}

func extOf(name string) string {
	ext := filepath.Ext(name)
	return strings.ToLower(strings.TrimPrefix(ext, "."))
}

// collect walks root and returns the files to scan (absolute paths), applying
// the skip rules. Unreadable directories abort the walk with an error.
func collect(root string) ([]string, error) {
	var files []string
	var walk func(dir string) error
	walk = func(dir string) error {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("read %s: %w", dir, err)
		}
		for _, e := range ents {
			name := e.Name()
			hidden := strings.HasPrefix(name, ".") && !allowedDots[name]
			if e.IsDir() {
				if hidden || skipDirs[name] {
					continue
				}
				if err := walk(filepath.Join(dir, name)); err != nil {
					return err
				}
				continue
			}
			if hidden || binaryExts[extOf(name)] {
				continue
			}
			files = append(files, filepath.Join(dir, name))
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// Search runs q over the tree rooted at root. Literal mode uses raw-byte
// contains when q.CaseSensitive, else case-folded comparisons via
// strings.ToLower on both term and lines (byte-offset columns are approximate
// when folding changes byte lengths); regex mode compiles the pattern with
// (?i) unless q.CaseSensitive. Files are filtered through the
// include/exclude globs (rel '/'-separated paths) and scanned by
// min(NumCPU, 16) workers.
func Search(root string, q Query, ctx context.Context) (Result, error) {
	if strings.TrimSpace(q.Term) == "" {
		return Result{}, errors.New("search: empty term")
	}
	maxSize := int64(q.MaxSizeMB) << 20
	if maxSize == 0 {
		maxSize = defaultMaxSizeMB << 20
	}
	maxMatches := q.MaxMatches
	if maxMatches == 0 {
		maxMatches = defaultMaxMatch
	}
	maxFiles := q.MaxFiles
	if maxFiles == 0 {
		maxFiles = defaultMaxFiles
	}

	var re *regexp.Regexp
	term := q.Term
	if q.Regex {
		var err error
		pat := q.Term
		if !q.CaseSensitive {
			pat = "(?i)" + pat
		}
		re, err = regexp.Compile(pat)
		if err != nil {
			return Result{}, fmt.Errorf("search: %w", err)
		}
	} else if !q.CaseSensitive {
		term = strings.ToLower(q.Term)
	}

	files, err := collect(root)
	if err != nil {
		return Result{}, err
	}

	filter := globFilter(q.Include, q.Exclude)
	if filter != nil {
		rootPrefix := filepath.Clean(root) + string(filepath.Separator)
		kept := files[:0]
		for _, f := range files {
			rel := strings.TrimPrefix(strings.TrimPrefix(f, rootPrefix), string(filepath.Separator))
			rel = filepath.ToSlash(rel)
			if filter(rel) {
				kept = append(kept, f)
			}
		}
		files = kept
	}
	if len(files) == 0 {
		return Result{}, nil
	}

	// runCtx is canceled internally once a cap is hit; the parent ctx still
	// controls user cancellation and is checked separately afterwards.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	paths := make(chan int)
	var mu sync.Mutex
	perFile := make([]FileMatches, len(files))
	totalMatches, filesHit, truncated := 0, 0, false

	worker := func() {
		for idx := range paths {
			fm, stop := scanFile(runCtx, files[idx], term, re, maxSize, maxMatches, q.CaseSensitive)
			mu.Lock()
			add := len(fm.Matches)
			over := stop
			// File cap already reached: drop this result entirely.
			if filesHit >= maxFiles && add > 0 {
				over = true
				add = 0
				fm.Matches = nil
			}
			// Match cap: clamp the tail of this file's matches.
			if add > 0 && totalMatches+add > maxMatches {
				fm.Matches = fm.Matches[:maxMatches-totalMatches]
				add = maxMatches - totalMatches
				over = true
			}
			if add > 0 {
				perFile[idx] = fm
				totalMatches += add
				filesHit++
			}
			if over || totalMatches >= maxMatches || filesHit >= maxFiles {
				truncated = true
				cancel()
			}
			mu.Unlock()
			select {
			case <-runCtx.Done():
				return
			default:
			}
		}
	}

	nw := runtime.NumCPU()
	if nw > maxWorkers {
		nw = maxWorkers
	}
	if nw < 1 {
		nw = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < nw; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker()
		}()
	}

	produce := func() error {
		defer close(paths)
		for i := range files {
			select {
			case paths <- i:
			case <-runCtx.Done():
				return runCtx.Err()
			}
		}
		return nil
	}
	_ = produce()
	wg.Wait()

	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}

	res := Result{Truncated: truncated}
	for _, fm := range perFile {
		if len(fm.Matches) == 0 {
			continue
		}
		res.FilesMatches = append(res.FilesMatches, fm)
	}
	res.Files = len(res.FilesMatches)
	res.Matches = totalMatches
	return res, nil
}

// scanFile scans one file for term/re and returns (matches, stop) where stop
// reports that a cap was hit mid-file.
func scanFile(
	ctx context.Context,
	path string,
	term string, re *regexp.Regexp,
	maxSize int64, maxMatches int, caseSensitive bool,
) (FileMatches, bool) {
	fm := FileMatches{Path: path}
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxSize {
		return fm, false
	}
	f, err := os.Open(path)
	if err != nil {
		return fm, false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		select {
		case <-ctx.Done():
			return fm, true
		default:
		}
		line := sc.Text()
		for _, m := range lineMatches(line, term, re, caseSensitive) {
			if len(fm.Matches) >= maxMatches {
				return fm, true
			}
			fm.Matches = append(fm.Matches, Match{
				Line: lineNo,
				Col:  m + 1,
				Text: capLine(line),
			})
		}
	}
	return fm, false
}

// lineMatches returns the 0-based byte offsets of every match in line.
// Regex mode uses re. Literal mode: if caseSensitive, raw-byte contains on
// the line; otherwise both sides are folded with strings.ToLower and the
// folded offset is reused against the raw line (approximate for multi-byte
// case folds).
func lineMatches(line, term string, re *regexp.Regexp, caseSensitive bool) []int {
	if re != nil {
		locs := re.FindAllIndex([]byte(line), -1)
		out := make([]int, 0, len(locs))
		for _, l := range locs {
			out = append(out, l[0])
		}
		return out
	}
	if caseSensitive {
		var out []int
		for off := 0; ; {
			j := strings.Index(line[off:], term)
			if j < 0 {
				break
			}
			out = append(out, off+j)
			off += j + len(term)
		}
		return out
	}
	folded := strings.ToLower(line)
	var out []int
	for off := 0; ; {
		j := strings.Index(folded[off:], term)
		if j < 0 {
			break
		}
		out = append(out, off+j)
		off += j + len(term)
	}
	return out
}

// capLine trims the line to maxLineBytes (best-effort byte cap).
func capLine(line string) string {
	if len(line) <= maxLineBytes {
		return line
	}
	return line[:maxLineBytes]
}
