package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

type Config struct {
	SchemaVersion   string                  `yaml:"schemaVersion"`
	PluginDirectory string                  `yaml:"pluginDirectory"`
	Plugins         map[string]PluginConfig `yaml:"plugins"`
}

type PluginConfig struct {
	Src string `yaml:"src"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <plugins-dir>\n", os.Args[0])
		os.Exit(1)
	}

	pluginsDir := os.Args[1]
	genPluginsDir := filepath.Join(filepath.Dir(pluginsDir), "gen", "plugins")

	if err := os.MkdirAll(genPluginsDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create directory %s: %v\n", genPluginsDir, err)
		os.Exit(1)
	}

	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read plugins directory %s: %v\n", pluginsDir, err)
		os.Exit(1)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginName := entry.Name()
		pluginSrcDir := filepath.Join(pluginsDir, pluginName)

		// Check if main.go exists in this plugin directory
		if _, err := os.Stat(filepath.Join(pluginSrcDir, "main.go")); os.IsNotExist(err) {
			continue
		}

		if err := buildWasmPlugin(pluginName, pluginSrcDir, genPluginsDir); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to build WASM plugin %s: %v\n", pluginName, err)
			os.Exit(1)
		}

		fmt.Printf("Built WASM plugin: %s.wasm\n", pluginName)
	}
}

func buildWasmPlugin(pluginName, srcDir, genDir string) error {
	tempDir, err := os.MkdirTemp("", "scenarigo-plugin-build-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Copy plugin source to temp directory
	tempPluginSrcDir := filepath.Join(tempDir, "plugin", "src")
	if err := copyDir(srcDir, tempPluginSrcDir); err != nil {
		return fmt.Errorf("failed to copy source files: %w", err)
	}

	// Create scenarigo.yaml config
	config := Config{
		SchemaVersion:   "config/v1",
		PluginDirectory: "./plugin/gen",
		Plugins: map[string]PluginConfig{
			pluginName + ".wasm": {
				Src: "./plugin/src",
			},
		},
	}

	configData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := filepath.Join(tempDir, "scenarigo.yaml")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Create go.mod for plugin build
	if err := createGoMod(tempDir); err != nil {
		return fmt.Errorf("failed to create go.mod: %w", err)
	}

	// Get repository root for running scenarigo command
	// The script is executed from the repository root, so current directory is the repo root
	repoRoot, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("failed to get repository root: %w", err)
	}

	// Run scenarigo plugin build from repository root, but with config in tempDir
	cmd := exec.Command("go", "run", "./cmd/scenarigo", "plugin", "build", "--wasm", "-c", filepath.Join(tempDir, "scenarigo.yaml"))
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run scenarigo plugin build: %w", err)
	}

	// Copy generated WASM file to target directory
	generatedWasmPath := filepath.Join(tempDir, "plugin", "gen", pluginName+".wasm")
	targetWasmPath := filepath.Join(genDir, pluginName+".wasm")

	return copyFile(generatedWasmPath, targetWasmPath)
}

func createGoMod(tempDir string) error {
	// Get the absolute path to repository root
	// The script is executed from the repository root, so current directory is the repo root
	repoRoot, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("failed to get repository root: %w", err)
	}

	// Initialize go module in plugin/src directory
	pluginSrcDir := filepath.Join(tempDir, "plugin", "src")
	cmd := exec.Command("go", "mod", "init", "temp-plugin-build")
	cmd.Dir = pluginSrcDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run go mod init: %w", err)
	}

	// Use absolute path instead of relative path for replace directive
	// This should be more reliable than relative paths
	cmd = exec.Command("go", "mod", "edit", "-replace", "github.com/scenarigo/scenarigo="+repoRoot)
	cmd.Dir = pluginSrcDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add replace directive: %w", err)
	}

	// Add a simple dependency first to test if go.mod works
	cmd = exec.Command("go", "mod", "tidy")
	cmd.Dir = pluginSrcDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run go mod tidy: %w", err)
	}

	return nil
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = dstFile.ReadFrom(srcFile)
	return err
}
