package cmd

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"

	"github.com/scenarigo/scenarigo/version"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/cobra"
)

func TestVersion(t *testing.T) {
	var b bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&b)
	printVersion(cmd, nil)
	if got, expect := b.String(), fmt.Sprintf("%s version %s %s %s/%s\n", appName, version.String(), runtime.Version(), runtime.GOOS, runtime.GOARCH); got != expect {
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(expect, got, false)
		t.Errorf("output differs:\n%s", dmp.DiffPrettyText(diffs))
	}
}
