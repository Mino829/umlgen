package sequencego

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/activity"
	"github.com/Mino829/umlgen/internal/sequence"
	"github.com/Mino829/umlgen/internal/state"
)

func stateFor(t *testing.T, source string) state.Analysis {
	t.Helper()
	result, err := AnalyzeState(context.Background(), selected(t, map[string]string{"state.go": source}), "example.test/app.Connection", "state", 500)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func stateDiagnostic(result state.Analysis, code string) bool {
	for _, d := range result.Diagnostics {
		if d.Code == code {
			return true
		}
	}
	return false
}

func stateGraph(t *testing.T, result state.Analysis, id string) activity.Graph {
	t.Helper()
	for _, g := range result.Functions {
		if g.Function.ID == id {
			return g
		}
	}
	t.Fatalf("missing graph for %s", id)
	return activity.Graph{}
}

func TestStateConnectionFixture(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "state", "go", "connection.go"))
	if err != nil {
		t.Fatal(err)
	}
	sources := selected(t, map[string]string{"connection.go": string(content)})
	result, err := AnalyzeState(context.Background(), sources, "Connection", "state", 500)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]string{}
	for _, value := range result.Values {
		keys[value.Name] = value.Key
	}
	if len(result.Values) != 4 || keys["Closed"] == "" || keys["Closed"] != keys["AlsoClosed"] || keys["Unused"] == "" || keys["Open"] == keys["Closed"] {
		t.Fatalf("constant evidence: %#v", result.Values)
	}
	if len(result.Updates) != 3 || len(result.Functions) != 3 {
		t.Fatalf("candidate functions: %#v", result)
	}
	for _, u := range result.Updates {
		if u.Kind != "assignment" || u.Receiver != "c" || len(u.NodeIDs) != 1 {
			t.Fatalf("invalid candidate: %#v", u)
		}
		g := stateGraph(t, result, u.FunctionID)
		if err := g.Validate(500); err != nil {
			t.Fatal(err)
		}
		if g.Function.Body != nil || g.Files != nil {
			t.Fatalf("CFG must reuse declaration without sequence projection: %#v", g)
		}
		if !activityPath(g, g.Entry, u.NodeIDs[0]) {
			t.Fatalf("candidate disconnected: %#v", u)
		}
		switch u.Expression {
		case "Open":
			if u.ValueKey != keys["Open"] || !reflect.DeepEqual(u.Guards, []string{"if (c.state == Closed)"}) {
				t.Fatalf("Open evidence: %#v", u)
			}
		case "Closed":
			if u.ValueKey != keys["Closed"] || !reflect.DeepEqual(u.Guards, []string{"if (c.state == Open)"}) {
				t.Fatalf("Close evidence: %#v", u)
			}
		case "next":
			if u.ValueKey != "" || len(u.Guards) != 0 {
				t.Fatalf("unknown RHS became proof: %#v", u)
			}
		default:
			t.Fatalf("unexpected candidate: %#v", u)
		}
	}
	if !stateDiagnostic(result, "state-boundary") || !stateDiagnostic(result, "state-receiver-identity") {
		t.Fatal("missing source/runtime boundary")
	}
	second, err := AnalyzeState(context.Background(), sources, result.Target.ID, "state", 500)
	if err != nil || !reflect.DeepEqual(result, second) {
		t.Fatalf("canonical selection/determinism: %v", err)
	}
}

func TestStateSemanticFieldShadowingAndOtherFunctions(t *testing.T) {
	result := stateFor(t, `package flow
 type State int
 const (Closed State = iota; Open; Unused)
 type Connection struct { state State }
 type Other struct { state State }
 func (c *Connection) Update(other *Connection, unrelated *Other) {
  c.state = Open
  other.state = Closed
  unrelated.state = Open
  { c := Other{}; c.state = Open }
  { type Connection struct { state State }; c := Connection{}; c.state = Open }
 }
 func Elsewhere(c *Connection, next State) { c.state = next }
 `)
	if len(result.Updates) != 3 {
		t.Fatalf("shadowed fields leaked into evidence: %#v", result.Updates)
	}
	if result.Updates[0].Receiver != "c" || result.Updates[1].Receiver != "other" || result.Updates[2].Expression != "next" || result.Updates[2].ValueKey != "" {
		t.Fatalf("direct/other-function evidence: %#v", result.Updates)
	}
	if result.Updates[0].FunctionID != result.Updates[1].FunctionID || result.Updates[0].FunctionID == result.Updates[2].FunctionID {
		t.Fatal("incorrect callable binding")
	}
}

