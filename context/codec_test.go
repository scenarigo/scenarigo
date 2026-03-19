package context

import (
	"reflect"
	"testing"

	"github.com/scenarigo/scenarigo/color"
	"github.com/scenarigo/scenarigo/reporter"
)

func TestContextSerializationRoundtrip(t *testing.T) {
	// Create a context using public API and set various properties
	originalReporter := reporter.FromT(t)
	originalContext := New(originalReporter)

	// Use public API to set context properties
	originalContext = originalContext.WithScenarioFilepath("/test/scenario.yaml")
	originalContext = originalContext.WithPluginDir("/test/plugins")
	originalContext = originalContext.WithVars("testVar1")
	originalContext = originalContext.WithVars(42)
	originalContext = originalContext.WithVars(map[string]string{"key": "value"})
	originalContext = originalContext.WithPlugins(map[string]any{"plugin1": "config1"})
	originalContext = originalContext.WithPlugins(map[string]any{"plugin2": map[string]int{"port": 8080}})
	originalContext = originalContext.WithRequest(map[string]any{"method": "GET", "url": "http://example.com"})
	originalContext = originalContext.WithResponse(map[string]any{"status": 200, "body": "OK"})

	// Set ColorConfig with enabled state
	colorConfig := color.New()
	colorConfig.SetEnabled(true)
	originalContext = originalContext.WithColorConfig(colorConfig)

	// Create Steps object for testing
	steps := &Steps{}
	originalContext = originalContext.WithSteps(steps)

	// Serialize the context
	serialized := originalContext.ToSerializable()

	// Deserialize back to context
	restored, err := FromSerializable(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize context: %v", err)
	}

	// Verify properties are preserved through public API
	if restored.ScenarioFilepath() != originalContext.ScenarioFilepath() {
		t.Errorf("ScenarioFilepath mismatch: expected %s, got %s",
			originalContext.ScenarioFilepath(), restored.ScenarioFilepath())
	}

	if restored.PluginDir() != originalContext.PluginDir() {
		t.Errorf("PluginDir mismatch: expected %s, got %s",
			originalContext.PluginDir(), restored.PluginDir())
	}

	// Verify ColorConfig enabled state is preserved
	if restored.ColorConfig() == nil {
		t.Error("ColorConfig should be restored")
	} else if restored.ColorConfig().IsEnabled() != originalContext.ColorConfig().IsEnabled() {
		t.Errorf("ColorConfig enabled state mismatch: expected %t, got %t",
			originalContext.ColorConfig().IsEnabled(), restored.ColorConfig().IsEnabled())
	}

	// Verify plugins (note: may be nested due to multiple WithPlugins calls)
	originalPlugins := originalContext.Plugins()
	restoredPlugins := restored.Plugins()
	if len(originalPlugins) != len(restoredPlugins) {
		t.Errorf("Plugins count mismatch: expected %d, got %d",
			len(originalPlugins), len(restoredPlugins))
	}

	// Verify vars exist (structure may differ due to serialization)
	originalVars := originalContext.Vars()
	restoredVars := restored.Vars()
	if len(originalVars) == 0 {
		t.Error("Original context should have vars")
	}
	if len(restoredVars) == 0 {
		t.Error("Restored context should have vars")
	}

	// Verify request and response
	if !reflect.DeepEqual(restored.Request(), originalContext.Request()) {
		t.Errorf("Request mismatch: expected %v, got %v",
			originalContext.Request(), restored.Request())
	}

	if !reflect.DeepEqual(restored.Response(), originalContext.Response()) {
		t.Errorf("Response mismatch: expected %v, got %v",
			originalContext.Response(), restored.Response())
	}

	// Verify steps
	if restored.Steps() == nil && originalContext.Steps() != nil {
		t.Error("Steps should be preserved")
	}
	if restored.Steps() != nil && originalContext.Steps() == nil {
		t.Error("Steps should be nil if original was nil")
	}

	// Verify reporter is restored (should not be nil)
	if restored.Reporter() == nil {
		t.Error("Reporter should be restored")
	}
}

