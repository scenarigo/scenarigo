package reporter

import (
	"fmt"
	"testing"
)

func TestMatcher_Match(t *testing.T) {
	tests := map[string]struct {
		run    string
		parent string
		name   string
		ok     []string
		ng     []string
	}{
		"empty -run": {
			run:    "",
			parent: "Test",
			name:   "foo/bar",
			ok:     []string{"hoge", "fuga"}, // match all
		},
		"-run is a part of the test name": {
			run:    "Test/foo/bar",
			parent: "Test",
			name:   "foo/bar/baz",
			ok:     []string{"hoge"}, // match all
		},
		"-run is the same as test name": {
			run:    "Test/foo/bar",
			parent: "Test",
			name:   "foo/bar",
			ok:     []string{"hoge"}, // match all
		},
		"-run includes the test name": {
			run:    "Test/foo/bar",
			parent: "Test",
			name:   "foo",
			ok:     []string{"bar", "baz"},
			ng:     []string{"hoge"},
		},
		"name contains /": {
			run:    "Test/foo/bar/baz",
			parent: "Test",
			name:   "foo",
			ok:     []string{"bar/baz"},
			ng:     []string{"bar/hoge"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := newMatcher(test.run)
			if err != nil {
				t.Fatalf("failed to create matcher: %s", err)
			}
			if test.ok != nil {
				if !m.match(test.parent, test.name) {
					t.Fatalf("%s does not match", test.name)
				}
				parent := fmt.Sprintf("%s/%s", test.parent, test.name)
				for _, name := range test.ok {
					if !m.match(parent, name) {
						t.Fatalf("%s does not match", name)
					}
					parent = fmt.Sprintf("%s/%s", parent, name)
				}
			}
			if test.ng != nil {
				parent := fmt.Sprintf("%s/%s", test.parent, test.name)
				var fail bool
				for _, name := range test.ng {
					if !m.match(parent, name) {
						fail = true
						break
					}
					parent = fmt.Sprintf("%s/%s", parent, name)
				}
				if !fail {
					t.Errorf("%s match", parent)
				}
			}
		})
	}

	t.Run("compile error", func(t *testing.T) {
		if _, err := newMatcher("TestFoo/[a-z"); err == nil {
			t.Fatal("no error")
		}
	})
}
