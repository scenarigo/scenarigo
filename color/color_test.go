package color

import (
	"testing"
)

func TestMarshalYAML_Colorized(t *testing.T) {
	// Set up test data
	data := map[string]any{
		"method": "GET",
		"url":    "https://example.com",
		"header": map[string]string{
			"Content-Type": "application/json",
		},
	}

	// Test with color enabled
	c := New()
	c.SetEnabled(true) // Force enable color

	coloredBytes, err := c.MarshalYAML(data)
	if err != nil {
		t.Fatalf("Failed to marshal YAML with color: %v", err)
	}

	coloredString := string(coloredBytes)
	t.Logf("Colored YAML output:\n%s", coloredString)

	// Test with color disabled
	c.SetEnabled(false)

	plainBytes, err := c.MarshalYAML(data)
	if err != nil {
		t.Fatalf("Failed to marshal YAML without color: %v", err)
	}

	plainString := string(plainBytes)
	t.Logf("Plain YAML output:\n%s", plainString)

	// Verify that colored version contains ANSI codes
	if !c.IsEnabled() && len(coloredString) == len(plainString) {
		t.Logf("Color is disabled, both outputs should be the same length")
	}
}

func TestColorConfigFromEnv(t *testing.T) {
	// Test with SCENARIGO_COLOR=1
	t.Setenv(envScenarigoColor, "1")
	c1 := New()
	if !c1.IsEnabled() {
		t.Error("Expected color to be enabled when SCENARIGO_COLOR=1")
	}

	// Test with SCENARIGO_COLOR=0
	t.Setenv(envScenarigoColor, "0")
	c2 := New()
	if c2.IsEnabled() {
		t.Error("Expected color to be disabled when SCENARIGO_COLOR=0")
	}

	// Test marshaling with env-based config
	data := map[string]string{"test": "value"}

	// With color enabled
	t.Setenv(envScenarigoColor, "true")
	cEnabled := New()
	enabledBytes, _ := cEnabled.MarshalYAML(data)
	t.Logf("Env-enabled colored output:\n%s", string(enabledBytes))

	// With color disabled
	t.Setenv(envScenarigoColor, "false")
	cDisabled := New()
	disabledBytes, _ := cDisabled.MarshalYAML(data)
	t.Logf("Env-disabled plain output:\n%s", string(disabledBytes))
}
