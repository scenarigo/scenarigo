package context

import "time"

// CurrentStep represents the current running step in the context.
type CurrentStep struct {
	Index int

	// from github.com/scenarigo/scenarigo/schema.Step
	ID                      string
	Title                   string
	Description             string
	If                      string
	ContinueOnError         bool
	Vars                    map[string]any
	Secrets                 map[string]any
	Protocol                string
	Request                 any
	Expect                  any
	Include                 string
	Ref                     any
	BindVars                map[string]any
	BindSecrets             map[string]any
	Timeout                 *time.Duration
	PostTimeoutWaitingLimit *time.Duration
	Retry                   any
}
