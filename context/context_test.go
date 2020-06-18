package context

import (
	"testing"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"
)

func TestContext(t *testing.T) {
	t.Run("node", func(t *testing.T) {
		ctx := FromT(t)
		node := ast.String(token.String("", "", nil))
		ctx = ctx.WithNode(node)
		if ctx.Node() != node {
			t.Fatal("failed to get node")
		}
	})
	t.Run("enabledColor", func(t *testing.T) {
		ctx := FromT(t)
		ctx = ctx.WithEnabledColor(true)
		if !ctx.EnabledColor() {
			t.Fatal("failed to get enabledColor")
		}
	})
	t.Run("dryRun", func(t *testing.T) {
		ctx := FromT(t)
		ctx = ctx.WithDryRun(true)
		if !ctx.DryRun() {
			t.Fatal("failed to get dryRun")
		}
	})

}