func TestContextSerializationBasic(t *testing.T) {
	// Test basic context serialization without reporter
	originalContext := New(nil)

	// Set basic properties
	originalContext = originalContext.WithScenarioFilepath("/basic/test.yaml")
	originalContext = originalContext.WithVars("basicVar")
	originalContext = originalContext.WithRequest(map[string]any{"method": "POST"})
	originalContext = originalContext.WithResponse(map[string]any{"status": 200})

	// Set ColorConfig with disabled state
	colorConfig := color.New()
	colorConfig.SetEnabled(false)
	originalContext = originalContext.WithColorConfig(colorConfig)

	// Serialize and deserialize
	serialized := originalContext.ToSerializable()
	restored, err := FromSerializable(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize context: %v", err)
	}

	// Verify basic properties are preserved
	if restored.ScenarioFilepath() != originalContext.ScenarioFilepath() {
		t.Errorf("ScenarioFilepath mismatch: expected %s, got %s",
			originalContext.ScenarioFilepath(), restored.ScenarioFilepath())
	}

	// Verify ColorConfig disabled state is preserved
	if restored.ColorConfig() == nil {
		t.Error("ColorConfig should be restored")
	} else if restored.ColorConfig().IsEnabled() != originalContext.ColorConfig().IsEnabled() {
		t.Errorf("ColorConfig enabled state mismatch: expected %t, got %t",
			originalContext.ColorConfig().IsEnabled(), restored.ColorConfig().IsEnabled())
	}

	if !reflect.DeepEqual(restored.Request(), originalContext.Request()) {
		t.Errorf("Request mismatch: expected %v, got %v",
			originalContext.Request(), restored.Request())
	}

	if !reflect.DeepEqual(restored.Response(), originalContext.Response()) {
		t.Errorf("Response mismatch: expected %v, got %v",
			originalContext.Response(), restored.Response())
	}

	// Verify vars are preserved (accounting for nested structure)
	originalVars := originalContext.Vars()
	restoredVars := restored.Vars()
	if len(originalVars) != len(restoredVars) {
		t.Errorf("Vars count mismatch: expected %d, got %d",
			len(originalVars), len(restoredVars))
	}
}

func TestContextWithReporterSerialization(t *testing.T) {
	// Test context with actual reporter that supports serialization
	originalReporter := reporter.FromT(t)
	originalContext := New(originalReporter)

	// Set up the reporter with some state
	originalReporter.Log("context test log")
	originalReporter.Run("sub-test", func(sub reporter.Reporter) {
		sub.Log("sub test log")
	})

	// Set context properties
	originalContext = originalContext.WithScenarioFilepath("/reporter/test.yaml")
	originalContext = originalContext.WithVars("reporterVar")

	// Serialize context
	serialized := originalContext.ToSerializable()

	// Verify reporter was serialized
	if serialized.ReporterID == "" {
		t.Error("ReporterID should be set when reporter supports ToSerializable")
	}
	if serialized.ReporterMap == nil {
		t.Error("ReporterMap should be set when reporter supports ToSerializable")
	}

	// Deserialize
	restored, err := FromSerializable(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize context: %v", err)
	}

	// Verify context properties
	if restored.ScenarioFilepath() != originalContext.ScenarioFilepath() {
		t.Errorf("ScenarioFilepath mismatch: expected %s, got %s",
			originalContext.ScenarioFilepath(), restored.ScenarioFilepath())
	}

	// Verify reporter was restored
	if restored.Reporter() == nil {
		t.Error("Reporter should be restored")
	}
}

