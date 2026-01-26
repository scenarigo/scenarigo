package plugin

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"

	"github.com/scenarigo/scenarigo/cmd/scenarigo/cmd/config"
	"github.com/scenarigo/scenarigo/internal/plugin/wasm/image"
)

var testWASMBytes = []byte{
	0x00, 0x61, 0x73, 0x6d, // WASM magic number
	0x01, 0x00, 0x00, 0x00, // WASM version 1
}

func TestPushCommand(t *testing.T) {
	// Start in-memory registry
	reg := registry.New()
	server := httptest.NewServer(reg)
	defer server.Close()

	// Create a temporary directory with config and plugin
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "gen")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("failed to create plugin directory: %v", err)
	}

	// Write config file (no plugins section needed, we just need pluginDirectory)
	configContent := `schemaVersion: config/v1
pluginDirectory: ./gen
`
	if err := os.WriteFile(filepath.Join(tmpDir, "scenarigo.yaml"), []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Write test WASM plugin
	pluginPath := filepath.Join(pluginDir, "test.wasm")
	if err := os.WriteFile(pluginPath, testWASMBytes, 0o644); err != nil {
		t.Fatalf("failed to write test plugin: %v", err)
	}

	// Change to temp directory and reset config globals
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	origConfigPath := config.ConfigPath
	origRoot := config.Root
	defer func() {
		os.Chdir(origDir)
		config.ConfigPath = origConfigPath
		config.Root = origRoot
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	config.ConfigPath = ""
	config.Root = ""

	// Create push command
	cmd := newPushCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	// Set insecure flag for local registry
	pushInsecure = true
	defer func() { pushInsecure = false }()

	// Run push command
	ref := strings.TrimPrefix(server.URL, "http://") + "/test/plugin:v1"
	cmd.SetArgs([]string{"test.wasm", ref})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("push command failed: %v", err)
	}

	// Verify output contains "Pushed:"
	if !strings.Contains(stdout.String(), "Pushed:") {
		t.Errorf("expected output to contain 'Pushed:', got: %s", stdout.String())
	}

	// Verify the image was pushed by pulling it
	ctx := context.Background()
	pulledImg, err := image.Pull(ctx, ref, image.WithPullInsecure(true))
	if err != nil {
		t.Fatalf("failed to pull pushed image: %v", err)
	}

	extractedWASM, err := image.ExtractWASM(pulledImg)
	if err != nil {
		t.Fatalf("failed to extract WASM from pulled image: %v", err)
	}

	if !bytes.Equal(extractedWASM, testWASMBytes) {
		t.Errorf("extracted WASM bytes don't match: got %v, want %v", extractedWASM, testWASMBytes)
	}
}

func TestPushCommand_PluginNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "gen")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("failed to create plugin directory: %v", err)
	}

	// Write config file without creating the plugin
	configContent := `schemaVersion: config/v1
pluginDirectory: ./gen
`
	if err := os.WriteFile(filepath.Join(tmpDir, "scenarigo.yaml"), []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Change to temp directory and reset config globals
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	origConfigPath := config.ConfigPath
	origRoot := config.Root
	defer func() {
		os.Chdir(origDir)
		config.ConfigPath = origConfigPath
		config.Root = origRoot
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	config.ConfigPath = ""
	config.Root = ""

	// Create push command
	cmd := newPushCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	// Run push command with non-existent plugin
	cmd.SetArgs([]string{"nonexistent.wasm", "ghcr.io/test/plugin:v1"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent plugin")
	}
	if !strings.Contains(err.Error(), "plugin not found") {
		t.Errorf("expected 'plugin not found' error, got: %v", err)
	}
}

func TestPushCommand_InvalidArgs(t *testing.T) {
	cmd := newPushCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	// Test with no arguments
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing arguments")
	}

	// Test with only one argument
	cmd.SetArgs([]string{"plugin.wasm"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing image ref argument")
	}
}

func TestBuildPushOptions(t *testing.T) {
	tests := []struct {
		name          string
		username      string
		password      string
		passwordStdin bool
		insecure      bool
		wantErr       bool
	}{
		{
			name:     "default options with keychain",
			wantErr:  false,
		},
		{
			name:     "with username and password",
			username: "user",
			password: "pass",
			wantErr:  false,
		},
		{
			name:     "with username only",
			username: "user",
			wantErr:  true,
		},
		{
			name:     "with password only",
			password: "pass",
			wantErr:  true,
		},
		{
			name:     "with insecure",
			insecure: true,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global flags
			pushUsername = tt.username
			pushPassword = tt.password
			pushPasswordStdin = tt.passwordStdin
			pushInsecure = tt.insecure
			defer func() {
				pushUsername = ""
				pushPassword = ""
				pushPasswordStdin = false
				pushInsecure = false
			}()

			opts, err := buildPushOptions()
			if (err != nil) != tt.wantErr {
				t.Errorf("buildPushOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && opts == nil {
				t.Error("buildPushOptions() returned nil options")
			}
		})
	}
}

func TestBuildPushOptions_MutuallyExclusive(t *testing.T) {
	pushPassword = "existing"
	pushPasswordStdin = true
	defer func() {
		pushPassword = ""
		pushPasswordStdin = false
	}()

	_, err := buildPushOptions()
	if err == nil {
		t.Fatal("expected error for mutually exclusive flags")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' error, got: %v", err)
	}
}
