package lsp

import (
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/scenarigo/scenarigo/internal/lsp/yamlutil"
)

// Scenarios that include each other share names: an included scenario runs
// with the context of the including one (its vars, secrets and plugins), and
// the bind of the include step reads the final context of the included one
// (its vars, secrets, plugins and steps).

// maxIncludeScanEntries and maxIncludeScanBytes bound the walk that builds the
// include index, which runs while a client waits.
const (
	maxIncludeScanEntries = 20000
	maxIncludeScanBytes   = 64 << 20
)

// readText returns the text of a file, from the open buffer when there is one.
func (s *Server) readText(path string) (string, bool) {
	if doc := s.docs.GetByPath(path); doc != nil {
		return doc.Text, true
	}
	data, ok := readScannedFile(path)
	return string(data), ok
}

// scenarioNames returns the names a file of scenarios leaves in the final
// context of its scenarios: their plugins, vars and secrets, what their steps
// bind, and their step ids. With passed set it adds the vars and secrets of
// its include steps, which only the included scenarios see.
func (s *Server) scenarioNames(path string, passed bool) (templateDefinitions, bool) {
	defs := newTemplateDefinitions()
	text, ok := s.readText(path)
	if !ok {
		return defs, false
	}
	parsed := yamlutil.Parse(text)
	if parsed == nil || parsed.File == nil {
		return defs, false
	}
	for v, d := range yamlDocuments(parsed.File) {
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
			if passed && v.stringValue(v.entryValue(entries, "include")) != "" {
				defs.add(defs.vars, v.keys(v.entryValue(entries, nsVars)))
				defs.add(defs.secrets, v.keys(v.entryValue(entries, nsSecrets)))
			}
			bind := v.entries(v.entryValue(entries, "bind"))
			defs.add(defs.vars, v.keys(v.entryValue(bind, nsVars)))
			defs.add(defs.secrets, v.keys(v.entryValue(bind, nsSecrets)))
		}
	}
	return defs, true
}

// includedPath resolves the include of a step in the document at docPath.
// Like the runner (step.go), it joins an absolute include to the directory
// of the document too.
func includedPath(docPath, include string) string {
	return filepath.Join(filepath.Dir(docPath), include)
}

// includeIndex maps the YAML files of a workspace root to the files they
// include. It is built by one walk of the root and then follows the open
// buffers, so that diagnostics do not walk the workspace on every change.
type includeIndex struct {
	root     string
	includes map[string][]string // includer path -> included paths
	open     map[string]bool     // paths whose entry comes from an open buffer
	partial  bool                // the walk missed a file or stopped at a bound
	dropped  map[string]bool     // included paths that lost an includer since takeDroppedIncludes
}