func TestStateTypeAndFieldSelection(t *testing.T) {
	first := selected(t, map[string]string{"state.go": `package flow; type State string; const Open State = "one"; type Connection struct { state State }; func Set(c *Connection) { c.state = Open }`})
	second := selected(t, map[string]string{"state.go": `package flow; type State string; const Open State = "two"; type Connection struct { state State }; func Set(c *Connection) { c.state = Open }`})
	one, err := AnalyzeState(context.Background(), first, "Connection", "state", 500)
	if err != nil {
		t.Fatal(err)
	}
	combined := append(append([]sequence.Source(nil), second...), first...)
	for _, selector := range []string{"Connection", "example.test/app.Connection"} {
		partial, err := AnalyzeState(context.Background(), combined, selector, "state", 500)
		if !errors.Is(err, state.ErrSelection) || !errors.Is(err, state.ErrAmbiguous) || len(partial.Files) != 2 {
			t.Fatalf("ambiguous %s: %#v %v", selector, partial, err)
		}
	}
	bound, err := AnalyzeState(context.Background(), combined, one.Target.ID, "state", 500)
	if err != nil || len(bound.Updates) != 1 || len(bound.Values) != 1 || bound.Values[0].Literal != `"one"` {
		t.Fatalf("cross-root objects merged: %#v %v", bound, err)
	}
	for _, field := range []string{"", "missing"} {
		partial, err := AnalyzeState(context.Background(), first, "Connection", field, 500)
		if !errors.Is(err, state.ErrSelection) || partial.Target.ID == "" || len(partial.Files) != 1 {
			t.Fatalf("field %q: %#v %v", field, partial, err)
		}
	}
	for _, source := range []string{
		`package flow; type Connection int`,
		`package flow; type Connection = struct { state string }`,
		`package flow; type Connection struct { state []string }`,
		`package flow; type Embedded struct { state string }; type Connection struct { Embedded }`,
	} {
		_, err := AnalyzeState(context.Background(), selected(t, map[string]string{"state.go": source}), "Connection", "state", 500)
		if !errors.Is(err, state.ErrSelection) {
			t.Fatalf("unsupported selection accepted: %s: %v", source, err)
		}
	}
	_, err = AnalyzeState(context.Background(), selected(t, map[string]string{"state.go": `package flow; type Connection struct { state string; state int }`}), "Connection", "state", 500)
	if !errors.Is(err, state.ErrAmbiguous) {
		t.Fatalf("duplicate field selection: %v", err)
	}
}

func TestStateConstantsLiteralsAndUnknownUpdates(t *testing.T) {
	result := stateFor(t, `package flow
 type State int8
 const (Closed State = 0; Open State = 1; Unused State = 2)
 const Alias = 1
 const TooLarge = 128
 type Other int8
 const Foreign Other = 1
 type Connection struct { state State }
 func Set(c *Connection, next State) {
  c.state = 1
  c.state = 1.0
  c.state = Alias
  c.state = next
  c.state += Open
  c.state++
  switch c.state { case Closed: c.state = Open; default: c.state = -1 }
 }
 `)
	keys := map[string]string{}
	for _, value := range result.Values {
		keys[value.Name] = value.Key
	}
	if keys["TooLarge"] != "" || keys["Foreign"] != "" || keys["Unused"] == "" || keys["Open"] != keys["Alias"] {
		t.Fatalf("compiler constant binding: %#v", result.Values)
	}
	if len(result.Updates) != 8 {
		t.Fatalf("updates: %#v", result.Updates)
	}
	for _, i := range []int{0, 1, 2} {
		if result.Updates[i].ValueKey != keys["Open"] {
			t.Fatalf("exact scalar equality lost: %#v", result.Updates[i])
		}
	}
	for _, i := range []int{3, 4, 5} {
		if result.Updates[i].ValueKey != "" {
			t.Fatalf("unknown/compound update invented value: %#v", result.Updates[i])
		}
	}
	if !reflect.DeepEqual(result.Updates[6].Guards, []string{"switch c.state", "case Closed"}) || !reflect.DeepEqual(result.Updates[7].Guards, []string{"switch c.state", "default"}) {
		t.Fatalf("case context: %#v", result.Updates)
	}
}

