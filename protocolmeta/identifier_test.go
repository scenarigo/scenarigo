package protocolmeta

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBuildScenarioIdentifier(t *testing.T) {
	tests := map[string]struct {
		filepath      string
		scenarioIndex int
		title         string
		expect        string
	}{
		"without title": {
			filepath:      "scenarios/test.yaml",
			scenarioIndex: 0,
			title:         "",
			expect:        "scenarios/test.yaml#scenario=0",
		},
		"with title": {
			filepath:      "scenarios/test.yaml",
			scenarioIndex: 0,
			title:         "Login Test",
			expect:        "scenarios/test.yaml#scenario=0&title=Login+Test",
		},
		"with special characters in title": {
			filepath:      "scenarios/test.yaml",
			scenarioIndex: 1,
			title:         "テスト/シナリオ",
			expect:        "scenarios/test.yaml#scenario=1&title=%E3%83%86%E3%82%B9%E3%83%88%2F%E3%82%B7%E3%83%8A%E3%83%AA%E3%82%AA",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := BuildScenarioIdentifier(test.filepath, test.scenarioIndex, test.title)
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildStepIdentifier(t *testing.T) {
	tests := map[string]struct {
		filepath      string
		scenarioIndex int
		stepIndex     int
		title         string
		expect        string
	}{
		"without title": {
			filepath:      "scenarios/test.yaml",
			scenarioIndex: 0,
			stepIndex:     1,
			title:         "",
			expect:        "scenarios/test.yaml#scenario=0&step=1",
		},
		"with title": {
			filepath:      "scenarios/test.yaml",
			scenarioIndex: 0,
			stepIndex:     2,
			title:         "Login Test",
			expect:        "scenarios/test.yaml#scenario=0&step=2&title=Login+Test",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := BuildStepIdentifier(test.filepath, test.scenarioIndex, test.stepIndex, test.title)
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseScenarioIdentifier(t *testing.T) {
	tests := map[string]struct {
		input  string
		expect *ScenarioIdentifier
	}{
		"without title": {
			input: "scenarios/test.yaml#scenario=0",
			expect: &ScenarioIdentifier{
				FilePath:      "scenarios/test.yaml",
				ScenarioIndex: 0,
				Title:         "",
			},
		},
		"with title": {
			input: "scenarios/test.yaml#scenario=0&title=Login+Test",
			expect: &ScenarioIdentifier{
				FilePath:      "scenarios/test.yaml",
				ScenarioIndex: 0,
				Title:         "Login Test",
			},
		},
		"with encoded title": {
			input: "scenarios/test.yaml#scenario=1&title=%E3%83%86%E3%82%B9%E3%83%88",
			expect: &ScenarioIdentifier{
				FilePath:      "scenarios/test.yaml",
				ScenarioIndex: 1,
				Title:         "テスト",
			},
		},
		"no fragment": {
			input:  "scenarios/test.yaml",
			expect: nil,
		},
		"no scenario param": {
			input:  "scenarios/test.yaml#title=Test",
			expect: nil,
		},
		"invalid scenario index": {
			input:  "scenarios/test.yaml#scenario=abc",
			expect: nil,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := ParseScenarioIdentifier(test.input)
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseStepIdentifier(t *testing.T) {
	tests := map[string]struct {
		input  string
		expect *StepIdentifier
	}{
		"without title": {
			input: "scenarios/test.yaml#scenario=0&step=1",
			expect: &StepIdentifier{
				FilePath:      "scenarios/test.yaml",
				ScenarioIndex: 0,
				StepIndex:     1,
				Title:         "",
			},
		},
		"with title": {
			input: "scenarios/test.yaml#scenario=0&step=2&title=Login+Test",
			expect: &StepIdentifier{
				FilePath:      "scenarios/test.yaml",
				ScenarioIndex: 0,
				StepIndex:     2,
				Title:         "Login Test",
			},
		},
		"no step param": {
			input:  "scenarios/test.yaml#scenario=0",
			expect: nil,
		},
		"no fragment": {
			input:  "scenarios/test.yaml",
			expect: nil,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := ParseStepIdentifier(test.input)
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("differs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	t.Run("scenario identifier", func(t *testing.T) {
		original := &ScenarioIdentifier{
			FilePath:      "scenarios/test.yaml",
			ScenarioIndex: 2,
			Title:         "Login Test",
		}
		id := BuildScenarioIdentifier(original.FilePath, original.ScenarioIndex, original.Title)
		parsed := ParseScenarioIdentifier(id)
		if diff := cmp.Diff(original, parsed); diff != "" {
			t.Errorf("round-trip differs (-want +got):\n%s", diff)
		}
	})

	t.Run("step identifier", func(t *testing.T) {
		original := &StepIdentifier{
			FilePath:      "scenarios/test.yaml",
			ScenarioIndex: 1,
			StepIndex:     3,
			Title:         "Login Test",
		}
		id := BuildStepIdentifier(original.FilePath, original.ScenarioIndex, original.StepIndex, original.Title)
		parsed := ParseStepIdentifier(id)
		if diff := cmp.Diff(original, parsed); diff != "" {
			t.Errorf("round-trip differs (-want +got):\n%s", diff)
		}
	})
}
