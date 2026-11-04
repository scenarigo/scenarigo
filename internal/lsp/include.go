package lsp

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/scenarigo/scenarigo/internal/lsp/yamlutil"
)

// Scenarios that include each other share names: an included scenario runs
// with the context of the including one (its vars, secrets and plugins), and
// the bind of the include step reads the final context of the included one
// (its vars and its steps).

// maxIncludeDepth bounds the chain of including scenarios that is followed.
const maxIncludeDepth = 4

// readText returns the text of a file, from the open buffer when there is one.
func (s *Server) readText(path string) (string, bool) {
	if doc := s.docs.GetByPath(path); doc != nil {
		return doc.Text, true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// scenarioNames returns every name the scenario documents of a file define,
// wherever they are defined.
func (s *Server) scenarioNames(path string) (templateDefinitions, bool) {
	defs := newTemplateDefinitions()
	text, ok := s.readText(path)
	if !ok {
		return defs, false
	}
	parsed := yamlutil.Parse(text)
	if parsed == nil || parsed.File == nil {
		return defs, false
	}
	v := newYAMLView(parsed.File)
	for _, d := range parsed.File.Docs {
		body := v.entries(d.Body)
		if body == nil {
			continue
		}
		defs.add(defs.plugins, v.keys(v.entryValue(body, nsPlugins)))
		defs.add(defs.vars, v.keys(v.entryValue(body, nsVars)))
		defs.add(defs.secrets, v.keys(v.entryValue(body, nsSecrets)))
		for _, step := range v.sequence(v.entryValue(body, nsSteps)) {
			entries := v.entries(step)
			if id := v.stringValue(v.entryValue(entries, "id")); id != "" {
				defs.steps[id] = true
			}
			// Step vars are how an include step passes parameters.
			defs.add(defs.vars, v.keys(v.entryValue(entries, nsVars)))
			defs.add(defs.secrets, v.keys(v.entryValue(entries, nsSecrets)))
			bind := v.entries(v.entryValue(entries, "bind"))
			defs.add(defs.vars, v.keys(v.entryValue(bind, nsVars)))
			defs.add(defs.secrets, v.keys(v.entryValue(bind, nsSecrets)))
		}
	}
	return defs, true
}

// includedPath resolves the include of a step in the document at docPath.
func includedPath(docPath, include string) string {
	if filepath.IsAbs(include) {
		return filepath.Clean(include)
	}
	return filepath.Join(filepath.Dir(docPath), include)
}

// includeIndex maps the YAML files of a workspace root to the files they
// include. It is built by one walk of the root and then follows the open
// buffers, so that diagnostics do not walk the workspace on every change.
type includeIndex struct {
	root     string
	includes map[string][]string // includer path -> included paths
	open     map[string]bool     // paths whose entry comes from an open buffer
}

// maxIncludeScanEntries bounds the walk that builds the include index.
const maxIncludeScanEntries = 20000

func (s *Server) buildIncludeIndex(rootPath string) *includeIndex {
	idx := &includeIndex{root: rootPath, includes: map[string][]string{}, open: map[string]bool{}}
	visited := 0
	_ = filepath.WalkDir(rootPath, func(p string, d fs.DirEntry, err error) error {
		if visited++; visited > maxIncludeScanEntries {
			return filepath.SkipAll
		}
		if err != nil {
			return nil //nolint:nilerr // skip what cannot be read
		}
		if d.IsDir() {
			if p != rootPath && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(p); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		if data, err := os.ReadFile(p); err == nil {
			idx.set(p, string(data))
		}
		return nil
	})
	return idx
}

// set records the includes of the file at path from its text.
func (idx *includeIndex) set(path, text string) {
	var included []string
	for _, include := range includeValues(text) {
		included = append(included, includedPath(path, include))
	}
	if len(included) == 0 {
		delete(idx.includes, path)
		return
	}
	idx.includes[path] = included
}

// findIncluders returns the YAML files under the workspace root that
// include the file at path.
func (s *Server) findIncluders(path string) []string {
	rootPath := uriToPath(s.rootURI)
	if rootPath == "" || path == "" {
		return nil
	}
	s.includeIndexMu.Lock()
	defer s.includeIndexMu.Unlock()
	if s.includeIndex == nil || s.includeIndex.root != rootPath {
		s.includeIndex = s.buildIncludeIndex(rootPath)
	}
	// Open buffers win over the files on disk; a buffer closed since the
	// last lookup gives the file on disk back its say, unsaved edits dropped.
	idx := s.includeIndex
	open := map[string]bool{}
	for _, uri := range s.docs.URIs() {
		if p := uriToPath(uri); strings.HasPrefix(p, rootPath+string(filepath.Separator)) {
			if doc := s.docs.Get(uri); doc != nil {
				idx.set(p, doc.Text)
				open[p] = true
			}
		}
	}
	for p := range idx.open {
		if open[p] {
			continue
		}
		if data, err := os.ReadFile(p); err == nil {
			idx.set(p, string(data))
		} else {
			delete(idx.includes, p)
		}
	}
	idx.open = open
	var found []string
	for includer, included := range s.includeIndex.includes {
		if includer != path && slices.Contains(included, path) {
			found = append(found, includer)
		}
	}
	slices.Sort(found)
	return found
}

// includeValues returns the values of the include keys of a YAML text, in
// block (include: a.yaml) or flow ({include: a.yaml, bind: ...}) style.
func includeValues(text string) []string {
	const key = "include:"
	var out []string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for off := 0; ; {
			i := strings.Index(line[off:], key)
			if i < 0 {
				break
			}
			i += off
			off = i + len(key)
			if i > 0 && !strings.ContainsRune(" \t{,-", rune(line[i-1])) {
				continue // another key ending in "include"
			}
			value := strings.TrimSpace(line[off:])
			if value != "" && (value[0] == '"' || value[0] == '\'') {
				if end := strings.IndexByte(value[1:], value[0]); end >= 0 {
					value = value[1 : end+1]
				}
			} else {
				if j := strings.IndexAny(value, ",}"); j >= 0 {
					value = value[:j]
				}
				value, _, _ = strings.Cut(value, " #")
			}
			if value = strings.TrimSpace(value); value != "" {
				out = append(out, value)
			}
		}
	}
	return out
}

// includerNames returns the names that the scenarios including the file at
// path, directly or through other includes, define. An included scenario
// runs with their vars, secrets and plugins.
func (s *Server) includerNames(path string) templateDefinitions {
	defs := newTemplateDefinitions()
	seen := map[string]bool{path: true}
	current := []string{path}
	for range maxIncludeDepth {
		var next []string
		for _, p := range current {
			for _, includer := range s.findIncluders(p) {
				if seen[includer] {
					continue
				}
				seen[includer] = true
				names, _ := s.scenarioNames(includer)
				defs.merge(names)
				next = append(next, includer)
			}
		}
		if len(next) == 0 {
			break
		}
		current = next
	}
	return defs
}

// publishDependentDiagnostics publishes the diagnostics of the other open
// documents again when uri is scenarigo.yaml, whose names they can use.
func (s *Server) publishDependentDiagnostics(uri string) {
	rootPath := uriToPath(s.rootURI)
	if rootPath == "" || uriToPath(uri) != filepath.Join(rootPath, "scenarigo.yaml") {
		return
	}
	for _, other := range s.docs.URIs() {
		if other != uri {
			s.publishDiagnostics(other)
		}
	}
}
