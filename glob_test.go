package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlob(t *testing.T) {
	cases := []struct {
		pattern, rel string
		want         bool
	}{
		{"*", "a.go", true}, {"*", "sub/a.go", true}, {"*.go", "sub/a.go", true}, {"*.go", "a.py", false},
		{"sub/*.go", "sub/a.go", true}, {"sub/*.go", "sub/deep/a.go", false}, {"**/*.go", "sub/deep/a.go", true},
		{"lib/**.js", "lib/a/b.js", true}, {"*.{js,ts}", "x.ts", true}, {"*.{js,ts}", "x.go", false},
		{"[Mm]akefile", "Makefile", true}, {"[!a]*.go", "b.go", true}, {"[!a]*.go", "a.go", false},
		{"file{1..3}.txt", "file2.txt", true}, {"file{1..3}.txt", "file4.txt", false}, {"a?c", "abc", true},
		{"a?c", "a/c", false}, {"{package.json,*.yml}", "package.json", true},
	}
	for _, c := range cases {
		if got := matches(c.pattern, c.rel); got != c.want {
			t.Errorf("matches(%q, %q) = %v, want %v", c.pattern, c.rel, got, c.want)
		}
	}
}

func TestOptions(t *testing.T) {
	got := options(map[string]string{"indent_style": "tab", "indent_size": "tab", "tab_width": "8",
		"trim_trailing_whitespace": "true", "insert_final_newline": "false"})
	want := map[string]string{"insert_spaces": "no", "tab_width": "8", "trim_trailing_whitespace": "yes", "final_newline": "no"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	// a tab-indented file inherits indent_size = 2 from [*] but shows tabs at tab_width
	if got := options(map[string]string{"indent_style": "tab", "indent_size": "2", "tab_width": "8"}); got["tab_width"] != "8" {
		t.Errorf("tab file: tab_width = %q, want 8", got["tab_width"])
	}
	if got := options(map[string]string{"indent_style": "space", "indent_size": "2", "tab_width": "8"}); got["tab_width"] != "2" {
		t.Errorf("space file: tab_width = %q, want 2", got["tab_width"])
	}
	if got := options(map[string]string{"indent_size": "3"}); got["tab_width"] != "3" {
		t.Errorf("size only: tab_width = %q, want 3", got["tab_width"])
	}
}

func TestRoot(t *testing.T) {
	x := &ext{project: "/w/app", folders: []string{"/w/app", "/w/lib", "/w/app/vendor"}}
	for path, want := range map[string]string{
		"/w/app/main.go":     "/w/app",
		"/w/lib/a/b.go":      "/w/lib",
		"/w/app/vendor/x.go": "/w/app/vendor", // the nearest folder
		"/w/library/x.go":    "",              // not in /w/lib
		"/elsewhere/x.go":    "",
	} {
		if got := filepath.ToSlash(x.root(path)); got != want {
			t.Errorf("root(%q) = %q, want %q", path, got, want)
		}
	}
	// Windows paths as rhun writes them: a drive and forward slashes
	x = &ext{project: "C:/w/app", folders: []string{"C:/w/app", "C:/w/lib"}}
	for path, want := range map[string]string{
		"C:/w/app/main.go":  "C:/w/app",
		"C:/w/lib/a/b.go":   "C:/w/lib",
		"C:/w/library/x.go": "",
	} {
		if got := filepath.ToSlash(x.root(path)); got != want {
			t.Errorf("root(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestLookupStopsAtTheRoot(t *testing.T) {
	top := t.TempDir()
	app := filepath.Join(top, "app")
	if err := os.MkdirAll(filepath.Join(app, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	// above the workspace folder: must not apply
	os.WriteFile(filepath.Join(top, ".editorconfig"), []byte("[*]\nindent_style = tab\n"), 0o644)
	os.WriteFile(filepath.Join(app, ".editorconfig"), []byte("[*]\nindent_size = 3\n"), 0o644)
	file := filepath.ToSlash(filepath.Join(app, "src", "x.go")) // as rhun writes it
	x := &ext{project: filepath.ToSlash(app)}
	got := lookup(file, x.root(file))
	if got["indent_size"] != "3" || got["indent_style"] != "" {
		t.Errorf("lookup = %v, want indent_size 3 and nothing from above the folder", got)
	}
}
