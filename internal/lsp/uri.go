package lsp

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// uriToPath converts a file URI sent by the client into a local file path.
// Clients percent-encode URIs (spaces, non-ASCII characters), so the path is
// unescaped here. It returns "" for URIs that are not file URIs.
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	path := u.Path
	if runtime.GOOS == "windows" {
		// file:///C:/dir/file.yaml parses as "/C:/dir/file.yaml".
		if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		path = filepath.FromSlash(path)
	}
	return path
}

// pathToURI converts a local file path into a percent-encoded file URI.
func pathToURI(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path // Windows drive paths.
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}
