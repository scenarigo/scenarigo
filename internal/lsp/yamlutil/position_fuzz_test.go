package yamlutil

import (
	"testing"
)

func FuzzGetCursorContext(f *testing.F) {
	f.Add("schemaVersion: scenario/v1\ntitle: test\nsteps:\n  - title: step1\n    protocol: http\n    ", 5, 4)
	f.Add("schemaVersion: config/v1\n", 1, 0)
	f.Add("", 0, 0)
	f.Add("key: value\n", 0, 5)
	f.Add("a:\n  b:\n    c: d\n", 2, 6)
	f.Add("steps:\n  - include: file.yaml\n", 1, 14)

	f.Fuzz(func(t *testing.T, text string, line, col int) {
		if line < 0 || col < 0 {
			return
		}
		doc := Parse(text)
		if doc == nil {
			return
		}
		doc.GetCursorContext(line, col)
	})
}
