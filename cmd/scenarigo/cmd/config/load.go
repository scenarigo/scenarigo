package config

import (
	"os"
	"path/filepath"

	"github.com/scenarigo/scenarigo/schema"
)

var (
	// ConfigPath is the configuration file path. Set by the root command.
	ConfigPath string
	Root       string
)

// Load loads configuration.
func Load() (*schema.Config, error) {
	root := Root
	var err error
	if root != "" {
		root, err = filepath.Abs(Root)
		if err != nil {
			return nil, err
		}
	}

	if ConfigPath == "-" {
		if root == "" {
			root, err = os.Getwd()
			if err != nil {
				return nil, err
			}
		}
		return schema.LoadConfigFromReader(os.Stdin, root)
	}

	c, err := load(root)
	if err != nil {
		if ConfigPath == "" && os.IsNotExist(err) {
			return nil, nil //nolint:nilnil
		}
		return nil, err
	}
	return c, nil
}

func load(root string) (*schema.Config, error) {
	path := ConfigPath
	if path == "" {
		path = DefaultConfigFileName
	}

	if root == "" {
		return schema.LoadConfig(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return schema.LoadConfigFromReader(f, root)
}
