package lsp

import (
	"context"

	"github.com/go-language-server/protocol"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func (s *Server) completion(ctx context.Context, params *protocol.CompletionParams) (*protocol.CompletionList, error) {
	s.logger.With(zap.Any("params", params)).Info("completion")
	v := s.session.ViewOf(params.TextDocument.URI)
	s.logger.Info(v.Folder())
	f, err := v.GetFile(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, errors.Wrap(err, "file not found")
	}
	str, err := f.Line(int(params.Position.Line))
	if err != nil {
		return nil, errors.Wrap(err, "file not found")
	}
	s.logger.With(zap.Float64("line", params.Position.Line), zap.String("====> test", str)).Info("completion")
	items := []protocol.CompletionItem{}
	if compl, found := keyCompl[str]; found {
		items = compl.items
	}
	return &protocol.CompletionList{
		Items: items}, nil
}

var keyCompl = map[string]struct {
	items []protocol.CompletionItem
}{
	"": {
		items: []protocol.CompletionItem{
			protocol.CompletionItem{
				Label:  "title",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "description",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "plugins",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "vars",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "steps",
				Detail: "key",
			},
		},
	},
	"  - ": {
		items: []protocol.CompletionItem{
			protocol.CompletionItem{
				Label:  "title",
				Detail: "key",
			},
		},
	},
	"    ": {
		items: []protocol.CompletionItem{
			protocol.CompletionItem{
				Label:  "title",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "description",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "vars",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "protocol",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "request",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "expect",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "include",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "ref",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "bind",
				Detail: "key",
			},
		},
	},
	"    protocol: ": {
		items: []protocol.CompletionItem{
			protocol.CompletionItem{
				Label: "http",
				Kind:  13,
			},
			protocol.CompletionItem{
				Label: "grpc",
				Kind:  13,
			},
		},
	},
	"      ": {
		items: []protocol.CompletionItem{
			protocol.CompletionItem{
				Label:  "client",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "method",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "url",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "header",
				Detail: "key",
			},
			protocol.CompletionItem{
				Label:  "body",
				Detail: "key",
			},
		},
	},
	"      method: ": {
		items: []protocol.CompletionItem{
			protocol.CompletionItem{
				Label: "GET",
				Kind:  13,
			},
			protocol.CompletionItem{
				Label: "HEAD",
				Kind:  13,
			},
			protocol.CompletionItem{
				Label: "POST",
				Kind:  13,
			},
			protocol.CompletionItem{
				Label: "PUT",
				Kind:  13,
			},
			protocol.CompletionItem{
				Label: "PATCH",
				Kind:  13,
			},
			protocol.CompletionItem{
				Label: "DELETE",
				Kind:  13,
			},
			protocol.CompletionItem{
				Label: "CONNECT",
				Kind:  13,
			},
			protocol.CompletionItem{
				Label: "OPTIONS",
				Kind:  13,
			},
			protocol.CompletionItem{
				Label: "TRACE",
				Kind:  13,
			},
		},
	},
}
