// editorconfig: applies .editorconfig to every document rhun opens, through rhun's per-document
// options: indent_style and indent_size/tab_width, trim_trailing_whitespace and
// insert_final_newline. Files are read from the document's folder up to the root of its
// workspace folder (or a section with root = true); later files and later sections win, as the
// EditorConfig specification says. When a .editorconfig changes on disk, the open documents get
// their settings again. Nothing is written anywhere.
// Protocol: https://rhun.app/extensions/v1/ and its 1.1 features workspace-folders (each folder of
// the window is a root) and workspace_watch (the .editorconfig files are read again on a change).
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/rhun-editor/extension-protocol/go/rhunext"
)

type ext struct {
	project string
	folders []string                          // the window's workspace folders (protocol 1.1)
	paths   map[rhunext.Doc]string            // the open documents with a path
	applied map[rhunext.Doc]map[string]string // what was sent, for the show command and changes
}

func (x *ext) Ready(e *rhunext.Ext, project string) {
	x.project = project
	x.paths = map[rhunext.Doc]string{}
	x.applied = map[rhunext.Doc]map[string]string{}
	if e.Has(rhunext.FeatureWorkspaceWatch) && project != "" {
		if _, err := e.WatchWorkspace(); err != nil {
			e.Logf("watch: %v", err)
		}
	}
}

func (x *ext) Event(e *rhunext.Ext, ev rhunext.Event) {
	if folders, ok := rhunext.ParseFolders(ev); ok {
		// before ready, and again when they change; a folder of another kind (remote) is left out
		x.folders = x.folders[:0]
		for _, f := range folders {
			if f.Location == "local" {
				x.folders = append(x.folders, f.Path)
			}
		}
		return
	}
	if _, _, ok := rhunext.ParseWorkspaceWatch(ev); ok {
		// something changed in the workspace: maybe a .editorconfig, so every open document again
		for doc := range x.paths {
			x.apply(e, doc)
		}
		return
	}
	switch ev.Name {
	case "opened":
		o, ok := rhunext.ParseOpened(ev)
		if !ok || o.Path == "" {
			return
		}
		x.paths[o.Doc] = o.Path
		x.apply(e, o.Doc)
	case "closed":
		var id int64
		if len(ev.Args) > 0 {
			fmt.Sscan(ev.Args[0], &id)
		}
		delete(x.paths, rhunext.Doc(id))
		delete(x.applied, rhunext.Doc(id))
	}
}

// apply sends the options that apply to a document now, and "-" for those that no longer do.
func (x *ext) apply(e *rhunext.Ext, doc rhunext.Doc) {
	path := x.paths[doc]
	opts := options(lookup(path, x.root(path)))
	before := x.applied[doc]
	for key, value := range opts {
		if before[key] == value {
			continue
		}
		if err := e.Option(doc, key, value); err != nil {
			e.Logf("option %s %s: %v", key, value, err)
		}
	}
	for key := range before {
		if _, still := opts[key]; !still {
			if err := e.Option(doc, key, "-"); err != nil {
				e.Logf("option %s -: %v", key, err)
			}
		}
	}
	x.applied[doc] = opts
}

// root is the folder the lookup stops at: the workspace folder the file is in (protocol 1.1), else
// the project when the file is in it; "" (the file system's root) otherwise.
func (x *ext) root(path string) string {
	best := ""
	for _, f := range append(x.folders, x.project) {
		if f != "" && strings.HasPrefix(path, f+string(filepath.Separator)) && len(f) > len(best) {
			best = f
		}
	}
	return best
}

func (x *ext) Run(e *rhunext.Ext, command string, doc rhunext.Doc) {
	if command != "editorconfig.show" {
		return
	}
	opts := x.applied[doc]
	if len(opts) == 0 {
		e.Toast("EditorConfig: nothing applies to this file")
		return
	}
	keys := make([]string, 0, len(opts))
	for k := range opts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+opts[k])
	}
	e.Toast("EditorConfig: " + strings.Join(parts, " "))
}

// options maps EditorConfig properties to rhun's option keys.
func options(props map[string]string) map[string]string {
	out := map[string]string{}
	switch props["indent_style"] {
	case "tab":
		out["insert_spaces"] = "no"
	case "space":
		out["insert_spaces"] = "yes"
	}
	// rhun has one width for tabs and indents: a tab-indented file shows tab_width columns per
	// tab (indent_size when there is none), a space-indented one indents by indent_size
	size, width := props["indent_size"], props["tab_width"]
	if size == "tab" {
		size = ""
	}
	if props["indent_style"] == "tab" && width != "" {
		size = width
	}
	if size == "" {
		size = width
	}
	if n, err := strconv.Atoi(size); err == nil && n >= 1 && n <= 16 {
		out["tab_width"] = strconv.Itoa(n)
	}
	if v, ok := yesNo(props["trim_trailing_whitespace"]); ok {
		out["trim_trailing_whitespace"] = v
	}
	if v, ok := yesNo(props["insert_final_newline"]); ok {
		out["final_newline"] = v
	}
	return out
}

