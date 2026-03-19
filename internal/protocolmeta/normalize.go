package protocolmeta

import public "github.com/scenarigo/scenarigo/protocolmeta"

// NormalizeScenarioFilepath returns a stable, relative filepath when possible.
func NormalizeScenarioFilepath(path string) string {
	return public.NormalizeScenarioFilepath(path)
}
