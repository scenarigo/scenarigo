package plugin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/spf13/cobra"

	"github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/config"
	"github.com/scenarigo/scenarigo/internal/filepathutil"
	"github.com/scenarigo/scenarigo/internal/plugin/wasm/image"
)

var (
	pushUsername      string
	pushPassword      string
	pushPasswordStdin bool
	pushInsecure      bool
)

func newPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push <plugin-name> <image-ref>",
		Short: "push a WASM plugin to an OCI registry",
		Long: strings.Trim(`
Pushes a WASM plugin to an OCI registry.

The plugin must be built first using "scenarigo plugin build --wasm".

Examples:
  # Push a plugin to a registry
  scenarigo plugin push myplugin.wasm ghcr.io/myorg/myplugin:v1.0.0

  # Push with explicit credentials
  scenarigo plugin push --username=user --password=pass myplugin.wasm ghcr.io/myorg/myplugin:v1.0.0

  # Push with password from stdin
  echo $TOKEN | scenarigo plugin push --username=user --password-stdin myplugin.wasm ghcr.io/myorg/myplugin:v1.0.0

  # Push to an insecure (HTTP) registry
  scenarigo plugin push --insecure myplugin.wasm localhost:5000/myplugin:latest
`, "\n"),
		Args:          cobra.ExactArgs(2),
		RunE:          pushRun,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.Flags().StringVar(&pushUsername, "username", "", "registry username")
	cmd.Flags().StringVar(&pushPassword, "password", "", "registry password")
	cmd.Flags().BoolVar(&pushPasswordStdin, "password-stdin", false, "read password from stdin")
	cmd.Flags().BoolVar(&pushInsecure, "insecure", false, "allow insecure (HTTP) registry")
	return cmd
}

func pushRun(cmd *cobra.Command, args []string) error {
	pluginName := args[0]
	imageRef := args[1]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg == nil {
		return errors.New("config file not found")
	}

	pluginDir := filepathutil.From(cfg.Root, cfg.PluginDirectory)
	pluginPath := filepath.Join(pluginDir, pluginName)

	if _, err := os.Stat(pluginPath); err != nil {
		return fmt.Errorf("plugin not found: %s (run 'scenarigo plugin build --wasm' first)", pluginPath)
	}

	img, err := image.Build(pluginPath)
	if err != nil {
		return fmt.Errorf("failed to build OCI image: %w", err)
	}

	opts, err := buildPushOptions()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	digestRef, err := image.Push(ctx, img, imageRef, opts...)
	if err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pushed: %s\n", digestRef)
	return nil
}

func buildPushOptions() ([]image.PushOption, error) {
	var opts []image.PushOption

	if pushInsecure {
		opts = append(opts, image.WithInsecure(true))
	}

	if pushPasswordStdin {
		if pushPassword != "" {
			return nil, errors.New("--password and --password-stdin are mutually exclusive")
		}
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			pushPassword = scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read password from stdin: %w", err)
		}
	}

	if pushUsername != "" && pushPassword != "" {
		opts = append(opts, image.WithAuth(pushUsername, pushPassword))
	} else if pushUsername != "" || pushPassword != "" {
		return nil, errors.New("both --username and --password must be specified together")
	} else {
		opts = append(opts, image.WithKeychain(authn.DefaultKeychain))
	}

	return opts, nil
}
