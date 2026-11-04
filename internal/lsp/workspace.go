package lsp

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Workspace folders and files changed on disk.
//
// A client can open several workspace folders, each with its scenarigo.yaml.
// rootURI is the folder of the document being handled: handleMessage points
// it at the folder of the request's document before dispatching, and the
// loops that publish the diagnostics of several documents do so for each
// document. Requests are handled one at a time, so the field is not shared.
//
// Files that change on disk without being open in the editor
// (scenarigo.yaml, included scenarios, plugin sources, proto files) are
// reported by workspace/didChangeWatchedFiles; the server drops what it
// derived from them and publishes the diagnostics of the open documents
// again.

// Extensions of YAML files.
const (
	extYAML = ".yaml"
	extYML  = ".yml"
)

// watchedFilePatterns are the files whose changes the server needs to hear
// about.
var watchedFilePatterns = []string{"**/*.{yaml,yml}", "**/*.go", "**/*.proto"}

// initWorkspace reads the workspace folders and the client capabilities of
// the initialize request.
func (s *Server) initWorkspace(params InitializeParams) {
	s.folders = nil
	for _, f := range params.WorkspaceFolders {
		s.folders = append(s.folders, f.URI)
	}
	if len(s.folders) == 0 && params.RootURI != "" {
		s.folders = []string{params.RootURI}
	}
	s.rootOnly = len(params.WorkspaceFolders) == 0 && params.RootURI != ""
	s.rootURI = ""
	if len(s.folders) > 0 {
		s.rootURI = s.folders[0]
	}
	if w := params.Capabilities.Workspace; w != nil && w.DidChangeWatchedFiles != nil {
		s.registerWatchers = w.DidChangeWatchedFiles.DynamicRegistration
	}
}

// workspaceCapabilities announces the support of several workspace folders.
func workspaceCapabilities() *WorkspaceServerCapabilities {
	return &WorkspaceServerCapabilities{
		WorkspaceFolders: &WorkspaceFoldersServerCapabilities{Supported: true, ChangeNotifications: true},
	}
}

// folderPath returns the local path of a folder URI without a trailing
// separator.
func folderPath(uri string) string {
	if p := uriToPath(uri); p != "" {
		return filepath.Clean(p)
	}
	return ""
}

func pathWithin(path, dir string) bool {
	return dir != "" && (path == dir || strings.HasPrefix(path, dir+string(filepath.Separator)))
}

// containingFolders returns the workspace folders that contain a path,
// innermost first.
func (s *Server) containingFolders(path string) []string {
	var out []string
	for _, f := range s.folders {
		if pathWithin(path, folderPath(f)) {
			out = append(out, f)
		}
	}
	slices.SortStableFunc(out, func(a, b string) int { return len(folderPath(b)) - len(folderPath(a)) })
	return out
}

// folderOf returns the workspace folder whose scenarigo.yaml a document
// uses: the innermost folder that contains the document and has a
// scenarigo.yaml, or else the innermost one. A document outside every
// folder has none, unless the client gave rootUri alone: then the root is
// used, as before workspace folders were supported.
func (s *Server) folderOf(uri string) string {
	containing := s.containingFolders(uriToPath(uri))
	if len(containing) == 0 {
		if s.rootOnly && len(s.folders) == 1 {
			return s.folders[0]
		}
		return ""
	}
	for _, f := range containing {
		config := filepath.Join(folderPath(f), "scenarigo.yaml")
		if s.docs.GetByPath(config) != nil {
			return f
		}
		if _, err := os.Stat(config); err == nil {
			return f
		}
	}
	return containing[0]
}

// includeRootOf returns the directory whose include index covers a path:
// the outermost workspace folder that contains it, so that scenarios of an
// outer folder that include a file of an inner one are found.
func (s *Server) includeRootOf(path string) string {
	containing := s.containingFolders(path)
	if len(containing) > 0 {
		return folderPath(containing[len(containing)-1])
	}
	return folderPath(s.rootURI)
}

// useFolderOf points rootURI at the workspace folder of a document.
func (s *Server) useFolderOf(uri string) {
	s.rootURI = s.folderOf(uri)
}

// useFolderOfRequest points rootURI at the workspace folder of the document
// a textDocument request is about.
func (s *Server) useFolderOfRequest(req *Request) {
	if !strings.HasPrefix(req.Method, "textDocument/") || len(req.Params) == 0 {
		return
	}
	var params struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(req.Params, &params); err == nil && params.TextDocument.URI != "" {
		s.useFolderOf(params.TextDocument.URI)
	}
}

// publishAllDiagnostics publishes the diagnostics of every open document,
// each with its own workspace folder.
func (s *Server) publishAllDiagnostics() {
	for _, uri := range s.docs.URIs() {
		s.useFolderOf(uri)
		s.publishDiagnostics(uri)
	}
}

// handleInitialized registers the file watchers when the client supports
// their dynamic registration. Other clients either watch the workspace on
// their own or never send the notification.
func (s *Server) handleInitialized() {
	s.registerWatcherPatterns()
}

