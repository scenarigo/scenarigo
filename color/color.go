package color

import (
	"os"
	"strconv"

	"github.com/fatih/color"
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

// Color instances getters.
func (c *Config) Red() *Color    { return c.red }
func (c *Config) HiRed() *Color  { return c.hiRed }
func (c *Config) Green() *Color  { return c.green }
func (c *Config) Yellow() *Color { return c.yellow }
func (c *Config) Blue() *Color   { return c.blue }
func (c *Config) Cyan() *Color   { return c.cyan }
func (c *Config) White() *Color  { return c.white }