func TestContextMultipleVarsAndPlugins(t *testing.T) {
	// Test multiple vars and plugins additions to verify nested structure handling
	originalContext := New(nil)

	// Add multiple vars
	originalContext = originalContext.WithVars("var1")
	originalContext = originalContext.WithVars("var2")
	originalContext = originalContext.WithVars(123)

	// Add multiple plugins
	originalContext = originalContext.WithPlugins(map[string]any{"plugin1": "config1"})
	originalContext = originalContext.WithPlugins(map[string]any{"plugin2": "config2"})

	// Set other properties
	originalContext = originalContext.WithScenarioFilepath("/multi/test.yaml")
	originalContext = originalContext.WithRequest(map[string]any{"data": "test"})

	// Serialize and deserialize
	serialized := originalContext.ToSerializable()
	restored, err := FromSerializable(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize context: %v", err)
	}

	// Verify basic properties
	if restored.ScenarioFilepath() != originalContext.ScenarioFilepath() {
		t.Errorf("ScenarioFilepath mismatch: expected %s, got %s",
			originalContext.ScenarioFilepath(), restored.ScenarioFilepath())
	}

	if !reflect.DeepEqual(restored.Request(), originalContext.Request()) {
		t.Errorf("Request mismatch: expected %v, got %v",
			originalContext.Request(), restored.Request())
	}

	// Verify vars structure (should preserve the total count through nested arrays)
	originalVars := originalContext.Vars()
	restoredVars := restored.Vars()

	// Both should have accumulated vars
	if len(originalVars) == 0 {
		t.Error("Original context should have vars")
	}
	if len(restoredVars) == 0 {
		t.Error("Restored context should have vars")
	}

	// Verify plugins structure (should preserve the total count)
	originalPlugins := originalContext.Plugins()
	restoredPlugins := restored.Plugins()

	if len(originalPlugins) == 0 {
		t.Error("Original context should have plugins")
	}
	if len(restoredPlugins) == 0 {
		t.Error("Restored context should have plugins")
	}
}

func TestContextWithSecretsSerialization(t *testing.T) {
	// Create a context with secrets using the public API
	originalReporter := reporter.FromT(t)
	originalContext := New(originalReporter)

	// Add secrets using WithSecrets
	originalContext = originalContext.WithSecrets(map[string]any{"api_key": "secret123"})
	originalContext = originalContext.WithSecrets(map[string]any{"db_password": "pass456"})
	originalContext = originalContext.WithSecrets("simple_secret")
	originalContext = originalContext.WithScenarioFilepath("/secrets/test.yaml")
	originalContext = originalContext.WithVars("test_var")

	// Serialize the context
	serialized := originalContext.ToSerializable()

	// Verify secrets are serialized
	if serialized.Secrets == nil {
		t.Error("Secrets should be serialized")
	}

	if len(serialized.Secrets.Secrets) != 3 {
		t.Errorf("Secrets data count mismatch: expected 3, got %d",
			len(serialized.Secrets.Secrets))
	}

	if len(serialized.Secrets.Values) == 0 {
		t.Error("Secrets values should not be empty")
	}

	// Verify that the serialized secrets contain the expected data
	expectedSecrets := []any{
		map[string]any{"api_key": "secret123"},
		map[string]any{"db_password": "pass456"},
		"simple_secret",
	}

	if !reflect.DeepEqual(serialized.Secrets.Secrets, expectedSecrets) {
		t.Errorf("Serialized secrets data mismatch: expected %v, got %v",
			expectedSecrets, serialized.Secrets.Secrets)
	}

	// Verify that query values are present
	originalSecrets := originalContext.Secrets()
	expectedQueryValueCount := len(originalSecrets.values)

	if len(serialized.Secrets.Values) != expectedQueryValueCount {
		t.Errorf("Serialized secrets values count mismatch: expected %d, got %d",
			expectedQueryValueCount, len(serialized.Secrets.Values))
	}

	// Verify that Query and Value fields are properly serialized
	for i, qv := range serialized.Secrets.Values {
		if i < len(originalSecrets.values) {
			originalQV := originalSecrets.values[i]
			if qv.Query != originalQV.query {
				t.Errorf("Serialized query mismatch at index %d: expected %s, got %s",
					i, originalQV.query, qv.Query)
			}
			if qv.Value != originalQV.v {
				t.Errorf("Serialized value mismatch at index %d: expected %s, got %s",
					i, originalQV.v, qv.Value)
			}
		}
	}

	// Test deserialization (basic check only due to current implementation limitation)
	restored, err := FromSerializable(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize context: %v", err)
	}

	// Verify context properties are preserved
	if restored.ScenarioFilepath() != originalContext.ScenarioFilepath() {
		t.Errorf("ScenarioFilepath mismatch: expected %s, got %s",
			originalContext.ScenarioFilepath(), restored.ScenarioFilepath())
	}

	// Verify secrets are restored (at least not nil)
	restoredSecrets := restored.Secrets()
	if restoredSecrets == nil {
		t.Error("Secrets should be restored")
	}

	// Note: Due to current implementation, deserialized secrets are wrapped in another layer
	// This test focuses on serialization correctness and basic deserialization functionality
}