// registerWatcherPatterns registers the file watchers, replacing the
// previous registration: clients such as nvim bind the patterns to the
// folders known when they are registered.
func (s *Server) registerWatcherPatterns() {
	if !s.registerWatchers {
		return
	}
	const method = "workspace/didChangeWatchedFiles"
	if s.watchRegistration != "" {
		s.sendRequest("client/unregisterCapability", UnregistrationParams{
			Unregisterations: []Unregistration{{ID: s.watchRegistration, Method: method}},
		})
	}
	watchers := make([]FileSystemWatcher, 0, len(watchedFilePatterns)+1)
	for _, p := range watchedFilePatterns {
		watchers = append(watchers, FileSystemWatcher{GlobPattern: p})
	}
	// Directories that appear or disappear with the files they hold, which
	// clients report as one event for the directory.
	watchers = append(watchers, FileSystemWatcher{GlobPattern: "**/*", Kind: WatchKindCreate | WatchKindDelete})
	s.watchRegistration = fmt.Sprintf("scenarigo-watched-files-%d", s.requestSeq+1)
	s.sendRequest("client/registerCapability", RegistrationParams{Registrations: []Registration{{
		ID:              s.watchRegistration,
		Method:          method,
		RegisterOptions: DidChangeWatchedFilesRegistrationOptions{Watchers: watchers},
	}}})
}

// sendRequest sends a request to the client. The server does not wait for
// the answer: handleMessage drops answers.
func (s *Server) sendRequest(method string, params any) {
	p, err := json.Marshal(params)
	if err != nil {
		s.logger.Printf("marshal %s params: %v", method, err)
		return
	}
	s.requestSeq++
	id := json.RawMessage(strconv.Quote(fmt.Sprintf("scenarigo-%d", s.requestSeq)))
	s.writeMessage(Request{JSONRPC: "2.0", ID: &id, Method: method, Params: p})
}

func (s *Server) handleDidChangeWatchedFiles(req *Request) {
	var params DidChangeWatchedFilesParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("didChangeWatchedFiles unmarshal error: %v", err)
		return
	}
	for _, change := range params.Changes {
		path := filepath.Clean(uriToPath(change.URI))
		switch filepath.Ext(path) {
		case ".go":
			s.forgetPluginSymbols(filepath.Dir(path))
		case extYAML, extYML:
			s.updateIncludeIndexes(path, change.Type == FileChangeTypeDeleted)
		case ".proto":
			// Republishing below is all a proto file change needs here.
		default:
			// A directory, created, moved or deleted with what it holds; other
			// files that still exist do not matter.
			info, err := os.Stat(path)
			if err == nil && !info.IsDir() {
				continue
			}
			s.forgetPluginSymbols(path)
			s.reindexDirectory(path, err == nil)
		}
	}
	s.publishAllDiagnostics()
}

func (s *Server) handleDidChangeWorkspaceFolders(req *Request) {
	var params DidChangeWorkspaceFoldersParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.logger.Printf("didChangeWorkspaceFolders unmarshal error: %v", err)
		return
	}
	for _, f := range params.Event.Removed {
		s.folders = slices.DeleteFunc(s.folders, func(uri string) bool { return folderPath(uri) == folderPath(f.URI) })
		s.forgetIncludeIndexesCovering(folderPath(f.URI))
	}
	for _, f := range params.Event.Added {
		if !slices.ContainsFunc(s.folders, func(uri string) bool { return folderPath(uri) == folderPath(f.URI) }) {
			s.folders = append(s.folders, f.URI)
		}
	}
	s.rootOnly = false
	if len(params.Event.Added)+len(params.Event.Removed) > 0 {
		s.registerWatcherPatterns()
	}
	s.publishAllDiagnostics()
}

// forgetPluginSymbols drops the cached symbols of the plugin source
// directories that contain dir or that dir contains.
func (s *Server) forgetPluginSymbols(dir string) {
	s.pluginSymbolsMu.Lock()
	defer s.pluginSymbolsMu.Unlock()
	for key := range s.pluginSymbolsCache {
		if k := filepath.Clean(key); pathWithin(dir, k) || pathWithin(k, dir) {
			delete(s.pluginSymbolsCache, key)
		}
	}
}

// updateIncludeIndexes updates the include indexes that cover a YAML file
// changed on disk.
func (s *Server) updateIncludeIndexes(path string, deleted bool) {
	s.includeIndexMu.Lock()
	defer s.includeIndexMu.Unlock()
	for root, idx := range s.includeIndexes {
		if !pathWithin(path, root) {
			continue
		}
		data, err := os.ReadFile(path)
		if deleted || err != nil {
			delete(idx.includes, path)
			continue
		}
		idx.set(path, string(data))
	}
}

// reindexDirectory updates the include indexes for a directory that
// appeared (exists) or a path that is gone: the entries under it are
// dropped, and the YAML files of an existing directory are read again.
func (s *Server) reindexDirectory(dir string, exists bool) {
	s.includeIndexMu.Lock()
	defer s.includeIndexMu.Unlock()
	for root, idx := range s.includeIndexes {
		if !pathWithin(dir, root) {
			if pathWithin(root, dir) {
				delete(s.includeIndexes, root) // the folder itself moved
			}
			continue
		}
		for p := range idx.includes {
			if pathWithin(p, dir) {
				delete(idx.includes, p)
			}
		}
		if exists {
			sub := s.buildIncludeIndex(dir)
			maps.Copy(idx.includes, sub.includes)
		}
	}
}

// forgetIncludeIndexesCovering drops the include indexes that cover a path
// or lie under it; they are built again when needed.
func (s *Server) forgetIncludeIndexesCovering(path string) {
	s.includeIndexMu.Lock()
	defer s.includeIndexMu.Unlock()
	for root := range s.includeIndexes {
		if pathWithin(path, root) || pathWithin(root, path) {
			delete(s.includeIndexes, root)
		}
	}
}
