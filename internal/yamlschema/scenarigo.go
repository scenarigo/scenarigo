package yamlschema

import "strings"

// Scenarigo schema definitions for config and scenario YAML files.

// ConfigSchema returns the schema for scenarigo.yaml configuration files.
func ConfigSchema() *Schema {
	return &Schema{
		Fields: []*FieldInfo{
			{Name: "schemaVersion", Type: FieldTypeString, Description: "Schema version of the configuration file", Required: true, EnumValues: []string{"config/v1", "scenario/v1"}},
			{Name: "vars", Type: FieldTypeMap, Description: "Variables available in all scenarios"},
			{Name: "secrets", Type: FieldTypeMap, Description: "Secret variables (not displayed in logs)"},
			{Name: "scenarios", Type: FieldTypeArray, Description: "Paths to scenario files or directories", IsFilePath: true},
			{Name: "pluginDirectory", Type: FieldTypeString, Description: "Directory for plugin builds", IsFilePath: true},
			{Name: "plugins", Type: FieldTypeMap, Description: "Plugin definitions keyed by plugin name", Children: pluginConfigFields()},
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
			{Name: "schemaVersion", Type: FieldTypeString, Description: "Schema version of the scenario file", Required: true, EnumValues: []string{"scenario/v1", "config/v1"}},
			{Name: "title", Type: FieldTypeString, Description: "Scenario title"},
			{Name: "description", Type: FieldTypeString, Description: "Scenario description"},
			{Name: "plugins", Type: FieldTypeMap, Description: "Plugin name to path mapping", IsFilePath: true},
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
		{Name: "protocol", Type: FieldTypeString, Description: "Protocol to use (plugins can register more)", EnumValues: []string{"http", "grpc"}, OpenEnum: true},
		{
			Name: "request", Type: FieldTypeObject, Description: "Request definition (protocol-specific)",
			DynamicKey: "protocol",
			DynamicChildren: func(discriminator string) []*FieldInfo {
				switch discriminator {
				case "http":
					return httpRequestFields()
				case "grpc":
					return grpcRequestFields()
				default:
					return mergeFields(httpRequestFields(), grpcRequestFields())
				}
			},
		},
		{
			Name: "expect", Type: FieldTypeObject, Description: "Expected response (protocol-specific)",
			DynamicKey: "protocol",
			DynamicChildren: func(discriminator string) []*FieldInfo {
				switch discriminator {
				case "http":
					return httpExpectFields()
				case "grpc":
					return grpcExpectFields()
				default:
					return mergeFields(httpExpectFields(), grpcExpectFields())
				}
			},
		},
		{Name: "include", Type: FieldTypeString, Description: "Path to another scenario file to include", IsFilePath: true},
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

func pluginConfigFields() []*FieldInfo {
	return []*FieldInfo{
		{Name: "src", Type: FieldTypeString, Description: "Go module path or local directory of the plugin source", IsFilePath: true},
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
		{Name: "method", Type: FieldTypeString, Description: "HTTP method", Required: true, EnumValues: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}},
		{Name: "url", Type: FieldTypeString, Description: "Request URL", Required: true},
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
		{Name: "body", Type: FieldTypeAny, Description: "Request message (deprecated: use message)", Deprecated: true},
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
		{Name: "body", Type: FieldTypeAny, Description: "Expected response message (deprecated: use message)", Deprecated: true},
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
			{Name: "tls", Type: FieldTypeObject, Description: "TLS configuration", Children: []*FieldInfo{
				{Name: "minVersion", Type: FieldTypeString, Description: "Minimum acceptable TLS version (default: 1.2)"},
				{Name: "maxVersion", Type: FieldTypeString, Description: "Maximum acceptable TLS version (default: 1.3)"},
				{Name: "certificate", Type: FieldTypeString, Description: "Path to the CA certificate file", IsFilePath: true},
				{Name: "skip", Type: FieldTypeBool, Description: "Skip server certificate verification"},
			}},
		}},
	}
}

// DetectSchemaType determines whether a YAML document is config or scenario.
// Returns nil if the document has no "schemaVersion:" key, indicating it
// is not a scenarigo YAML file and the LSP should remain silent.
// When the key exists but the value is empty or unrecognized, it defaults
// to ScenarioSchema (the most common type) so that completion and other
// features remain available while the user is still typing.
func DetectSchemaType(text string) *Schema {
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "schemaVersion:") {
			value := strings.TrimSpace(trimmed[len("schemaVersion:"):])
			value = trimQuotes(value)
			switch value {
			case "config/v1":
				return ConfigSchema()
			case "scenario/v1":
				return ScenarioSchema()
			default:
				// Key exists but value is empty or unrecognized.
				// Default to scenario schema so features stay active.
				return ScenarioSchema()
			}
		}
	}
	return nil
}

// mergeFields combines two field slices, deduplicating by name.
func mergeFields(a, b []*FieldInfo) []*FieldInfo {
	seen := make(map[string]bool, len(a))
	result := make([]*FieldInfo, 0, len(a)+len(b))
	for _, f := range a {
		seen[f.Name] = true
		result = append(result, f)
	}
	for _, f := range b {
		if !seen[f.Name] {
			result = append(result, f)
		}
	}
	return result
}

func trimQuotes(s string) string {
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		return s[1 : len(s)-1]
	}
	return s
}