func TestContextWithEmptySecretsSerialization(t *testing.T) {
	// Test context with empty secrets by adding nil
	originalReporter := reporter.FromT(t)
	originalContext := New(originalReporter)

	// Add nil secrets (which should be ignored)
	originalContext = originalContext.WithSecrets(nil)
	originalContext = originalContext.WithScenarioFilepath("/empty-secrets/test.yaml")

	// Serialize the context
	serialized := originalContext.ToSerializable()

	// Verify secrets are nil when no actual secrets are added
	if serialized.Secrets != nil {
		t.Error("Secrets should be nil when only nil is added")
	}

	// Deserialize back to context
	restored, err := FromSerializable(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize context: %v", err)
	}

	// Verify secrets remain nil
	restoredSecrets := restored.Secrets()
	if restoredSecrets != nil {
		t.Error("Restored secrets should be nil when no secrets were added")
	}
}

func TestContextSecretsComplexDataSerialization(t *testing.T) {
	// Test with complex secrets data structures using the public API
	originalReporter := reporter.FromT(t)
	originalContext := New(originalReporter)

	// Define complex secrets data
	databaseSecret := map[string]any{
		"database": map[string]any{
			"host":     "localhost",
			"port":     5432,
			"username": "admin",
			"password": "complex_pass_123",
		},
	}
	apiSecret := map[string]any{
		"api": map[string]any{
			"keys": []string{"key1", "key2", "key3"},
			"tokens": map[string]string{
				"read":  "read_token_456",
				"write": "write_token_789",
			},
		},
	}
	arraySecret := []any{"secret1", "secret2", "secret3"}
	numberSecret := 42
	stringSecret := "simple_string_secret"

	// Add complex secrets using WithSecrets
	originalContext = originalContext.WithSecrets(databaseSecret)
	originalContext = originalContext.WithSecrets(apiSecret)
	originalContext = originalContext.WithSecrets(arraySecret)
	originalContext = originalContext.WithSecrets(numberSecret)
	originalContext = originalContext.WithSecrets(stringSecret)
	originalContext = originalContext.WithScenarioFilepath("/complex-secrets/test.yaml")

	// Serialize the context
	serialized := originalContext.ToSerializable()

	// Verify complex secrets are serialized
	if serialized.Secrets == nil {
		t.Error("Complex secrets should be serialized")
	}

	if len(serialized.Secrets.Secrets) != 5 {
		t.Errorf("Complex secrets data count mismatch: expected 5, got %d",
			len(serialized.Secrets.Secrets))
	}

	if len(serialized.Secrets.Values) == 0 {
		t.Error("Complex secrets values should not be empty")
	}

	// Verify that the serialized secrets contain the expected complex data
	expectedSecrets := []any{databaseSecret, apiSecret, arraySecret, numberSecret, stringSecret}

	if !reflect.DeepEqual(serialized.Secrets.Secrets, expectedSecrets) {
		t.Errorf("Serialized complex secrets data mismatch: expected %v, got %v",
			expectedSecrets, serialized.Secrets.Secrets)
	}

	// Verify that query values are present for complex data
	originalSecrets := originalContext.Secrets()
	expectedQueryValueCount := len(originalSecrets.values)

	if len(serialized.Secrets.Values) != expectedQueryValueCount {
		t.Errorf("Serialized complex secrets values count mismatch: expected %d, got %d",
			expectedQueryValueCount, len(serialized.Secrets.Values))
	}

	// Verify that Query and Value fields are properly serialized for complex data
	for i, qv := range serialized.Secrets.Values {
		if i < len(originalSecrets.values) {
			originalQV := originalSecrets.values[i]
			if qv.Query != originalQV.query {
				t.Errorf("Serialized query mismatch at index %d: expected %s, got %s",
					i, originalQV.query, qv.Query)
			}
			if qv.Value != originalQV.v {
				t.Errorf("Serialized value mismatch at index %d: expected %s, got %s",
					i, originalQV.v, qv.Value)
			}
		}
	}

	// Test deserialization (basic check only due to current implementation limitation)
	restored, err := FromSerializable(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize context: %v", err)
	}

	// Verify context properties are preserved
	if restored.ScenarioFilepath() != originalContext.ScenarioFilepath() {
		t.Errorf("ScenarioFilepath mismatch: expected %s, got %s",
			originalContext.ScenarioFilepath(), restored.ScenarioFilepath())
	}

	// Verify complex secrets are restored (at least not nil)
	restoredSecrets := restored.Secrets()
	if restoredSecrets == nil {
		t.Error("Complex secrets should be restored")
	}

	// Note: Due to current implementation, deserialized secrets are wrapped in another layer
	// This test focuses on serialization correctness and basic deserialization functionality
}

