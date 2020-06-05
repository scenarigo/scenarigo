package schema

import (
	"io"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
	"github.com/zoncoen/scenarigo/context"
)

// LoadScenarios loads test scenarios from path.
func LoadScenarios(ctx *context.Context, path string) ([]*Scenario, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var scenarios []*Scenario
	d := yaml.NewDecoder(f, yaml.UseOrderedMap(), yaml.Strict())
	for {
		var s Scenario
		if err := d.Decode(&s); err != nil {
			if err == io.EOF {
				break
			}
			return nil, errors.Wrap(err, "failed to decode YAML")
		}
		s.filepath = path
		scenarios = append(scenarios, &s)
	}
	return scenarios, nil
}
