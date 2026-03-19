package schema

import "testing"

func TestReadDocWithSchemaVersions(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tests := map[string]struct {
			path   string
			expect []*docWithSchemaVersion
		}{
			"no schemaVersion": {
				path: "testdata/valid.yaml",
				expect: []*docWithSchemaVersion{
					{},
				},
			},
			"has schemaVersion": {
				path: "testdata/config/valid.yaml",
				expect: []*docWithSchemaVersion{
					{
						schemaVersion: "config/v1",
					},
				},
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				docs, err := readDocsWithSchemaVersion(test.path)
				if err != nil {
					t.Fatal(err)
				}
				if got, expect := len(docs), len(test.expect); got != expect {
					t.Errorf("expect %d but got %d", expect, got)
				}
				for i, doc := range docs {
					if got, expect := doc.schemaVersion, test.expect[i].schemaVersion; got != expect {
						t.Errorf("expect %q but got %q", expect, got)
					}
				}
			})
		}
	})
	t.Run("failure", func(t *testing.T) {
		tests := map[string]struct {
			path   string
			expect string
		}{
			"not found": {
				path:   "testdata/not-found.yaml",
				expect: "open testdata/not-found.yaml: no such file or directory",
			},
			"broken YAML": {
				path: "testdata/broken.yaml",
				expect: `[1:1] unexpected key name
>  1 | :
       ^
`,
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				_, err := readDocsWithSchemaVersion(test.path)
				if err == nil {
					t.Fatal("no error")
				}
				if got, expect := err.Error(), test.expect; got != expect {
					t.Errorf("expect %q but got %q", expect, got)
				}
			})
		}
	})
}
