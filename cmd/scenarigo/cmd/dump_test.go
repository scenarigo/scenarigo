package cmd

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/config"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/cobra"
)

func TestDump(t *testing.T) {
	tests := map[string]struct {
		args   []string
		expect string
	}{
		"no args": {
			expect: `title: /echo
steps:
- title: POST /echo
  vars:
    message: hello
  protocol: http
  request:
    method: POST
    url: "{{env.TEST_ADDR}}/echo"
    body:
      message: "{{vars.message}}"
  expect:
    code: "200"
    body:
      message: "{{request.body.message}}"
---
title: /echo
steps:
- title: POST /echo
  vars:
    message: hello
  protocol: http
  request:
    method: POST
    url: "{{env.TEST_ADDR}}/echo"
    body:
      message: hello ytt
  expect:
    code: "200"
    body:
      message: "{{request.body.message}}"
`,
		},
		"with args": {
			args: []string{filepath.Join("testdata", "scenarios", "ytt.yaml")},
			expect: `title: /echo
steps:
- title: POST /echo
  vars:
    message: hello
  protocol: http
  request:
    method: POST
    url: "{{env.TEST_ADDR}}/echo"
    body:
      message: hello ytt
  expect:
    code: "200"
    body:
      message: "{{request.body.message}}"
`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			config.ConfigPath = filepath.Join("testdata", "scenarigo-ytt.yaml")
			if err := dump(cmd, test.args); err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != test.expect {
				dmp := diffmatchpatch.New()
				diffs := dmp.DiffMain(test.expect, got, false)
				t.Errorf("stdout differs:\n%s", dmp.DiffPrettyText(diffs))
			}
		})
	}
}
