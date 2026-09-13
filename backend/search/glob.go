package search

import "strings"

// globMatch reports whether path (rel, '/'-separated) matches a single glob
// pattern. Semantics: '**' matches any number of path segments (including
// zero), '*' matches any run of non-separator chars within one segment, '?'
// matches exactly one non-separator char. A pattern with no '/' matches
// against the basename at any depth (like '*.ts' matching 'src/a.ts').
func globMatch(pattern, path string) bool {
	pattern = strings.TrimPrefix(pattern, "./")
	if pattern == "" || path == "" {
		return false
	}
	if !strings.Contains(pattern, "/") {
		pattern = "**/" + pattern
	}
	return matchSegs(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

func matchSegs(pat, path []string) bool {
	for {
		if len(pat) == 0 {
			return len(path) == 0
		}
		if pat[0] == "**" {
			// Collapse consecutive '**' and try every split point.
			for len(pat) > 1 && pat[1] == "**" {
				pat = pat[1:]
			}
			for i := 0; i <= len(path); i++ {
				if matchSegs(pat[1:], path[i:]) {
					return true
				}
			}
			return false
		}
		if len(path) == 0 || !globSeg(pat[0], path[0]) {
			return false
		}
		pat, path = pat[1:], path[1:]
	}
}

// globSeg matches one path segment: pattern bytes against segment with
// literal, '?' and '*' (one '*' may span any length in the segment).
func globSeg(pat, seg string) bool {
	// Iterative with backtracking: remember star position.
	var pi, si, starP, starS = 0, 0, -1, 0
	for si < len(seg) {
		switch {
		case pi < len(pat) && (pat[pi] == '?' || pat[pi] == seg[si]):
			pi++
			si++
		case pi < len(pat) && pat[pi] == '*':
			starP = pi
			starS = si
			pi++
		case starP >= 0:
			starS++
			si = starS
			pi = starP + 1
		default:
			return false
		}
	}
	for pi < len(pat) && pat[pi] == '*' {
		pi++
	}
	return pi == len(pat)
}

// splitGlobs splits a comma-separated glob expression into trimmed, non-empty
// patterns.
func splitGlobs(spec string) []string {
	parts := strings.Split(spec, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// globFilter returns a filter over '/'-separated rel paths built from
// include/exclude comma-separated spec empties. nil = keep everything.
func globFilter(include, exclude string) func(rel string) bool {
	inc := splitGlobs(include)
	exc := splitGlobs(exclude)
	if len(inc) == 0 && len(exc) == 0 {
		return nil
	}
	return func(rel string) bool {
		for _, e := range exc {
			if globMatch(e, rel) {
				return false
			}
		}
		if len(inc) == 0 {
			return true
		}
		for _, i := range inc {
			if globMatch(i, rel) {
				return true
			}
		}
		return false
	}
}