func (s *Server) buildIncludeIndex(rootPath string) *includeIndex {
	idx := &includeIndex{root: rootPath, includes: map[string][]string{}, open: map[string]bool{}, dropped: map[string]bool{}}
	visited, size := 0, 0
	_ = filepath.WalkDir(rootPath, func(p string, d fs.DirEntry, err error) error {
		if visited++; visited > maxIncludeScanEntries {
			idx.partial = true
			return filepath.SkipAll
		}
		if err != nil {
			idx.partial = true
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
		data, ok := readScannedFile(p)
		if !ok {
			idx.partial = true
			return nil
		}
		if size += len(data); size > maxIncludeScanBytes {
			idx.partial = true
			return filepath.SkipAll
		}
		if !idx.setText(p, string(data)) {
			idx.partial = true
		}
		return nil
	})
	clear(idx.dropped)
	return idx
}

// setText records the includes of the file at path from its text, which most
// YAML files of a workspace need not be parsed for. It reports whether the
// text could be read for them.
func (idx *includeIndex) setText(path, text string) bool {
	if !strings.Contains(text, "include") {
		idx.set(path, nil)
		return true
	}
	parsed := yamlutil.Parse(text)
	idx.set(path, parsed)
	return parsed != nil
}

// set records the includes of the file at path from its parsed text.
func (idx *includeIndex) set(path string, parsed *yamlutil.Document) {
	var included []string
	for _, include := range includeValues(parsed) {
		included = append(included, includedPath(path, include))
	}
	for _, old := range idx.includes[path] {
		if !slices.Contains(included, old) {
			idx.dropped[old] = true
		}
	}
	if len(included) == 0 {
		delete(idx.includes, path)
		return
	}
	idx.includes[path] = included
}

// includeValues returns the include of each step of the scenarios of a file.
func includeValues(parsed *yamlutil.Document) []string {
	if parsed == nil || parsed.File == nil {
		return nil
	}
	var out []string
	for v, d := range yamlDocuments(parsed.File) {
		for _, step := range v.sequence(v.entryValue(v.entries(d.Body), nsSteps)) {
			if include := v.stringValue(v.entryValue(v.entries(step), "include")); include != "" {
				out = append(out, include)
			}
		}
	}
	return out
}

// includeEdges returns the includes of the workspace as includer path ->
// included paths, and whether the walk read all of a workspace. Open buffers
// win over the files on disk; a buffer closed since the last call gives the
// file on disk back its say, unsaved edits dropped.
func (s *Server) includeEdges() (map[string][]string, bool) {
	rootPath := uriToPath(s.rootURI)
	if rootPath == "" {
		return nil, false
	}
	rootPath = filepath.Clean(rootPath)
	s.includeIndexMu.Lock()
	defer s.includeIndexMu.Unlock()
	if s.includeIndex == nil || s.includeIndex.root != rootPath {
		s.includeIndex = s.buildIncludeIndex(rootPath)
	}
	idx := s.includeIndex
	open := map[string]bool{}
	for _, uri := range s.docs.URIs() {
		if p := uriToPath(uri); strings.HasPrefix(p, rootPath+string(filepath.Separator)) {
			if doc := s.docs.Get(uri); doc != nil {
				idx.set(p, doc.Parsed)
				open[p] = true
			}
		}
	}
	for p := range idx.open {
		if !open[p] {
			data, _ := readScannedFile(p)
			idx.setText(p, string(data))
		}
	}
	idx.open = open
	return maps.Clone(idx.includes), !idx.partial
}

// takeDroppedIncludes returns the paths that lost an includer since the last
// call: reading the edges can come first, for the diagnostics of the file that
// dropped them.
func (s *Server) takeDroppedIncludes() []string {
	s.includeIndexMu.Lock()
	defer s.includeIndexMu.Unlock()
	if s.includeIndex == nil {
		return nil
	}
	dropped := slices.Collect(maps.Keys(s.includeIndex.dropped))
	clear(s.includeIndex.dropped)
	return dropped
}

// reachable returns the paths that edges lead to from path, itself excluded,
// following any number of steps. The includes of a workspace may form cycles.
func reachable(edges map[string][]string, path string) []string {
	seen := map[string]bool{path: true}
	var out []string
	pending := []string{path}
	for len(pending) > 0 {
		next := edges[pending[len(pending)-1]]
		pending = pending[:len(pending)-1]
		for _, p := range next {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
				pending = append(pending, p)
			}
		}
	}
	slices.Sort(out)
	return out
}

func invertEdges(edges map[string][]string) map[string][]string {
	inverted := map[string][]string{}
	for from, tos := range edges {
		for _, to := range tos {
			inverted[to] = append(inverted[to], from)
		}
	}
	return inverted
}

// includerNames returns the names that the scenarios including the file at
// path, directly or through other includes, define for it: an included
// scenario runs with their vars, secrets and plugins. The second result is
// false when one of them could not be read, or a path has no workspace.
func (s *Server) includerNames(path string) (templateDefinitions, bool) {
	defs := newTemplateDefinitions()
	if path == "" {
		return defs, true
	}
	edges, complete := s.includeEdges()
	for _, includer := range reachable(invertEdges(edges), path) {
		names, ok := s.scenarioNames(includer, true)
		defs.merge(names)
		complete = complete && ok
	}
	return defs, complete
}

// publishDependentDiagnostics publishes the diagnostics of the other open
// documents that can use the names of uri again: all of them for
// scenarigo.yaml, otherwise the scenarios it includes and those that include
// it, and those it included until this change. Without a workspace a scenario
// has none: the includes are not indexed.
func (s *Server) publishDependentDiagnostics(uri string) {
	open := s.docs.URIs()
	if len(open) == 0 || (len(open) == 1 && open[0] == uri) {
		return
	}
	path := uriToPath(uri)
	rootPath := uriToPath(s.rootURI)
	isConfig := rootPath != "" && path == filepath.Join(rootPath, "scenarigo.yaml")
	related := map[string]bool{}
	if !isConfig {
		edges, _ := s.includeEdges()
		for _, p := range slices.Concat(reachable(edges, path), reachable(invertEdges(edges), path), s.takeDroppedIncludes()) {
			related[p] = true
		}
	}
	for _, other := range open {
		if other != uri && (isConfig || related[uriToPath(other)]) {
			s.publishDiagnostics(other)
		}
	}
}
