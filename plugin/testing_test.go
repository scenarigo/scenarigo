package plugin

import "testing"

func TestSetup(t *testing.T) {
	t.Run("no setup", func(t *testing.T) {
		newPlugin = &openedPlugin{}
		Setup(t)
	})
	t.Run("setup without teardown", func(t *testing.T) {
		newPlugin = &openedPlugin{}
		var setup bool
		RegisterSetup(func(ctx *Context) (*Context, func(*Context)) {
			setup = true
			return ctx, nil
		})
		Setup(t)
		if !setup {
			t.Error("shouuld call setup")
		}
	})
	t.Run("setup with teardown", func(t *testing.T) {
		newPlugin = &openedPlugin{}
		var (
			setup    bool
			teardown bool
		)
		t.Cleanup(func() {
			if !teardown {
				t.Error("should call teardown")
			}
		})
		RegisterSetup(func(ctx *Context) (*Context, func(*Context)) {
			setup = true
			return ctx, func(ctx *Context) {
				teardown = true
			}
		})
		Setup(t)
		if !setup {
			t.Error("should call setup")
		}
		if teardown {
			t.Error("shouldn't call teardown")
		}
	})
}

func TestSetupEachScenario(t *testing.T) {
	t.Run("no setup", func(t *testing.T) {
		newPlugin = &openedPlugin{}
		SetupEachScenario(t)
	})
	t.Run("setup without teardown", func(t *testing.T) {
		newPlugin = &openedPlugin{}
		var setup bool
		RegisterSetupEachScenario(func(ctx *Context) (*Context, func(*Context)) {
			setup = true
			return ctx, nil
		})
		SetupEachScenario(t)
		if !setup {
			t.Error("shouuld call setup")
		}
	})
	t.Run("setup with teardown", func(t *testing.T) {
		newPlugin = &openedPlugin{}
		var (
			setup    bool
			teardown bool
		)
		t.Cleanup(func() {
			if !teardown {
				t.Error("should call teardown")
			}
		})
		RegisterSetupEachScenario(func(ctx *Context) (*Context, func(*Context)) {
			setup = true
			return ctx, func(ctx *Context) {
				teardown = true
			}
		})
		SetupEachScenario(t)
		if !setup {
			t.Error("should call setup")
		}
		if teardown {
			t.Error("shouldn't call teardown")
		}
	})
}
