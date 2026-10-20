package protocolmeta

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// W3C Baggage limits a receiver MUST accept. Larger input is not truncated by the
// OpenTelemetry parser: it is dropped as a whole, so scenarigo checks the raw value first.
const (
	maxBaggageBytes   = 8192
	maxBaggageMembers = 64

	// targetBaggageBytes bounds what scenarigo itself adds, leaving room for the user's members.
	targetBaggageBytes = 2048
)

// Metadata is what scenarigo puts on the wire for one request.
type Metadata struct {
	RunID string
	// Filepath is already normalized (relative to the config directory, "/" separators).
	Filepath      string
	ScenarioIndex *int
	StepIndex     *int
	ScenarioTitle string
	StepName      string
	// RootFilepath and RootScenarioIndex name the top-level scenario. Inside an
	// included scenario they differ from Filepath and ScenarioIndex, which
	// describe the included file.
	RootFilepath      string
	RootScenarioIndex *int
}

// InjectOptions selects which headers Inject writes.
type InjectOptions struct {
	Traceparent bool
	Baggage     bool
}

// Inject writes the W3C traceparent and baggage headers into carrier.
// ctx must carry the span whose span_id goes on the wire (the request span);
// without a valid span context the traceparent is left untouched.
// A user-supplied traceparent is replaced together with its tracestate: that
// tracestate belongs to the trace being discarded. User baggage members are kept;
// only "scenarigo.*" members are replaced. The returned strings are warnings for
// the caller to log.
func Inject(ctx context.Context, carrier Carrier, meta Metadata, opts InjectOptions) []string {
	var warnings []string
	if opts.Traceparent && trace.SpanContextFromContext(ctx).IsValid() {
		carrier.Delete(TraceparentHeader)
		carrier.Delete(TracestateHeader)
		propagation.TraceContext{}.Inject(ctx, carrier)
	}
	if opts.Baggage {
		warnings = append(warnings, injectBaggage(ctx, carrier, meta)...)
	}
	return warnings
}

// scenarigoMemberPrefix marks the baggage members scenarigo owns and replaces.
const scenarigoMemberPrefix = "scenarigo."

func injectBaggage(ctx context.Context, carrier Carrier, meta Metadata) []string {
	raw := strings.Join(carrier.Values(BaggageHeader), ",")
	if n := len(raw); n > maxBaggageBytes {
		return []string{fmt.Sprintf("baggage header is %d bytes (limit %d): scenarigo.* members are not added, and receivers drop the whole header", n, maxBaggageBytes)}
	}
	if n := countMembers(raw); n > maxBaggageMembers {
		return []string{fmt.Sprintf("baggage header has %d members (limit %d): scenarigo.* members are not added, and receivers drop the members beyond the limit", n, maxBaggageMembers)}
	}

	var warnings []string
	for _, v := range carrier.Values(BaggageHeader) {
		for part := range strings.SplitSeq(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, err := baggage.Parse(part); err != nil {
				warnings = append(warnings, fmt.Sprintf("invalid baggage member %q is dropped: %s", part, err))
			}
		}
	}
	bag := baggage.FromContext(propagation.Baggage{}.Extract(context.Background(), carrier))
	// A scenarigo.* member written in the scenario describes another request:
	// only the members computed for this one stay.
	for _, m := range bag.Members() {
		if strings.HasPrefix(m.Key(), scenarigoMemberPrefix) {
			bag = bag.DeleteMember(m.Key())
		}
	}
	required, optional, memberWarnings := meta.members()
	warnings = append(warnings, memberWarnings...)
	members, truncated := fitMembers(required, optional)
	if truncated {
		if m, err := baggage.NewMemberRaw(BaggageTruncatedKey, "true"); err == nil {
			members = append(members, m)
		}
	}
	for _, m := range members {
		var err error
		bag, err = bag.SetMember(m)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("failed to set baggage member %s: %s", m.Key(), err))
		}
	}

	// Encode into a scratch carrier first: when the merged value would break a
	// receiver limit, the user's header is left as it was.
	scratch := propagation.MapCarrier{}
	propagation.Baggage{}.Inject(baggage.ContextWithBaggage(ctx, bag), scratch)
	merged := scratch.Get(BaggageHeader)
	if n := len(bag.Members()); n > maxBaggageMembers {
		return append(warnings, fmt.Sprintf("baggage header would have %d members with scenarigo.* members (limit %d): scenarigo.* members are not added", n, maxBaggageMembers))
	}
	if n := len(merged); n > maxBaggageBytes {
		return append(warnings, fmt.Sprintf("baggage header would be %d bytes with scenarigo.* members (limit %d): scenarigo.* members are not added", n, maxBaggageBytes))
	}
	carrier.Delete(BaggageHeader)
	if merged != "" {
		carrier.Set(BaggageHeader, merged)
	}
	return warnings
}

