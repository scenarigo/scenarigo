package lsp

import (
	"encoding/json"
	"strings"
)

// Code lenses.
//
// A scenario file gets a "Run" lens on its first schemaVersion line. The
// lens names a command of the client, scenarigo.runScenario, which the VS
// Code extension implements by running `scenarigo run <file>` in a terminal:
// the server never runs a scenario, and opening a file runs nothing until
// the user clicks the lens.

// configSchemaVersion is the schemaVersion of scenarigo.yaml.
const configSchemaVersion = "config/v1"

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

// runnableLine returns the line of the first schemaVersion key of a file that
// `scenarigo run` accepts: a scenario or a ytt/v1 document, not a
// configuration file. The text is read line by line, so a file that does not
// parse still gets the lens.
func runnableLine(text string) (int, bool) {
	for i, line := range strings.Split(text, "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "schemaVersion:")
		if !ok {
			continue
		}
		return i, plainScalar(value) != configSchemaVersion
	}
	return 0, false
}
