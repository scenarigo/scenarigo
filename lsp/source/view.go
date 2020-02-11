package source

import (
	"context"
	"sync"

	"github.com/go-language-server/uri"
)

// View represents a single workspace.
type View interface {
	Name() string
	Folder() string

	// GetFile returns the file object for a given URI.
	GetFile(ctx context.Context, uri uri.URI) (File, error)
}

type view struct {
	session *session
	id      int64
	name    string
	folder  string

	fileMu sync.Mutex
	files  map[uri.URI]File
}

func (v *view) Name() string {
	return v.name
}

func (v *view) Folder() string {
	return v.folder
}

func (v *view) GetFile(ctx context.Context, uri uri.URI) (File, error) {
	v.fileMu.Lock()
	defer v.fileMu.Unlock()

	// if f, found := v.files[uri]; found {
	// 	return f, nil
	// }

	if f, found := v.session.overlays[uri]; found {
		return f, nil
	}

	panic("todo")
}
