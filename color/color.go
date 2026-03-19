package color

import (
	"fmt"
	"os"
	"strconv"

	"github.com/fatih/color"
	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/printer"
)

const (
	envScenarigoColor = "SCENARIGO_COLOR"
)

type Color = color.Color

// Config represents color configuration for a Runner instance.
type Config struct {
	enabled *bool // nil means use default behavior
	// Color instances for different output types
	red    *Color
	hiRed  *Color
	green  *Color
	yellow *Color
	blue   *Color
	cyan   *Color
	white  *Color
	// Array of all color instances for efficient iteration
	colors []*Color
}

// New creates a new color configuration instance initialized from environment variables.
func New() *Config {
	c := &Config{
		red:    color.New(color.FgRed),
		hiRed:  color.New(color.FgHiRed),
		green:  color.New(color.FgGreen),
		yellow: color.New(color.FgYellow),
		blue:   color.New(color.FgBlue),
		cyan:   color.New(color.FgCyan),
		white:  color.New(color.FgWhite),
	}
	// Store references to all color instances for efficient iteration
	c.colors = []*Color{c.red, c.hiRed, c.green, c.yellow, c.blue, c.cyan, c.white}

	// Initialize from SCENARIGO_COLOR environment variable
	if envColor := os.Getenv(envScenarigoColor); envColor != "" {
		if result, err := strconv.ParseBool(envColor); err == nil {
			c.enabled = &result
			c.updateColorInstances()
		}
	}
	return c
}

// updateColorInstances enables or disables color instances based on the enabled setting.
func (c *Config) updateColorInstances() {
	if c.enabled != nil && *c.enabled {
		// Enable color for all instances
		for _, colorInstance := range c.colors {
			colorInstance.EnableColor()
		}
	} else if c.enabled != nil && !*c.enabled {
		// Disable color for all instances
		for _, colorInstance := range c.colors {
			colorInstance.DisableColor()
		}
	}
	// If c.enabled is nil, leave instances as default (respects color.NoColor)
}

// IsEnabled returns whether color output is enabled for this Config instance.
func (c *Config) IsEnabled() bool {
	if c != nil && c.enabled != nil {
		return *c.enabled
	}
	// Default behavior if not set
	return !color.NoColor
}

// SetEnabled sets the enabled state for this Config instance and updates color instances.
func (c *Config) SetEnabled(enabled bool) {
	c.enabled = &enabled
	c.updateColorInstances()
}

// Red returns the red Color instance.
func (c *Config) Red() *Color    { return c.red }
func (c *Config) HiRed() *Color  { return c.hiRed }
func (c *Config) Green() *Color  { return c.green }
func (c *Config) Yellow() *Color { return c.yellow }
func (c *Config) Blue() *Color   { return c.blue }
func (c *Config) Cyan() *Color   { return c.cyan }
func (c *Config) White() *Color  { return c.white }

// MarshalYAML marshals the given value to YAML with optional color formatting.
func (c *Config) MarshalYAML(v any) ([]byte, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	if !c.IsEnabled() {
		return b, nil
	}
	tokens := lexer.Tokenize(string(b))
	var p printer.Printer
	p.Bool = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiMagenta),
			Suffix: format(color.Reset),
		}
	}
	p.Number = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiMagenta),
			Suffix: format(color.Reset),
		}
	}
	p.MapKey = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiCyan),
			Suffix: format(color.Reset),
		}
	}
	p.Anchor = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiYellow),
			Suffix: format(color.Reset),
		}
	}
	p.Alias = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiYellow),
			Suffix: format(color.Reset),
		}
	}
	p.String = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiGreen),
			Suffix: format(color.Reset),
		}
	}
	p.Comment = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgHiBlack),
			Suffix: format(color.Reset),
		}
	}
	return []byte(p.PrintTokens(tokens)), nil
}

const escape = "\x1b"

func format(attr color.Attribute) string {
	return fmt.Sprintf("%s[%dm", escape, attr)
}
