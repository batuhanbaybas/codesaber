package search

import "testing"

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pat  string
		path string
		want bool
	}{
		{"*.ts", "a.ts", true},
		{"*.ts", "src/a.ts", true}, // no '/' → any depth
		{"*.ts", "src/a.tsx", false},
		{"src/*.ts", "src/a.ts", true},
		{"src/*.ts", "src/sub/a.ts", false}, // '*' stays in one level
		{"src/**/*.ts", "src/sub/a.ts", true},
		{"src/**/*.ts", "src/a.ts", true}, // '**' matches zero segments
		{"**/*_test.go", "pkg/a_test.go", true},
		{"**/*_test.go", "pkg/sub/b_test.go", true},
		{"**/*_test.go", "pkg/a.go", false},
		{"a?.ts", "ab.ts", true},
		{"a?.ts", "a.ts", false},
		{"docs/a.md", "docs/a.md", true},
		{"docs/a.md", "docs/b.md", false},
		{"**/*.md", "docs/readme.md", true},
		{"**/*.md", "readme.md", true},
	}
	for _, c := range cases {
		if got := globMatch(c.pat, c.path); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.pat, c.path, got, c.want)
		}
	}
}
