package schema

// Scenarigo schema definitions for config and scenario YAML files.

// ConfigSchema returns the schema for scenarigo.yaml configuration files.
func ConfigSchema() *Schema {
	return &Schema{
		Fields: []*FieldInfo{
			{Name: "schemaVersion", Type: FieldTypeString, Description: "Schema version of the configuration file", EnumValues: []string{"config/v1"}},
			{Name: "vars", Type: FieldTypeMap, Description: "Variables available in all scenarios"},
			{Name: "secrets", Type: FieldTypeMap, Description: "Secret variables (not displayed in logs)"},
			{Name: "scenarios", Type: FieldTypeArray, Description: "Paths to scenario files or directories"},
			{Name: "pluginDirectory", Type: FieldTypeString, Description: "Directory for plugin builds"},
			{Name: "plugins", Type: FieldTypeMap, Description: "Plugin definitions", Children: []*FieldInfo{
				// Each plugin key maps to PluginConfig
			}},
			{Name: "protocols", Type: FieldTypeObject, Description: "Protocol-specific options", Children: []*FieldInfo{
				{Name: "grpc", Type: FieldTypeObject, Description: "gRPC protocol options", Children: grpcOptionFields()},
			}},
			{Name: "input", Type: FieldTypeObject, Description: "Input configuration", Children: []*FieldInfo{
				{Name: "excludes", Type: FieldTypeArray, Description: "File patterns to exclude"},
				{Name: "yaml", Type: FieldTypeObject, Description: "YAML input options", Children: []*FieldInfo{
					{Name: "ytt", Type: FieldTypeObject, Description: "ytt template options", Children: []*FieldInfo{
						{Name: "enabled", Type: FieldTypeBool, Description: "Enable ytt preprocessing"},
						{Name: "defaultFiles", Type: FieldTypeArray, Description: "Default ytt data files"},
					}},
				}},
			}},
			{Name: "output", Type: FieldTypeObject, Description: "Output configuration", Children: []*FieldInfo{
				{Name: "verbose", Type: FieldTypeBool, Description: "Enable verbose output"},
				{Name: "colored", Type: FieldTypeBool, Description: "Enable colored output"},
				{Name: "summary", Type: FieldTypeBool, Description: "Show summary after test run"},
				{Name: "report", Type: FieldTypeObject, Description: "Report output settings", Children: []*FieldInfo{
					{Name: "json", Type: FieldTypeObject, Description: "JSON report settings", Children: []*FieldInfo{
						{Name: "filename", Type: FieldTypeString, Description: "Output filename for JSON report"},
					}},
					{Name: "junit", Type: FieldTypeObject, Description: "JUnit report settings", Children: []*FieldInfo{
						{Name: "filename", Type: FieldTypeString, Description: "Output filename for JUnit report"},
					}},
				}},
			}},
			{Name: "execution", Type: FieldTypeObject, Description: "Execution settings", Children: []*FieldInfo{
				{Name: "parallel", Type: FieldTypeInt, Description: "Number of parallel test executions"},
			}},
		},
	}
}

// ScenarioSchema returns the schema for scenarigo test scenario YAML files.
func ScenarioSchema() *Schema {
	return &Schema{
		Fields: []*FieldInfo{
			{Name: "schemaVersion", Type: FieldTypeString, Description: "Schema version of the scenario file", EnumValues: []string{"scenario/v1"}},
			{Name: "title", Type: FieldTypeString, Description: "Scenario title"},
			{Name: "description", Type: FieldTypeString, Description: "Scenario description"},
			{Name: "plugins", Type: FieldTypeMap, Description: "Plugin name to path mapping"},
			{Name: "vars", Type: FieldTypeMap, Description: "Variables available in steps"},
			{Name: "secrets", Type: FieldTypeMap, Description: "Secret variables (not displayed in logs)"},
			{Name: "steps", Type: FieldTypeArray, Description: "Test steps", Children: stepFields()},
			{Name: "retry", Type: FieldTypeObject, Description: "Retry policy for all steps", Children: retryFields()},
			{Name: "anchors", Type: FieldTypeAny, Description: "YAML anchors (dummy field for anchor definitions)"},
		},
	}
}

