package lsp

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestURIToPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX paths")
	}
	tests := map[string]struct {
		uri  string
		want string
	}{
		"plain":           {uri: "file:///a/b.yaml", want: "/a/b.yaml"},
		"space":           {uri: "file:///a/my%20project/b.yaml", want: "/a/my project/b.yaml"},
		"multibyte lower": {uri: "file:///a/%e6%97%a5%e6%9c%ac/b.yaml", want: "/a/日本/b.yaml"},
		"multibyte upper": {uri: "file:///a/%E6%97%A5%E6%9C%AC/b.yaml", want: "/a/日本/b.yaml"},
		"raw multibyte":   {uri: "file:///a/日本/b.yaml", want: "/a/日本/b.yaml"},
		"not a file uri":  {uri: "untitled:Untitled-1", want: ""},
		"invalid":         {uri: "file://%zz", want: ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := uriToPath(tt.uri); got != tt.want {
				t.Errorf("uriToPath(%q) = %q, want %q", tt.uri, got, tt.want)
			}
		})
	}
}

func TestPathToURI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX paths")
	}
	tests := map[string]struct {
		path string
		want string
	}{
		"plain":     {path: "/a/b.yaml", want: "file:///a/b.yaml"},
		"space":     {path: "/a/my project/b.yaml", want: "file:///a/my%20project/b.yaml"},
		"multibyte": {path: "/a/日本/b.yaml", want: "file:///a/%E6%97%A5%E6%9C%AC/b.yaml"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := pathToURI(tt.path)
			if got != tt.want {
				t.Errorf("pathToURI(%q) = %q, want %q", tt.path, got, tt.want)
			}
			if back := uriToPath(got); back != tt.path {
				t.Errorf("uriToPath(pathToURI(%q)) = %q", tt.path, back)
			}
		})
	}
}

func TestPathToURI_Relative(t *testing.T) {
	abs, err := filepath.Abs("rel.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := pathToURI("rel.yaml"), pathToURI(abs); got != want {
		t.Errorf("pathToURI(rel) = %q, want %q", got, want)
	}
}