func TestStateInitializersClosuresAndAliasCaveats(t *testing.T) {
	result := stateFor(t, `package flow
 type State string
 const (Closed State = "closed"; Open State = "open")
 type Connection struct { state State }
 var Global = Connection{state: Closed}
 var Zero Connection
 func Factory(c *Connection, ok bool) *Connection {
  p := &c.state
  *p = Open
  copy := c
  copy.state = Open
  if ok {
   f := func() { c.state = Closed }
   go f()
  }
  defer func() { c.state = Open }()
  _ = Connection{}
  return &Connection{Open}
 }
 `)
	if len(result.Updates) != 5 {
		t.Fatalf("initializer/closure candidates: %#v", result.Updates)
	}
	global := result.Updates[0]
	if global.Kind != "initializer" || global.FunctionID != "" || len(global.NodeIDs) != 0 {
		t.Fatalf("global initializer acquired function execution: %#v", global)
	}
	var closureIDs []string
	for _, u := range result.Updates {
		if strings.Contains(u.FunctionID, ".func@") {
			closureIDs = append(closureIDs, u.FunctionID)
			if len(u.Guards) != 0 || len(u.NodeIDs) != 1 {
				t.Fatalf("closure inherited outer execution: %#v", u)
			}
		}
	}
	if len(closureIDs) != 2 || closureIDs[0] == closureIDs[1] {
		t.Fatalf("closure callable identity: %v", closureIDs)
	}
	for _, code := range []string{"state-initialization", "state-implicit-initialization", "state-field-alias", "state-whole-value-write", "state-concurrency", "state-deferred"} {
		if !stateDiagnostic(result, code) {
			t.Errorf("missing %s diagnostic", code)
		}
	}
	for _, d := range result.Diagnostics {
		if strings.HasPrefix(d.Code, "state-") && (d.File == "" || d.Span.Start.Line == 0) {
			t.Errorf("unlocated limitation: %#v", d)
		}
	}
}

