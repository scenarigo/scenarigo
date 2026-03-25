package protocolmeta

import "github.com/scenarigo/scenarigo/internal/protocolmeta"

// NormalizeScenarioFilepath returns a stable, relative filepath when possible.
func NormalizeScenarioFilepath(path string) string {
	return protocolmeta.NormalizeScenarioFilepath(path)
}