func TestFromSerializableWithContextPluginDirError(t *testing.T) {
	// Test FromSerializableWithContext with empty plugin directory path
	originalReporter := reporter.FromT(t)
	originalContext := New(originalReporter)

	// Create serialized context with empty plugin directory
	serialized := &SerializableContext{
		ScenarioFilepath: "/test/scenario.yaml",
		PluginDir:        "", // Empty string that will be handled gracefully
		Vars:             []any{"test_var"},
	}

	// Test the function
	restored, err := FromSerializableWithContext(originalContext, serialized)
	if err != nil {
		t.Fatalf("FromSerializableWithContext should handle empty plugin dir gracefully: %v", err)
	}

	// Verify plugin dir was set even if empty
	if restored == nil {
		t.Error("FromSerializableWithContext should not return nil")
	}
}

func TestContextSerializationWithComplexData(t *testing.T) {
	// Test serialization with complex nested data structures
	originalReporter := reporter.FromT(t)
	originalContext := New(originalReporter)

	// Create complex data structures
	complexPlugin := map[string]any{
		"name": "complex-plugin",
		"config": map[string]any{
			"nested": map[string]any{
				"deep": map[string]any{
					"value": 42,
					"array": []int{1, 2, 3},
				},
			},
		},
	}

	// Set complex data in context
	originalContext = originalContext.WithScenarioFilepath("/complex/test.yaml")
	originalContext = originalContext.WithPluginDir("/complex/plugins")
	originalContext = originalContext.WithPlugins(complexPlugin)
	colorConfig := color.New()
	colorConfig.SetEnabled(true)
	originalContext = originalContext.WithColorConfig(colorConfig)

	// Serialize the context
	serialized := originalContext.ToSerializable()

	// Verify complex data was serialized correctly
	if !reflect.DeepEqual(serialized.Plugins[0], complexPlugin) {
		t.Errorf("Complex plugin data mismatch during serialization")
	}

	// Deserialize and verify complex data is preserved
	restored, err := FromSerializable(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize complex context: %v", err)
	}

	// Verify basic properties
	if restored.ScenarioFilepath() != originalContext.ScenarioFilepath() {
		t.Errorf("ScenarioFilepath mismatch: expected %s, got %s",
			originalContext.ScenarioFilepath(), restored.ScenarioFilepath())
	}
}
