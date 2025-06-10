package reporter

import (
	"time"
)

// SerializableReporter represents a serializable version of reporter struct
type SerializableReporter struct {
	Name                 string                  `json:"name"`
	GoTestName           string                  `json:"goTestName"`
	Depth                int                     `json:"depth"`
	Failed               bool                    `json:"failed"`
	Skipped              bool                    `json:"skipped"`
	IsParallel           bool                    `json:"isParallel"`
	Logs                 []string                `json:"logs,omitempty"`
	Duration             time.Duration           `json:"duration"`
	Children             []*SerializableReporter `json:"children,omitempty"`
	Testing              bool                    `json:"testing"`
	Retryable            bool                    `json:"retryable"`
	NoFailurePropagation bool                    `json:"noFailurePropagation"`
}

// ToSerializable converts reporter struct to SerializableReporter
func (r *reporter) ToSerializable() *SerializableReporter {
	sr := &SerializableReporter{
		Name:                 r.name,
		GoTestName:           r.goTestName,
		Depth:                r.depth,
		Failed:               r.Failed(),
		Skipped:              r.Skipped(),
		IsParallel:           r.isParallel,
		Testing:              r.testing,
		Retryable:            r.retryable,
		NoFailurePropagation: r.noFailurePropagation,
	}

	// Convert logs
	if r.logs != nil {
		sr.Logs = r.logs.all()
	}

	// Convert duration
	sr.Duration = r.getDuration()

	// Convert children
	if len(r.children) > 0 {
		sr.Children = make([]*SerializableReporter, 0, len(r.children))
		for _, child := range r.children {
			sr.Children = append(sr.Children, child.ToSerializable())
		}
	}

	return sr
}

// FromSerializable creates a new reporter struct from SerializableReporter
func FromSerializable(sr *SerializableReporter) *reporter {
	// Create a new reporter
	r := newReporter()
	r.name = sr.Name
	r.goTestName = sr.GoTestName
	r.depth = sr.Depth
	r.isParallel = sr.IsParallel
	r.testing = sr.Testing
	r.retryable = sr.Retryable
	r.noFailurePropagation = sr.NoFailurePropagation

	if sr.Failed {
		r.Fail()
	}
	if sr.Skipped {
		r.SkipNow()
	}

	// Set logs
	if len(sr.Logs) > 0 {
		for _, log := range sr.Logs {
			r.logs.log(log)
		}
	}

	// Set children
	if len(sr.Children) > 0 {
		r.children = make([]*reporter, 0, len(sr.Children))
		for _, childSR := range sr.Children {
			child := FromSerializable(childSR)
			child.parent = r
			r.children = append(r.children, child)
		}
	}

	return r
}
