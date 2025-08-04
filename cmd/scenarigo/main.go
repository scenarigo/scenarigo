package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"

	"github.com/scenarigo/scenarigo/cmd/scenarigo/cmd"
)

func main() {
	fmt.Println("color.NoColor", color.NoColor)
	fmt.Println("TERM", os.Getenv("TERM"))
	fmt.Println("isatty.IsTerminal(os.Stdout.Fd())", isatty.IsTerminal(os.Stdout.Fd()))
	fmt.Println("isatty.IsCygwinTerminal(os.Stdout.Fd())", isatty.IsCygwinTerminal(os.Stdout.Fd()))
	if err := run(); err != nil {
		if errors.Is(err, cmd.ErrTestFailed) {
			os.Exit(10)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return cmd.Execute(ctx)
}
