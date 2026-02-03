package main

import (
	"fmt"

	"github.com/scenarigo/scenarigo/plugin"
	"github.com/scenarigo/scenarigo/schema"
)

var Nop = plugin.StepFunc(func(ctx *plugin.Context, step *schema.Step) *plugin.Context {
	return ctx
})

func Print(message string) plugin.StepFunc {
	return func(ctx *plugin.Context, step *schema.Step) *plugin.Context {
		fmt.Println(message)
		return ctx
	}
}

// UnaryFun demonstrates the bug where nil is passed as *plugin.Context instead of nil
func UnaryFun(arg any) any {
	fmt.Printf("  arg == nil: %v\n", arg == nil)
	fmt.Printf("  type: %T\n", arg)

	if _, ok := arg.(*plugin.Context); ok {
		fmt.Println("  🐛 BUG: received *plugin.Context instead of nil")
	}
	fmt.Println()

	return true
}

// BinaryFun demonstrates the bug with two arguments
func BinaryFun(arg1 any, arg2 any) any {
	fmt.Printf("  arg1 == nil: %v, type: %T\n", arg1 == nil, arg1)
	if _, ok := arg1.(*plugin.Context); ok {
		fmt.Println("  🐛 BUG: arg1 received *plugin.Context instead of nil")
	}

	fmt.Printf("  arg2 == nil: %v, type: %T\n", arg2 == nil, arg2)
	if _, ok := arg2.(*plugin.Context); ok {
		fmt.Println("  🐛 BUG: arg2 received *plugin.Context instead of nil")
	}
	fmt.Println()

	return true
}
