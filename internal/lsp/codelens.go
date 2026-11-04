package lsp

import (
	"encoding/json"
	"strings"
)

// Code lenses.
//
// A scenario file gets a "Run" lens on its schemaVersion line. The
// lens names a command of the client, scenarigo.runScenario, which the VS
// Code extension implements by running `scenarigo run <file>` in a terminal:
// the server never runs a scenario, and opening a file runs nothing until
// the user clicks the lens.

// The schemaVersion values of scenarigo.yaml and of the files `scenarigo
// run` accepts.
const (
	configSchemaVersion   = "config/v1"
	scenarioSchemaVersion = "scenario/v1"
	yttSchemaVersion      = "ytt/v1"
)

// runScenarioCommand is the client command of the Run lens.
const runScenarioCommand = "scenarigo.runScenario"

func (s *Server) handleCodeLens(req *Request) {
	var params CodeLensParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendResponse(req.ID, nil, invalidParams("codeLens", err))
		return
	}
	lenses := []CodeLens{}
	if doc := s.docs.Get(params.TextDocument.URI); doc != nil {
		if line, ok := runnableLine(doc.Text); ok {
			lenses = append(lenses, CodeLens{
				Range:   Range{Start: Position{Line: line}, End: Position{Line: line}},
				Command: &Command{Title: "Run", Command: runScenarioCommand, Arguments: []any{doc.URI}},
			})
		}
	}
	s.sendResponse(req.ID, lenses, nil)
}

// runnableLine returns the line of the first top-level schemaVersion key of
// a file that `scenarigo run` accepts: scenario/v1 or ytt/v1, not a
// configuration file or another tool's YAML that also has a schemaVersion
// (a devfile). Only a key at the start of a line counts, so that an
// indented one (in a block scalar or a nested mapping) does not. The text is
// read line by line, so a file that does not parse still gets the lens.
func runnableLine(text string) (int, bool) {
	for i, line := range strings.Split(text, "\n") {
		value, ok := strings.CutPrefix(line, "schemaVersion:")
		if !ok {
			continue
		}
		switch schemaVersionValue(value) {
		case scenarioSchemaVersion, yttSchemaVersion:
			return i, true
		default:
			return 0, false
		}
	}
	return 0, false
}

// schemaVersionValue returns the value of a schemaVersion line: without a
// comment (after a space or a tab), an anchor or a tag, and quotes.
func schemaVersionValue(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"&", "!"} {
		if strings.HasPrefix(value, prefix) {
			_, value, _ = strings.Cut(value, " ")
			value = strings.TrimSpace(value)
		}
	}
	if !strings.HasPrefix(value, `"`) && !strings.HasPrefix(value, "'") {
		for i := 1; i < len(value); i++ {
			if value[i] == '#' && (value[i-1] == ' ' || value[i-1] == '\t') {
				value = value[:i]
				break
			}
		}
	}
	return plainScalar(value)
}