func stepFields() []*FieldInfo {
	return []*FieldInfo{
		{Name: "id", Type: FieldTypeString, Description: "Step ID (alphanumeric, -, _)"},
		{Name: "title", Type: FieldTypeString, Description: "Step title"},
		{Name: "description", Type: FieldTypeString, Description: "Step description"},
		{Name: "if", Type: FieldTypeString, Description: "Conditional expression (template)"},
		{Name: "continueOnError", Type: FieldTypeBool, Description: "Continue even if this step fails"},
		{Name: "vars", Type: FieldTypeMap, Description: "Step-level variables"},
		{Name: "secrets", Type: FieldTypeMap, Description: "Step-level secrets"},
		{Name: "protocol", Type: FieldTypeString, Description: "Protocol to use", EnumValues: []string{"http", "grpc"}},
		{Name: "request", Type: FieldTypeObject, Description: "Request definition (protocol-specific)",
			DynamicKey: "protocol",
			DynamicChildren: func(discriminator string) []*FieldInfo {
				switch discriminator {
				case "http":
					return httpRequestFields()
				case "grpc":
					return grpcRequestFields()
				default:
					return nil
				}
			},
		},
		{Name: "expect", Type: FieldTypeObject, Description: "Expected response (protocol-specific)",
			DynamicKey: "protocol",
			DynamicChildren: func(discriminator string) []*FieldInfo {
				switch discriminator {
				case "http":
					return httpExpectFields()
				case "grpc":
					return grpcExpectFields()
				default:
					return nil
				}
			},
		},
		{Name: "include", Type: FieldTypeString, Description: "Path to another scenario file to include"},
		{Name: "ref", Type: FieldTypeAny, Description: "Reference to a step in another scenario"},
		{Name: "bind", Type: FieldTypeObject, Description: "Bind step results to variables", Children: []*FieldInfo{
			{Name: "vars", Type: FieldTypeMap, Description: "Variables to bind from response"},
			{Name: "secrets", Type: FieldTypeMap, Description: "Secrets to bind from response"},
		}},
		{Name: "timeout", Type: FieldTypeDuration, Description: "Step timeout (e.g. 30s, 1m)"},
		{Name: "postTimeoutWaitingLimit", Type: FieldTypeDuration, Description: "Max wait time after timeout"},
		{Name: "retry", Type: FieldTypeObject, Description: "Step-level retry policy", Children: retryFields()},
	}
}

func retryFields() []*FieldInfo {
	return []*FieldInfo{
		{Name: "constant", Type: FieldTypeObject, Description: "Constant backoff retry policy", Children: []*FieldInfo{
			{Name: "interval", Type: FieldTypeDuration, Description: "Interval between retries (default: 1s)"},
			{Name: "maxRetries", Type: FieldTypeInt, Description: "Maximum number of retries (default: 5, 0=forever)"},
			{Name: "maxElapsedTime", Type: FieldTypeDuration, Description: "Maximum total time (default: 0=forever)"},
		}},
		{Name: "exponential", Type: FieldTypeObject, Description: "Exponential backoff retry policy", Children: []*FieldInfo{
			{Name: "initialInterval", Type: FieldTypeDuration, Description: "Initial retry interval (default: 500ms)"},
			{Name: "factor", Type: FieldTypeFloat, Description: "Backoff multiplier (default: 1.5)"},
			{Name: "jitterFactor", Type: FieldTypeFloat, Description: "Random jitter factor (default: 0.5)"},
			{Name: "maxInterval", Type: FieldTypeDuration, Description: "Maximum retry interval (default: 60s)"},
			{Name: "maxRetries", Type: FieldTypeInt, Description: "Maximum number of retries (default: 5, 0=forever)"},
			{Name: "maxElapsedTime", Type: FieldTypeDuration, Description: "Maximum total time (default: 0=forever)"},
		}},
	}
}

// HTTP protocol fields.

