package cmd

import "testing"

func TestList(t *testing.T) {
	verboseList = true
	if err := list(nil, []string{"testdata"}); err != nil {
		t.Fatal(err)
	}
}
