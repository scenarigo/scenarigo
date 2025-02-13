package schema

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"golang.org/x/mod/module"

	"github.com/scenarigo/scenarigo/errors"
	"github.com/scenarigo/scenarigo/internal/filepathutil"
	"github.com/scenarigo/scenarigo/protocol"
)

// Config represents a configuration.
type Config struct {
	SchemaVersion   string                           `yaml:"schemaVersion,omitempty"`
	Vars            map[string]any                   `yaml:"vars,omitempty"`
	Secrets         map[string]any                   `yaml:"secrets,omitempty"`
	Scenarios       []string                         `yaml:"scenarios,omitempty"`
	PluginDirectory string                           `yaml:"pluginDirectory,omitempty"`
	Plugins         OrderedMap[string, PluginConfig] `yaml:"plugins,omitempty"`
	Protocols       ProtocolOptions                  `yaml:"protocols,omitempty"`
	Input           InputConfig                      `yaml:"input,omitempty"`
	Output          OutputConfig                     `yaml:"output,omitempty"`

	// absolute path to the configuration file
	Root     string          `yaml:"-"`
	Comments yaml.CommentMap `yaml:"-"`
	Node     ast.Node        `yaml:"-"`
}

// PluginConfig represents a plugin configuration.
type PluginConfig struct {
	Src string `yaml:"src,omitempty"`
}

// ProtocolOptions represents options for each protocol.
type ProtocolOptions OrderedMap[string, any]

// IsZero implements yaml.IsZeroer interface.
func (opts ProtocolOptions) IsZero() bool {
	return (OrderedMap[string, any])(opts).IsZero()
}

// MarshalYAML implements yaml.BytesMarshaler interface.
func (opts ProtocolOptions) MarshalYAML() ([]byte, error) {
	return (OrderedMap[string, any])(opts).MarshalYAML()
}

// UnmarshalYAML implements yaml.BytesUnmarshaler interface.
func (opts *ProtocolOptions) UnmarshalYAML(b []byte) error {
	in := NewOrderedMap[string, RawMessage]()
	if err := yaml.Unmarshal(b, &in); err != nil {
		return err
	}
	out := NewOrderedMap[string, any]()
	for _, o := range in.ToSlice() {
		out.Set(o.Key, o.Value)
	}
	*opts = ProtocolOptions(out)
	return nil
}

// Set sets protocol options.
func (opts ProtocolOptions) Set() error {
	for _, o := range (OrderedMap[string, any])(opts).ToSlice() {
		p := protocol.Get(o.Key)
		errPath := fmt.Sprintf("protocols.%s", o.Key)
		if p == nil {
			return errors.ErrorPathf(errPath, "unknown protocol: %s", o.Key)
		}
		var b []byte
		if v, ok := o.Value.(RawMessage); ok {
			b = v
		} else {
			v, err := yaml.Marshal(o.Value)
			if err != nil {
				return errors.WrapPath(err, errPath, "failed to marshal YAML")
			}
			b = v
		}
		if err := p.UnmarshalOption(b); err != nil {
			return errors.WrapPath(err, errPath, "failed to unmarshal YAML")
		}
	}
	return nil
}

// InputConfig represents an input configuration.
type InputConfig struct {
	Excludes []Regexp        `yaml:"excludes,omitempty"`
	YAML     YAMLInputConfig `yaml:"yaml,omitempty"`
}

// YAMLInputConfig represents a YAML file input configuration.
type YAMLInputConfig struct {
	YTT YTTConfig `yaml:"ytt,omitempty"`
}

// YTTConfig represents a YAML file input configuration.
type YTTConfig struct {
	Enabled      bool     `yaml:"enabled,omitempty"`
	DefaultFiles []string `yaml:"defaultFiles,omitempty"`
}

// OutputConfig represents an output configuration.
type OutputConfig struct {
	Verbose bool         `yaml:"verbose,omitempty"`
	Colored *bool        `yaml:"colored,omitempty"`
	Summary bool         `yaml:"summary,omitempty"`
	Report  ReportConfig `yaml:"report,omitempty"`
}

// ReportConfig represents a report configuration.
type ReportConfig struct {
	JSON  JSONReportConfig  `yaml:"json,omitempty"`
	JUnit JUnitReportConfig `yaml:"junit,omitempty"`
}

// JSONReportConfig represents a JSON report configuration.
type JSONReportConfig struct {
	Filename string `yaml:"filename,omitempty"`
}

// JUnitReportConfig represents a JUnit report configuration.
type JUnitReportConfig struct {
	Filename string `yaml:"filename,omitempty"`
}

// LoadConfig loads a configuration from path.
func LoadConfig(path string) (*Config, error) {
	r, err := os.OpenFile(path, os.O_RDONLY, 0o400)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	root, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("failed to get root directory: %w", err)
	}

	return LoadConfigFromReader(r, root)
}

// LoadConfigFromReader loads a configuration from r.
func LoadConfigFromReader(r io.Reader, root string) (*Config, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	docs, err := readDocsWithSchemaVersionFromBytes(b)
	if err != nil {
		return nil, err
	}

	if l := len(docs); l == 0 {
		return nil, errors.New("empty config")
	} else if l != 1 {
		return nil, errors.New("must be a config document but contains more than one document")
	}
	d := docs[0]

	switch d.schemaVersion {
	case "config/v1":
		var cfg Config
		cm := make(yaml.CommentMap)
		if err := yaml.NodeToValue(d.doc.Body, &cfg, yaml.Strict(), yaml.UseOrderedMap(), yaml.CommentToMap(cm)); err != nil {
			return nil, err
		}
		cfg.Root = root
		if len(cm) > 0 {
			cfg.Comments = cm
		}
		cfg.Node = d.doc.Body
		if err := validate(&cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	case "":
		return nil, errors.New("schemaVersion not found")
	default:
		return nil, errors.WithNodeAndColored(
			errors.ErrorPathf("schemaVersion", "unknown version %q", d.schemaVersion),
			d.doc.Body,
			!color.NoColor,
		)
	}
}

func validate(c *Config) error {
	var errs []error
	for i, p := range c.Scenarios {
		if err := stat(c, p, (&yaml.PathBuilder{}).Root().Child("scenarios").Index(uint(i)).Build()); err != nil {
			errs = append(errs, err)
		}
	}
	for _, item := range c.Plugins.ToSlice() {
		item := item
		if err := stat(c, item.Value.Src, (&yaml.PathBuilder{}).Root().Child("plugins").Child(item.Key).Child("src").Build()); err != nil {
			var neErr notExist
			if errors.As(err, &neErr) {
				m := item.Value.Src
				if i := strings.Index(m, "@"); i >= 0 { // trim version query
					m = item.Value.Src[:i]
				}
				// may be a Go module path, not local files
				if merr := module.CheckPath(m); merr == nil {
					err = nil
				}
			}
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Errors(errs...)
}

type notExist error

func stat(c *Config, p string, path *yaml.Path) error {
	if _, err := os.Stat(filepathutil.From(c.Root, p)); err != nil {
		if os.IsNotExist(err) {
			err = notExist(errors.Errorf("%s: no such file or directory", p))
		}
		return errors.WithNodeAndColored(
			errors.WithPath(err, strings.TrimPrefix(path.String(), "$.")),
			c.Node, !color.NoColor,
		)
	}
	return nil
}