func httpRequestFields() []*FieldInfo {
	return []*FieldInfo{
		{Name: "client", Type: FieldTypeString, Description: "Custom HTTP client (template expression)"},
		{Name: "method", Type: FieldTypeString, Description: "HTTP method", EnumValues: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}},
		{Name: "url", Type: FieldTypeString, Description: "Request URL"},
		{Name: "query", Type: FieldTypeMap, Description: "Query parameters"},
		{Name: "header", Type: FieldTypeMap, Description: "Request headers"},
		{Name: "body", Type: FieldTypeAny, Description: "Request body"},
	}
}

func httpExpectFields() []*FieldInfo {
	return []*FieldInfo{
		{Name: "code", Type: FieldTypeString, Description: "Expected HTTP status code (e.g. \"200\", \"OK\")"},
		{Name: "header", Type: FieldTypeMap, Description: "Expected response headers"},
		{Name: "body", Type: FieldTypeAny, Description: "Expected response body"},
	}
}

// gRPC protocol fields.

func grpcRequestFields() []*FieldInfo {
	return []*FieldInfo{
		{Name: "client", Type: FieldTypeString, Description: "Custom gRPC client (template expression)"},
		{Name: "target", Type: FieldTypeString, Description: "gRPC target address"},
		{Name: "service", Type: FieldTypeString, Description: "gRPC service name"},
		{Name: "method", Type: FieldTypeString, Description: "gRPC method name"},
		{Name: "metadata", Type: FieldTypeMap, Description: "gRPC metadata (headers)"},
		{Name: "message", Type: FieldTypeAny, Description: "Request message"},
		{Name: "options", Type: FieldTypeObject, Description: "gRPC request options", Children: grpcRequestOptionFields()},
	}
}

func grpcExpectFields() []*FieldInfo {
	return []*FieldInfo{
		{Name: "code", Type: FieldTypeString, Description: "Expected gRPC status code (e.g. \"OK\", \"NotFound\")"},
		{Name: "message", Type: FieldTypeAny, Description: "Expected response message"},
		{Name: "status", Type: FieldTypeObject, Description: "Expected gRPC status", Children: []*FieldInfo{
			{Name: "code", Type: FieldTypeString, Description: "Status code"},
			{Name: "message", Type: FieldTypeString, Description: "Status message"},
			{Name: "details", Type: FieldTypeArray, Description: "Status details"},
		}},
		{Name: "header", Type: FieldTypeMap, Description: "Expected response headers"},
		{Name: "trailer", Type: FieldTypeMap, Description: "Expected response trailers"},
	}
}

func grpcOptionFields() []*FieldInfo {
	return []*FieldInfo{
		{Name: "request", Type: FieldTypeObject, Description: "gRPC request options", Children: grpcRequestOptionFields()},
	}
}

func grpcRequestOptionFields() []*FieldInfo {
	return []*FieldInfo{
		{Name: "reflection", Type: FieldTypeObject, Description: "gRPC reflection options", Children: []*FieldInfo{
			{Name: "enabled", Type: FieldTypeBool, Description: "Enable gRPC server reflection"},
		}},
		{Name: "proto", Type: FieldTypeObject, Description: "Protocol Buffers options", Children: []*FieldInfo{
			{Name: "imports", Type: FieldTypeArray, Description: "Proto import paths"},
			{Name: "files", Type: FieldTypeArray, Description: "Proto files"},
		}},
		{Name: "auth", Type: FieldTypeObject, Description: "Authentication options", Children: []*FieldInfo{
			{Name: "insecure", Type: FieldTypeBool, Description: "Use insecure connection"},
			{Name: "tls", Type: FieldTypeObject, Description: "TLS configuration"},
		}},
	}
}

// DetectSchemaType determines whether a YAML document is config or scenario.
func DetectSchemaType(text string) *Schema {
	// Simple heuristic: check for schemaVersion field.
	for _, line := range splitLines(text) {
		trimmed := trimSpace(line)
		if hasPrefix(trimmed, "schemaVersion:") {
			value := trimSpace(trimmed[len("schemaVersion:"):])
			// Remove quotes.
			value = trimQuotes(value)
			if value == "config/v1" {
				return ConfigSchema()
			}
			return ScenarioSchema()
		}
	}
	// Default to scenario.
	return ScenarioSchema()
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	j := len(s)
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func trimQuotes(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}