func TestStateCFGBarriersCapsAndParserDamage(t *testing.T) {
	sources := selected(t, map[string]string{"state.go": `package flow
 type Connection struct { state string }
 func Worker(c *Connection, ch chan bool) {
  select { case <-ch: c.state = "hidden" }
  c.state = "disconnected"
 }
 func Damaged(c *Connection) { c.state =
 `})
	result, err := AnalyzeState(context.Background(), sources, "Connection", "state", 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Updates) != 2 || !stateDiagnostic(result, "parse-error") || !stateDiagnostic(result, "state-cfg-unmodeled") {
		t.Fatalf("partial data/barrier evidence: %#v", result)
	}
	if len(result.Updates[0].NodeIDs) != 0 || len(result.Updates[1].NodeIDs) != 1 {
		t.Fatalf("invented/omitted node identity: %#v", result.Updates)
	}
	g := stateGraph(t, result, result.Updates[1].FunctionID)
	if activityPath(g, g.Entry, result.Updates[1].NodeIDs[0]) {
		t.Fatal("unsupported barrier pretended to execute disconnected write")
	}
	limited, err := AnalyzeState(context.Background(), sources, "Connection", "state", 2)
	if !errors.Is(err, activity.ErrLimit) || len(limited.Updates) != 2 || len(limited.Files) != 1 || len(limited.Functions) != 1 || len(limited.Functions[0].Nodes) != 0 {
		t.Fatalf("node cap returned truncated proof: %#v %v", limited, err)
	}
	_, err = AnalyzeState(context.Background(), sources, "Connection", "state", 0)
	if err == nil {
		t.Fatal("zero node cap accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = AnalyzeState(ctx, sources, "Connection", "state", 500)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestStateBooleanLiteralsAndRangeBinding(t *testing.T) {
	result := stateFor(t, `package flow
 type Connection struct { state bool }
 func Set(c *Connection, values []bool) {
  c.state = true
  c.state = false
  for _, c.state = range values {}
 }
 `)
	if len(result.Values) != 2 || len(result.Updates) != 3 {
		t.Fatalf("boolean/range evidence: %#v", result)
	}
	for _, update := range result.Updates {
		if len(update.NodeIDs) != 1 {
			t.Fatalf("missing modeled write action: %#v", update)
		}
		if update.Kind == "range" && update.ValueKey != "" {
			t.Fatalf("range value inferred: %#v", update)
		}
		if update.Kind == "assignment" && update.ValueKey == "" {
			t.Fatalf("boolean literal lost: %#v", update)
		}
	}
}

func TestStateInstantiatedFieldIdentityAndDecrement(t *testing.T) {
	result := stateFor(t, `package flow
 type Connection[T any] struct { state int; payload T }
 type Other[T any] struct { state int }
 func Set(c *Connection[string], other *Other[string]) {
  c.state = 1
  c.state--
  other.state = 2
 }
 `)
	if len(result.Updates) != 2 || result.Updates[0].ValueKey == "" || result.Updates[1].Kind != "decrement" || result.Updates[1].ValueKey != "" {
		t.Fatalf("generic field origin/decrement: %#v", result)
	}
	for _, u := range result.Updates {
		if len(u.NodeIDs) != 1 {
			t.Fatalf("generic update CFG: %#v", u)
		}
	}
}

func TestStateDistinctDefinedOwnersDoNotShareFieldEvidence(t *testing.T) {
	a := stateFor(t, `package app
type Connection struct { state int }
type Other Connection
type Container struct { Connection }
func wrong(o *Other) { o.state = 1 }
func direct(c *Connection) { c.state = 2 }
func promoted(c *Container) { c.state = 3 }
`)
	if len(a.Updates) != 2 {
		t.Fatalf("defined underlying type contaminated selected field: %+v", a.Updates)
	}
	for _, u := range a.Updates {
		if u.Expression == "1" {
			t.Fatal("Other.state became Connection.state evidence")
		}
	}
}

func TestStateUnevaluatedInitializersNeverGainReturnCoverage(t *testing.T) {
	a := stateFor(t, `package app
import "unsafe"
type Connection struct { state int }
func F() bool { return false && (Connection{state: 1}).state == 1 }
func G() uintptr { return unsafe.Sizeof(Connection{state: 2}) }
`)
	if len(a.Updates) != 2 {
		t.Fatalf("initializer source candidates lost: %+v", a.Updates)
	}
	for _, u := range a.Updates {
		g := stateGraph(t, a, u.FunctionID)
		for _, id := range u.NodeIDs {
			if activityPath(g, g.Entry, id) {
				t.Fatalf("unevaluated initializer gained connected enclosing-node evidence: %+v", u)
			}
		}
	}
}

func TestStateObservedConstantSpellingsRetainSemanticAmbiguity(t *testing.T) {
	a := stateFor(t, `package app
type Connection struct { state int }
func A(c *Connection) { const n = 1; c.state = (n) }
func B(c *Connection) { const n = 2; c.state = (n) }
`)
	keys := map[string]bool{}
	for _, value := range a.Values {
		if value.Qualified == "(n)" {
			keys[value.Key] = true
		}
	}
	if len(keys) != 2 {
		t.Fatalf("distinct constant expressions with identical spelling merged: %+v", a.Values)
	}
	d := state.Definition{Version: 1, Initial: "unknown", States: []state.State{{Name: "unknown"}, {Name: "bound", Value: "(n)"}}}
	if _, err := state.Bind(d, a, 500); err == nil {
		t.Fatal("ambiguous source spelling bound to an arbitrary constant")
	}
}

func TestStateRangeTargetLiteralRetainsOwnWrite(t *testing.T) {
	a := stateFor(t, `package app
type Connection struct { state int }
func F(c *Connection, xs []int) {
 for (func() *Connection { c.state = 1; return c })().state = range xs {}
}`)
	var rangeID, assignmentID string
	for _, u := range a.Updates {
		if u.Kind == "range" {
			rangeID = u.FunctionID
		}
		if u.Kind == "assignment" && u.Expression == "1" {
			assignmentID = u.FunctionID
		}
	}
	if rangeID == "" || assignmentID == "" || rangeID == assignmentID {
		t.Fatalf("range-target literal write lost or assigned outer ownership: %+v", a.Updates)
	}
}
