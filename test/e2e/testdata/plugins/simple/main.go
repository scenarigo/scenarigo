package main

import (
	"io"

	"github.com/goccy/go-yaml"

	"github.com/zoncoen/scenarigo/context"
	"github.com/zoncoen/scenarigo/plugin"
	"github.com/zoncoen/scenarigo/schema"
)

var (
	String       = "string"
	Pointer      = &String
	Interface    = io.Reader(nil)
	DumpVarsStep = plugin.StepFunc(dumpVarsStep)
)

func Function() string { return "function" }

func dumpVarsStep(ctx *context.Context, step *schema.Step) *context.Context {
	b, err := yaml.Marshal(step.Vars)
	if err != nil {
		ctx.Reporter().Errorf("failed to marshal vars: %s", err)
	}
	ctx.Reporter().Log(string(b))
	return ctx
}
