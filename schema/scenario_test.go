package schema

import (
	"strings"
	"testing"

	"github.com/scenarigo/scenarigo/protocol"
)

func TestScenario_Reset_PreservesFilepath(t *testing.T) {
	p := &testProtocol{
		name: "test",
	}
	protocol.Register(p)
	defer protocol.Unregister(p.Name())

	yml := `title: test scenario
retry:
  constant:
    interval: 10ms
    maxRetries: 1
steps:
- title: test step
  protocol: test
`
	scenarios, err := LoadScenariosFromReader(strings.NewReader(yml))
	if err != nil {
		t.Fatalf("failed to load scenario: %s", err)
	}
	if len(scenarios) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(scenarios))
	}

	scn := scenarios[0]

	// Simulate setting filepath as LoadScenarios does
	expectedFilepath := "/path/to/test.yaml"
	scn.filepath = expectedFilepath

	if err := scn.Reset(); err != nil {
		t.Fatalf("Reset() failed: %s", err)
	}

	// Verify filepath is preserved after Reset
	if scn.filepath != expectedFilepath {
		t.Errorf("filepath not preserved: expected %q, got %q", expectedFilepath, scn.filepath)
	}

	// Verify Node is updated (not nil) after Reset
	if scn.Node == nil {
		t.Error("Node is nil after Reset")
	}
}
