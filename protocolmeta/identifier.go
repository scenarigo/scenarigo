package protocolmeta

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ScenarioIdentifier represents a parsed scenario identifier.
type ScenarioIdentifier struct {
	FilePath      string
	ScenarioIndex int
	Title         string // optional, empty if not set
}

// StepIdentifier represents a parsed step identifier.
type StepIdentifier struct {
	FilePath      string
	ScenarioIndex int
	StepIndex     int
	Title         string // scenario title, optional
}

// BuildScenarioIdentifier builds a scenario identifier string.
// title is optional — pass "" to omit.
func BuildScenarioIdentifier(filepath string, scenarioIndex int, title string) string {
	id := fmt.Sprintf("%s#scenario=%d", filepath, scenarioIndex)
	if title != "" {
		id += "&title=" + url.QueryEscape(title)
	}
	return id
}

// BuildStepIdentifier builds a step identifier string.
func BuildStepIdentifier(filepath string, scenarioIndex, stepIndex int, title string) string {
	id := fmt.Sprintf("%s#scenario=%d&step=%d", filepath, scenarioIndex, stepIndex)
	if title != "" {
		id += "&title=" + url.QueryEscape(title)
	}
	return id
}

// ParseScenarioIdentifier parses a scenario identifier string.
// Returns nil if the string is not a valid scenario identifier.
func ParseScenarioIdentifier(id string) *ScenarioIdentifier {
	path, fragment, ok := strings.Cut(id, "#")
	if !ok {
		return nil
	}
	params, err := url.ParseQuery(fragment)
	if err != nil {
		return nil
	}
	scenarioStr := params.Get("scenario")
	if scenarioStr == "" {
		return nil
	}
	scenarioIdx, err := strconv.Atoi(scenarioStr)
	if err != nil {
		return nil
	}
	return &ScenarioIdentifier{
		FilePath:      path,
		ScenarioIndex: scenarioIdx,
		Title:         params.Get("title"),
	}
}

// ParseStepIdentifier parses a step identifier string.
// Returns nil if the string is not a valid step identifier.
func ParseStepIdentifier(id string) *StepIdentifier {
	path, fragment, ok := strings.Cut(id, "#")
	if !ok {
		return nil
	}
	params, err := url.ParseQuery(fragment)
	if err != nil {
		return nil
	}
	scenarioStr := params.Get("scenario")
	if scenarioStr == "" {
		return nil
	}
	scenarioIdx, err := strconv.Atoi(scenarioStr)
	if err != nil {
		return nil
	}
	stepStr := params.Get("step")
	if stepStr == "" {
		return nil
	}
	stepIdx, err := strconv.Atoi(stepStr)
	if err != nil {
		return nil
	}
	return &StepIdentifier{
		FilePath:      path,
		ScenarioIndex: scenarioIdx,
		StepIndex:     stepIdx,
		Title:         params.Get("title"),
	}
}
