package protocolmeta

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
)

const (
	testTraceID     = "0af7651916cd43dd8448eb211c80319c"
	testSpanID      = "b7ad6b7169203331"
	testTraceparent = "00-" + testTraceID + "-" + testSpanID + "-01"
)

func spanCtx(t *testing.T) context.Context {
	t.Helper()
	traceID, err := trace.TraceIDFromHex(testTraceID)
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex(testSpanID)
	if err != nil {
		t.Fatal(err)
	}
	return trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))
}

func fullMeta() Metadata {
	return Metadata{
		RunID:             "run-1",
		Filepath:          "scenarios/a.yaml",
		ScenarioIndex:     new(0),
		StepIndex:         new(2),
		ScenarioTitle:     "scenario title",
		StepName:          "step name",
		RootFilepath:      "scenarios/root.yaml",
		RootScenarioIndex: new(1),
	}
}

func parseBaggage(t *testing.T, raw string) map[string]string {
	t.Helper()
	b, err := baggage.Parse(raw)
	if err != nil {
		t.Fatalf("baggage %q does not parse: %s", raw, err)
	}
	got := map[string]string{}
	for _, m := range b.Members() {
		got[m.Key()] = m.Value()
	}
	return got
}

func TestInject(t *testing.T) {
	scenarigoMembers := map[string]string{
		"scenarigo.run_id":              "run-1",
		"scenarigo.filepath":            "scenarios/a.yaml",
		"scenarigo.scenario_index":      "0",
		"scenarigo.step_index":          "2",
		"scenarigo.scenario_title":      "scenario title",
		"scenarigo.step_name":           "step name",
		"scenarigo.root_filepath":       "scenarios/root.yaml",
		"scenarigo.root_scenario_index": "1",
	}
	withUser := func(m map[string]string) map[string]string {
		out := map[string]string{"user": "1", "scenarigo.run_id": "stale"}
		maps.Copy(out, m)
		return out
	}
	userHeader := http.Header{
		"Traceparent": {"00-11111111111111111111111111111111-2222222222222222-01"},
		"Tracestate":  {"vendor=x"},
		"Baggage":     {"user=1,scenarigo.run_id=stale"},
	}

	tests := map[string]struct {
		opts        InjectOptions
		header      http.Header
		ctx         context.Context
		traceparent string
		tracestate  string
		baggage     map[string]string // nil: no baggage header
	}{
		"both on, no user headers": {
			opts:        InjectOptions{Traceparent: true, Baggage: true},
			header:      http.Header{},
			traceparent: testTraceparent,
			baggage:     scenarigoMembers,
		},
		"both on, user headers replaced and merged": {
			opts:        InjectOptions{Traceparent: true, Baggage: true},
			header:      userHeader.Clone(),
			traceparent: testTraceparent,
			baggage:     withUser(scenarigoMembers),
		},
		"traceparent only, user baggage untouched": {
			opts:        InjectOptions{Traceparent: true},
			header:      userHeader.Clone(),
			traceparent: testTraceparent,
			baggage:     map[string]string{"user": "1", "scenarigo.run_id": "stale"},
		},
		"baggage only, user traceparent and tracestate untouched": {
			opts:        InjectOptions{Baggage: true},
			header:      userHeader.Clone(),
			traceparent: "00-11111111111111111111111111111111-2222222222222222-01",
			tracestate:  "vendor=x",
			baggage:     withUser(scenarigoMembers),
		},
		"both off, nothing written": {
			opts:   InjectOptions{},
			header: http.Header{},
		},
		"both off, user headers untouched": {
			opts:        InjectOptions{},
			header:      userHeader.Clone(),
			traceparent: "00-11111111111111111111111111111111-2222222222222222-01",
			tracestate:  "vendor=x",
			baggage:     map[string]string{"user": "1", "scenarigo.run_id": "stale"},
		},
		"no span in context: user traceparent kept, baggage written": {
			opts:        InjectOptions{Traceparent: true, Baggage: true},
			header:      userHeader.Clone(),
			ctx:         context.Background(),
			traceparent: "00-11111111111111111111111111111111-2222222222222222-01",
			tracestate:  "vendor=x",
			baggage:     withUser(scenarigoMembers),
		},
		"user tracestate without traceparent is dropped with the new traceparent": {
			opts:        InjectOptions{Traceparent: true},
			header:      http.Header{"Tracestate": {"vendor=x"}},
			traceparent: testTraceparent,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := test.ctx
			if ctx == nil {
				ctx = spanCtx(t)
			}
			warnings := Inject(ctx, HeaderCarrier(test.header), fullMeta(), test.opts)
			if len(warnings) > 0 {
				t.Errorf("unexpected warnings: %v", warnings)
			}
			if got := test.header.Get("Traceparent"); got != test.traceparent {
				t.Errorf("traceparent = %q, want %q", got, test.traceparent)
			}
			if got := test.header.Get("Tracestate"); got != test.tracestate {
				t.Errorf("tracestate = %q, want %q", got, test.tracestate)
			}
			if test.baggage == nil {
				if got := test.header.Values("Baggage"); len(got) > 0 {
					t.Errorf("baggage must be absent, got %v", got)
				}
				return
			}
			if got := len(test.header.Values("Baggage")); got != 1 {
				t.Fatalf("baggage must be a single header value, got %d", got)
			}
			if diff := cmp.Diff(test.baggage, parseBaggage(t, test.header.Get("Baggage"))); diff != "" {
				t.Errorf("baggage members differ (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInject_BaggageEncoding(t *testing.T) {
	header := http.Header{}
	meta := Metadata{
		RunID:         "run-1",
		Filepath:      "dir/100%/a b,c;d.yaml",
		ScenarioTitle: "日本語 title", //nolint:gosmopolitan // a non-ASCII value is what the encoding test needs
	}
	if w := Inject(spanCtx(t), HeaderCarrier(header), meta, InjectOptions{Baggage: true}); len(w) > 0 {
		t.Fatalf("unexpected warnings: %v", w)
	}
	raw := header.Get("Baggage")
	for _, forbidden := range []string{" ", "\"", ";", "\\"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("raw baggage %q contains forbidden %q", raw, forbidden)
		}
	}
	if strings.Count(raw, ",") != 2 {
		t.Errorf("raw baggage %q must have exactly two member separators", raw)
	}
	got := parseBaggage(t, raw)
	if got["scenarigo.filepath"] != meta.Filepath || got["scenarigo.scenario_title"] != meta.ScenarioTitle {
		t.Errorf("round trip mismatch: %v", got)
	}
}

func TestInject_BaggageLimits(t *testing.T) {
	// What scenarigo adds for fullMeta, measured rather than assumed.
	probe := http.Header{}
	if w := Inject(spanCtx(t), HeaderCarrier(probe), fullMeta(), InjectOptions{Baggage: true}); len(w) > 0 {
		t.Fatalf("unexpected warnings: %v", w)
	}
	ours := probe.Get("Baggage")
	ourMembers := len(parseBaggage(t, ours))

	// userBaggage builds n members whose joined form is exactly size bytes.
	userBaggage := func(n, size int) string {
		parts := make([]string, 0, n)
		for i := range n {
			parts = append(parts, fmt.Sprintf("k%03d=v", i))
		}
		s := strings.Join(parts, ",")
		if len(s) > size {
			t.Fatalf("cannot build %d members in %d bytes", n, size)
		}
		return s + strings.Repeat("w", size-len(s))
	}
	roomBytes := maxBaggageBytes - len(ours) - 1 // one separator between the user's members and ours
	roomMembers := maxBaggageMembers - ourMembers

	tests := map[string]struct {
		values      []string
		expectAdded bool
		warning     string
	}{
		"fills exactly 8192 bytes":  {values: []string{userBaggage(1, roomBytes)}, expectAdded: true},
		"one byte over 8192":        {values: []string{userBaggage(1, roomBytes+1)}, warning: "would be 8193 bytes"},
		"user value alone too long": {values: []string{userBaggage(1, maxBaggageBytes+1)}, warning: "is 8193 bytes"},
		"fills exactly 64 members":  {values: []string{userBaggage(roomMembers, roomMembers*8)}, expectAdded: true},
		"one member over 64":        {values: []string{userBaggage(roomMembers+1, (roomMembers+1)*8)}, warning: "would have 65 members"},
		"user members alone too many": {
			values:  []string{userBaggage(maxBaggageMembers+1, (maxBaggageMembers+1)*8)},
			warning: "has 65 members",
		},
		"multiple header values count together": {
			values:  []string{userBaggage(40, 40*8), userBaggage(25, 25*8)},
			warning: "has 65 members",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			header := http.Header{"Baggage": test.values}
			before := append([]string(nil), test.values...)
			warnings := Inject(spanCtx(t), HeaderCarrier(header), fullMeta(), InjectOptions{Baggage: true})
			if test.expectAdded {
				if len(warnings) > 0 {
					t.Fatalf("unexpected warnings: %v", warnings)
				}
				got := parseBaggage(t, header.Get("Baggage"))
				if got["scenarigo.run_id"] != "run-1" {
					t.Errorf("scenarigo members must be added: %v", header.Get("Baggage"))
				}
				return
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], test.warning) {
				t.Fatalf("expected one warning containing %q, got %v", test.warning, warnings)
			}
			if diff := cmp.Diff(before, header.Values("Baggage")); diff != "" {
				t.Errorf("user baggage must be left untouched (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInject_DropOrder(t *testing.T) {
	long := strings.Repeat("x", 2000)
	tests := map[string]struct {
		meta      Metadata
		title     bool
		stepName  bool
		truncated bool
	}{
		"fits": {
			meta:  Metadata{RunID: "run-1", Filepath: "a.yaml", ScenarioTitle: "t", StepName: "s"},
			title: true, stepName: true,
		},
		"title dropped first": {
			meta:     Metadata{RunID: "run-1", Filepath: "a.yaml", ScenarioTitle: long, StepName: "s"},
			stepName: true, truncated: true,
		},
		"then step name": {
			meta:      Metadata{RunID: "run-1", Filepath: "a.yaml", ScenarioTitle: long, StepName: long},
			truncated: true,
		},
		"required members are never dropped": {
			meta:      Metadata{RunID: "run-1", Filepath: strings.Repeat("d/", 1500) + "a.yaml", ScenarioTitle: "t", StepName: "s"},
			truncated: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			header := http.Header{}
			if w := Inject(spanCtx(t), HeaderCarrier(header), test.meta, InjectOptions{Baggage: true}); len(w) > 0 {
				t.Fatalf("unexpected warnings: %v", w)
			}
			got := parseBaggage(t, header.Get("Baggage"))
			if got["scenarigo.run_id"] != "run-1" || got["scenarigo.filepath"] != test.meta.Filepath {
				t.Errorf("required members missing: %v", got)
			}
			if _, ok := got["scenarigo.scenario_title"]; ok != test.title {
				t.Errorf("scenario_title present=%t, want %t", ok, test.title)
			}
			if _, ok := got["scenarigo.step_name"]; ok != test.stepName {
				t.Errorf("step_name present=%t, want %t", ok, test.stepName)
			}
			if v, ok := got["scenarigo.truncated"]; ok != test.truncated || (ok && v != "true") {
				t.Errorf("truncated=%q present=%t, want present %t", v, ok, test.truncated)
			}
		})
	}
}

func TestInject_RootMembersAreRequired(t *testing.T) {
	// A required member alone exceeds the budget, so every optional member is dropped.
	meta := fullMeta()
	meta.Filepath = strings.Repeat("d/", 1500) + "a.yaml"
	header := http.Header{}
	if w := Inject(spanCtx(t), HeaderCarrier(header), meta, InjectOptions{Baggage: true}); len(w) > 0 {
		t.Fatalf("unexpected warnings: %v", w)
	}
	got := parseBaggage(t, header.Get("Baggage"))
	if got["scenarigo.truncated"] != "true" || got["scenarigo.scenario_title"] != "" || got["scenarigo.step_name"] != "" {
		t.Fatalf("the optional members must be dropped: %v", got)
	}
	if got["scenarigo.root_filepath"] != "scenarios/root.yaml" || got["scenarigo.root_scenario_index"] != "1" {
		t.Errorf("root members must survive truncation: %v", got)
	}
}

func TestInject_MDCarrier(t *testing.T) {
	md := metadata.Pairs(
		"traceparent", "00-11111111111111111111111111111111-2222222222222222-01",
		"tracestate", "vendor=x",
		"baggage", "user=1",
		"baggage", "other=2",
	)
	warnings := Inject(spanCtx(t), MDCarrier(md), fullMeta(), InjectOptions{Traceparent: true, Baggage: true})
	if len(warnings) > 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if diff := cmp.Diff([]string{testTraceparent}, md.Get("traceparent")); diff != "" {
		t.Errorf("traceparent (-want +got):\n%s", diff)
	}
	if got := md.Get("tracestate"); len(got) != 0 {
		t.Errorf("tracestate must be deleted, got %v", got)
	}
	values := md.Get("baggage")
	if len(values) != 1 {
		t.Fatalf("baggage must be merged into one value, got %v", values)
	}
	got := parseBaggage(t, values[0])
	if got["user"] != "1" || got["other"] != "2" || got["scenarigo.run_id"] != "run-1" {
		t.Errorf("baggage members: %v", got)
	}
}

func TestStartClientSpan(t *testing.T) {
	t.Run("no span in context", func(t *testing.T) {
		_, span := StartClientSpan(context.Background(), "scope", "name")
		if span.SpanContext().IsValid() {
			t.Error("span must be invalid without a parent span")
		}
		header := http.Header{}
		Inject(trace.ContextWithSpan(context.Background(), span), HeaderCarrier(header), Metadata{}, InjectOptions{Traceparent: true})
		if got := header.Get("Traceparent"); got != "" {
			t.Errorf("traceparent must not be written, got %q", got)
		}
	})
}

func TestInject_InvalidUserMember(t *testing.T) {
	h := http.Header{"Baggage": {"good=1,no-equals-sign,other=2"}}
	warnings := Inject(spanCtx(t), HeaderCarrier(h), fullMeta(), InjectOptions{Baggage: true})
	if len(warnings) != 1 || !strings.Contains(warnings[0], `"no-equals-sign"`) {
		t.Fatalf("expected one warning naming the invalid member, got %v", warnings)
	}
	got := parseBaggage(t, h.Get("Baggage"))
	for _, k := range []string{"good", "other", "scenarigo.run_id"} {
		if _, ok := got[k]; !ok {
			t.Errorf("member %q missing: %v", k, got)
		}
	}
}

func TestInject_ReplacesStaleScenarigoMembers(t *testing.T) {
	header := http.Header{}
	header.Set("Baggage", "scenarigo.step_name=old,scenarigo.truncated=true,user=1")
	meta := Metadata{RunID: "run-1", Filepath: "s.yaml"}
	if warnings := Inject(spanCtx(t), HeaderCarrier(header), meta, InjectOptions{Baggage: true}); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	expect := map[string]string{"user": "1", "scenarigo.run_id": "run-1", "scenarigo.filepath": "s.yaml"}
	if diff := cmp.Diff(expect, parseBaggage(t, header.Get("Baggage"))); diff != "" {
		t.Errorf("stale scenarigo.* members must go (-want +got):\n%s", diff)
	}
}

func TestInject_TruncatedMarkerFitsTheBudget(t *testing.T) {
	// required members: "scenarigo.run_id=r" and "scenarigo.filepath=f", 39 bytes with
	// the separator. With a 1980-byte step name the scenarigo part is 2040 bytes: within
	// 2048 on its own, over it once ",scenarigo.truncated=true" (25 bytes) joins.
	meta := Metadata{RunID: "r", Filepath: "f", ScenarioTitle: strings.Repeat("a", 100), StepName: strings.Repeat("b", 1980)}
	header := http.Header{}
	Inject(spanCtx(t), HeaderCarrier(header), meta, InjectOptions{Baggage: true})
	got := parseBaggage(t, header.Get("Baggage"))
	if _, ok := got["scenarigo.scenario_title"]; ok {
		t.Error("scenario_title must be dropped first")
	}
	if _, ok := got["scenarigo.step_name"]; ok {
		t.Error("step_name must be dropped: it does not fit next to the truncated marker")
	}
	if got["scenarigo.truncated"] != "true" {
		t.Errorf("truncated = %q", got["scenarigo.truncated"])
	}
	if n := len(header.Get("Baggage")); n > 2048 {
		t.Errorf("scenarigo members take %d bytes, limit 2048", n)
	}
}
