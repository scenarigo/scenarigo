package queryutil

import (
	"context"
	"errors"

	query "github.com/zoncoen/query-go/v2"
)

// ExtractFirst extracts q from the targets in order and returns the first
// value found. A target that does not have the value passes the lookup on to
// the next one, while a failure to look it up is returned as is.
func ExtractFirst(ctx context.Context, q *query.Query, targets ...any) (any, error) {
	notFound := query.ErrNotFound
	for _, target := range targets {
		v, err := q.Extract(ctx, target)
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, query.ErrNotFound) {
			return nil, err
		}
		// Keep the last absence rather than the bare sentinel: it names the
		// query and where it stopped, which a caller that reports the error to
		// the user needs.
		notFound = err
	}
	return nil, notFound
}
