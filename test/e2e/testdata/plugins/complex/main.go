package main

import (
	"errors"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/scenarigo/scenarigo/context"
	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/schema"
)

var (
	DumpVarsStep                      = plugin.StepFunc(dumpVarsStep)
	Join         plugin.LeftArrowFunc = &join{}
)

func dumpVarsStep(ctx *context.Context, step *schema.Step) *context.Context {
	b, err := yaml.Marshal(step.Vars)
	if err != nil {
		ctx.Reporter().Errorf("failed to marshal vars: %s", err)
	}
	ctx.Reporter().Log(string(b))
	return ctx
}

type join struct{}

type joinArg struct {
	Prefix string `yaml:"prefix"`
	Text   string `yaml:"text"`
	Suffix string `yaml:"suffix"`
}

func (_ *join) Exec(in interface{}) (interface{}, error) {
	arg, ok := in.(*joinArg)
	if !ok {
		return nil, errors.New("arg must be a joinArg")
	}
	return arg.Prefix + arg.Text + arg.Suffix, nil
}

func (_ *join) UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error) {
	var arg joinArg
	if err := unmarshal(&arg); err != nil {
		return nil, err
	}
	return &arg, nil
}

func Sleep(s string) (plugin.Step, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return nil, err
	}
	return plugin.StepFunc(func(ctx *context.Context, step *schema.Step) *context.Context {
		time.Sleep(d)
		ctx.Reporter().FailNow()
		return ctx
	}), nil
}

func SetVar(k string, v interface{}) (plugin.Step, error) {
	return plugin.StepFunc(func(ctx *context.Context, step *schema.Step) *context.Context {
		return ctx.WithVars(map[string]interface{}{k: v})
	}), nil
}
