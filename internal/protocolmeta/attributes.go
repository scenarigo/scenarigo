package protocolmeta

import "go.opentelemetry.io/otel/attribute"

// Span attribute keys shared by node spans (reporter) and request spans (protocols).
// The baggage member keys below use the same names so that a consumer can correlate
// what it sees on the wire with what it sees in a trace backend.
const (
	AttrRunID         = attribute.Key("scenarigo.run_id")
	AttrNode          = attribute.Key("scenarigo.node")
	AttrFilepath      = attribute.Key("scenarigo.filepath")
	AttrScenarioIndex = attribute.Key("scenarigo.scenario_index")
	AttrScenarioTitle = attribute.Key("scenarigo.scenario_title")
	AttrStepIndex     = attribute.Key("scenarigo.step_index")
	AttrStepName      = attribute.Key("scenarigo.step_name")
	AttrSkipped       = attribute.Key("scenarigo.skipped")
	AttrAttempt       = attribute.Key("scenarigo.attempt")
)

// Baggage member keys. All members scenarigo writes share the "scenarigo." prefix;
// members with other keys are the user's and are never touched.
const (
	BaggageRunIDKey         = "scenarigo.run_id"
	BaggageFilepathKey      = "scenarigo.filepath"
	BaggageScenarioIndexKey = "scenarigo.scenario_index"
	BaggageStepIndexKey     = "scenarigo.step_index"
	BaggageScenarioTitleKey = "scenarigo.scenario_title"
	BaggageStepNameKey      = "scenarigo.step_name"
	BaggageTruncatedKey     = "scenarigo.truncated"
	// BaggageRootFilepathKey and BaggageRootScenarioIndexKey name the top-level
	// scenario, the root of the trace. They have no attribute counterpart: in a
	// trace backend that scenario is the root span of the trace the request belongs to.
	BaggageRootFilepathKey      = "scenarigo.root_filepath"
	BaggageRootScenarioIndexKey = "scenarigo.root_scenario_index"
)

// W3C header names. gRPC metadata uses the same (lowercase) keys.
const (
	TraceparentHeader = "traceparent"
	TracestateHeader  = "tracestate"
	BaggageHeader     = "baggage"
)