// members returns the required members, the optional members in drop order, and
// warnings for values that cannot be carried (invalid UTF-8).
func (m Metadata) members() ([]baggage.Member, []baggage.Member, []string) {
	var required, optional []baggage.Member
	var warnings []string
	add := func(dst *[]baggage.Member, key, value string) {
		if value == "" {
			return
		}
		member, err := baggage.NewMemberRaw(key, value)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skip baggage member %s: %s", key, err))
			return
		}
		*dst = append(*dst, member)
	}
	add(&required, BaggageRunIDKey, m.RunID)
	add(&required, BaggageFilepathKey, m.Filepath)
	if m.ScenarioIndex != nil {
		add(&required, BaggageScenarioIndexKey, strconv.Itoa(*m.ScenarioIndex))
	}
	if m.StepIndex != nil {
		add(&required, BaggageStepIndexKey, strconv.Itoa(*m.StepIndex))
	}
	add(&required, BaggageRootFilepathKey, m.RootFilepath)
	if m.RootScenarioIndex != nil {
		add(&required, BaggageRootScenarioIndexKey, strconv.Itoa(*m.RootScenarioIndex))
	}
	add(&optional, BaggageScenarioTitleKey, m.ScenarioTitle)
	add(&optional, BaggageStepNameKey, m.StepName)
	return required, optional, warnings
}

// fitMembers drops optional members, first to last, until the encoded size of what
// scenarigo adds is within targetBaggageBytes. Required members are never dropped.
// Once a member is dropped, the truncated marker the caller appends takes its
// share of the budget.
func fitMembers(required, optional []baggage.Member) ([]baggage.Member, bool) {
	truncated := false
	limit := targetBaggageBytes
	for len(optional) > 0 && encodedLen(required, optional) > limit {
		optional = optional[1:]
		truncated = true
		limit = targetBaggageBytes - len(","+BaggageTruncatedKey+"=true")
	}
	return append(required, optional...), truncated
}

func encodedLen(lists ...[]baggage.Member) int {
	n := 0
	count := 0
	for _, members := range lists {
		for _, m := range members {
			n += len(m.String())
			count++
		}
	}
	if count > 1 {
		n += count - 1 // separators
	}
	return n
}

func countMembers(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	n := 0
	for part := range strings.SplitSeq(raw, ",") {
		if strings.TrimSpace(part) != "" {
			n++
		}
	}
	return n
}

// StartClientSpan starts a SpanKindClient span as a child of the span carried by
// ctx, using that span's TracerProvider so that the global provider is never
// touched. Without a valid span context in ctx the result is a no-op span whose
// context is invalid, and Inject then writes no traceparent. A non-recording
// span with a valid context (a reporter restored on the WASM guest) keeps that
// context, so Inject names the restored node's span.
func StartClientSpan(ctx context.Context, scope, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	tracer := trace.SpanFromContext(ctx).TracerProvider().Tracer(scope)
	return tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...)) //nolint:spancheck // the caller ends the span
}
