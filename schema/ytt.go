package schema

// YTT represents inputs for ytt.
type YTT struct {
	SchemaVersion string   `yaml:"schemaVersion,omitempty"`
	Files         []string `yaml:"files,omitempty"`
}
