package protocolmeta

import "github.com/scenarigo/scenarigo/internal/protocolmeta"

// ScenarioIdentifier represents a parsed scenario identifier.
type ScenarioIdentifier = protocolmeta.ScenarioIdentifier

// StepIdentifier represents a parsed step identifier.
type StepIdentifier = protocolmeta.StepIdentifier

// BuildScenarioIdentifier builds a scenario identifier string.
// title is optional — pass "" to omit.
func BuildScenarioIdentifier(filepath string, scenarioIndex int, title string) string {
	return protocolmeta.BuildScenarioIdentifier(filepath, scenarioIndex, title)
}

// BuildStepIdentifier builds a step identifier string.
func BuildStepIdentifier(filepath string, scenarioIndex, stepIndex int, title string) string {
	return protocolmeta.BuildStepIdentifier(filepath, scenarioIndex, stepIndex, title)
}

// ParseScenarioIdentifier parses a scenario identifier string.
// Returns nil if the string is not a valid scenario identifier.
func ParseScenarioIdentifier(id string) *ScenarioIdentifier {
	return protocolmeta.ParseScenarioIdentifier(id)
}

// ParseStepIdentifier parses a step identifier string.
// Returns nil if the string is not a valid step identifier.
func ParseStepIdentifier(id string) *StepIdentifier {
	return protocolmeta.ParseStepIdentifier(id)
}