func yesNo(v string) (string, bool) {
	switch v {
	case "true":
		return "yes", true
	case "false":
		return "no", true
	}
	return "", false
}

// lookup collects the properties that apply to the file: the .editorconfig files from root ("":
// the file system's root), or the first with root = true, down to the file's folder, each file's
// sections in order.
func lookup(path, root string) map[string]string {
	var files []string
	dir := filepath.Dir(path)
	for {
		files = append(files, filepath.Join(dir, ".editorconfig"))
		if root != "" && (dir == root || !strings.HasPrefix(dir, root+string(filepath.Separator))) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	props := map[string]string{}
	// nearest first: find the root, then apply from the farthest kept down to the nearest
	var parsed []*config
	for _, f := range files {
		c := parse(f)
		if c == nil {
			continue
		}
		parsed = append(parsed, c)
		if c.root {
			break
		}
	}
	for i := len(parsed) - 1; i >= 0; i-- {
		c := parsed[i]
		rel := path
		if r, err := filepath.Rel(c.dir, path); err == nil {
			rel = r
		}
		rel = filepath.ToSlash(rel)
		for _, sec := range c.sections {
			if matches(sec.pattern, rel) {
				for k, v := range sec.props {
					props[k] = v
				}
			}
		}
	}
	return props
}

type section struct {
	pattern string
	props   map[string]string
}

type config struct {
	dir      string
	root     bool
	sections []section
}

func parse(file string) *config {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer f.Close()
	c := &config{dir: filepath.Dir(file)}
	var cur *section
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' && strings.HasSuffix(line, "]") {
			c.sections = append(c.sections, section{pattern: line[1 : len(line)-1], props: map[string]string{}})
			cur = &c.sections[len(c.sections)-1]
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			k, v, ok = strings.Cut(line, ":")
		}
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.ToLower(strings.TrimSpace(v))
		if cur == nil {
			if k == "root" && v == "true" {
				c.root = true
			}
			continue
		}
		cur.props[k] = v
	}
	return c
}

// matches implements EditorConfig globs: *, **, ?, [...], [!...], {a,b}, {n..m}; a pattern
// without a slash matches the file's name anywhere below the config's folder.
func matches(pattern, rel string) bool {
	if !strings.Contains(pattern, "/") {
		pattern = "**/" + pattern
	} else {
		pattern = strings.TrimPrefix(pattern, "/")
	}
	return glob(pattern, rel)
}

func glob(p, s string) bool {
	for len(p) > 0 {
		switch {
		case strings.HasPrefix(p, "**"):
			rest := p[2:]
			if strings.HasPrefix(rest, "/") {
				// "**/": zero or more whole path segments
				rest = rest[1:]
				for i := 0; i <= len(s); i++ {
					if (i == 0 || s[i-1] == '/') && glob(rest, s[i:]) {
						return true
					}
				}
				return false
			}
			for i := 0; i <= len(s); i++ {
				if glob(rest, s[i:]) {
					return true
				}
			}
			return false
		case p[0] == '*':
			for i := 0; i <= len(s); i++ {
				if glob(p[1:], s[i:]) {
					return true
				}
				if i < len(s) && s[i] == '/' {
					return false
				}
			}
			return false
		case p[0] == '?':
			if s == "" || s[0] == '/' {
				return false
			}
			p, s = p[1:], s[1:]
		case p[0] == '[':
			end := strings.IndexByte(p, ']')
			if end < 0 || s == "" {
				return false
			}
			set, neg := p[1:end], false
			if strings.HasPrefix(set, "!") {
				set, neg = set[1:], true
			}
			ok := false
			for i := 0; i < len(set); i++ {
				if i+2 < len(set) && set[i+1] == '-' {
					if set[i] <= s[0] && s[0] <= set[i+2] {
						ok = true
					}
					i += 2
				} else if set[i] == s[0] {
					ok = true
				}
			}
			if ok == neg {
				return false
			}
			p, s = p[end+1:], s[1:]
		case p[0] == '{':
			end := strings.IndexByte(p, '}')
			if end < 0 {
				return false
			}
			alts := strings.Split(p[1:end], ",")
			if len(alts) == 1 {
				if a, b, ok := strings.Cut(alts[0], ".."); ok {
					lo, e1 := strconv.Atoi(a)
					hi, e2 := strconv.Atoi(b)
					if e1 == nil && e2 == nil {
						alts = nil
						for n := lo; n <= hi; n++ {
							alts = append(alts, strconv.Itoa(n))
						}
					}
				}
			}
			for _, a := range alts {
				if glob(a+p[end+1:], s) {
					return true
				}
			}
			return false
		case p[0] == '\\' && len(p) > 1:
			if s == "" || s[0] != p[1] {
				return false
			}
			p, s = p[2:], s[1:]
		default:
			if s == "" || s[0] != p[0] {
				return false
			}
			p, s = p[1:], s[1:]
		}
	}
	return s == ""
}

func main() {
	features := []string{rhunext.FeatureWorkspaceFolders, rhunext.FeatureWorkspaceWatch}
	if err := rhunext.RunWith(features, &ext{}); err != nil {
		os.Exit(1)
	}
}
