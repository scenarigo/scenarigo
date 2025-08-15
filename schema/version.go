package schema

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/scenarigo/scenarigo/errors"
)

var scehamaVersionPath *yaml.Path

func init() {
	p, err := yaml.PathString("$.schemaVersion")
	if err != nil {
		panic(fmt.Sprintf("YAML parser error: %s", err))
	}
	scehamaVersionPath = p
}

type docWithSchemaVersion struct {
	schemaVersion string
	doc           *ast.DocumentNode
}

func readDocsWithSchemaVersion(path string) ([]*docWithSchemaVersion, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return readDocsWithSchemaVersionFromBytes(b, nil)
}

func readDocsWithSchemaVersionFromBytes(b []byte, opt *loadOption) ([]*docWithSchemaVersion, error) {
	f, err := parser.ParseBytes(b, 1)
	if err != nil {
		return nil, err
	}
	if len(f.Docs) == 0 {
		return nil, nil
	}
	if len(f.Docs) == 1 && f.Docs[0].Body == nil {
		// YAML interprets an empty string as null, so if docs body is nil, it should be treated as empty content.
		return nil, nil
	}

	docs := []*docWithSchemaVersion{}
	for _, doc := range f.Docs {
		vnode, err := scehamaVersionPath.FilterNode(doc.Body)
		if err != nil {
			return nil, errors.WithNodeAndColored(
				errors.WithPath(err, "schemaVersion"),
				doc.Body,
				opt.colorConfig.IsEnabled(),
			)
		}

		var v string
		if vnode != nil {
			if err := yaml.NodeToValue(vnode, &v); err != nil {
				return nil, fmt.Errorf("invalid version: %w", err)
			}
		}

		docs = append(docs, &docWithSchemaVersion{
			schemaVersion: v,
			doc:           doc,
		})
	}

	return docs, nil
}
