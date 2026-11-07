package sequencejava

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func stateSource(t *testing.T, text string) []sequence.Source {
	t.Helper()
	root := t.TempDir()
	return []sequence.Source{{Root: root, Path: filepath.Join(root, "Sample.java"), Content: []byte(text)}}
}

func analyzeState(t *testing.T, sources []sequence.Source, selector, field string) state.Analysis {
	t.Helper()
	requireJDK(t)
	a, err := AnalyzeState(context.Background(), sources, selector, field, 500, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestStateConnectionEnumCandidatesAndExactCFGIdentity(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "state", "java", "Connection.java"))
	if err != nil {
		t.Fatal(err)
	}
	sources := stateSource(t, string(data))
	a := analyzeState(t, sources, "Connection", "state")
	if a.Target.Qualified != "fixture.Connection" || a.Target.ID != sources[0].Root+"::fixture.Connection" || a.Target.FieldType != "fixture.Connection.Phase" || a.Target.Span.Start.Line != 5 {
		t.Fatalf("target metadata: %+v", a.Target)
	}
	hash := sha256.Sum256(data)
	if len(a.Files) != 1 || a.Files[0].Hash != hex.EncodeToString(hash[:]) {
		t.Fatal("snapshot provenance lost")
	}
	if len(a.Values) != 4 {
		t.Fatalf("unused enum members lost or foreign values leaked: %+v", a.Values)
	}
	keys := map[string]bool{}
	for _, value := range a.Values {
		if value.Key != value.ID || value.ID != sources[0].Root+"::fixture.Connection.Phase."+value.Name || value.Qualified != "fixture.Connection.Phase."+value.Name || value.File != sources[0].Path {
			t.Fatalf("enum identity: %+v", value)
		}
		keys[value.Key] = true
	}
	graphs := map[string]activity.Graph{}
	for _, graph := range a.Functions {
		if err := graph.Validate(500); err != nil {
			t.Fatal(err)
		}
		graphs[graph.Function.ID] = graph
	}
	var initializer, unknown, guarded, caseGuard, alias, lambda, constructor, barrier, disconnected bool
	for _, update := range a.Updates {
		if update.ValueKey != "" && !keys[update.ValueKey] {
			t.Fatalf("unknown constant key: %+v", update)
		}
		if update.Kind == "initializer" {
			initializer = update.FunctionID == "" && len(update.NodeIDs) == 0
			continue
		}
		graph, ok := graphs[update.FunctionID]
		if !ok {
			t.Fatalf("missing owning graph: %+v", update)
		}
		if graph.Function.Name == "guarded" || (graph.Function.Name == "afterBarrier" && update.Expression == "Phase.OPEN") {
			if len(update.NodeIDs) != 0 {
				t.Fatalf("barrier fabricated assignment coverage: %+v", update)
			}
			barrier = true
		} else {
			if len(update.NodeIDs) != 1 {
				t.Fatalf("missing exact assignment node: %+v", update)
			}
			for _, id := range update.NodeIDs {
				node := graph.Nodes[id-1]
				if node.Kind != "action" || node.Span != update.Span {
					t.Fatalf("correlation used enclosing non-write: %+v %+v", update, node)
				}
				if graph.Function.Name == "afterBarrier" {
					disconnected = !activityReachable(graph, graph.Entry, id)
				}
			}
		}
		unknown = unknown || (update.Expression == "unknown()" && update.ValueKey == "")
		guarded = guarded || len(update.Guards) > 0
		for _, guard := range update.Guards {
			caseGuard = caseGuard || guard == "case 1"
		}
		alias = alias || update.Receiver == "other"
		lambda = lambda || graph.Function.Name == "<lambda>"
		constructor = constructor || graph.Function.Name == "<init>"
	}
	if !initializer || !unknown || !guarded || !caseGuard || !alias || !lambda || !constructor || !barrier || !disconnected {
		t.Fatalf("missing evidence: initializer=%v unknown=%v guards=%v case=%v alias=%v lambda=%v constructor=%v barrier=%v disconnected=%v", initializer, unknown, guarded, caseGuard, alias, lambda, constructor, barrier, disconnected)
	}
	codes := map[string]bool{}
	for _, diagnostic := range a.Diagnostics {
		codes[diagnostic.Code] = true
	}
	for _, code := range []string{"state.candidates", "state.alias-parallel", "state.initializer", "state.unknown-value", "state.receiver-alias", "state.cfg-unmodeled"} {
		if !codes[code] {
			t.Fatalf("missing %s caveat", code)
		}
	}
	for _, selector := range []string{a.Target.ID, a.Target.Qualified} {
		again := analyzeState(t, sources, selector, "state")
		if !reflect.DeepEqual(a, again) {
			t.Fatalf("non-deterministic selection: %s", selector)
		}
	}
	limited, err := AnalyzeState(context.Background(), sources, "Connection", "state", 1, Options{})
	if !errors.Is(err, activity.ErrLimit) || limited.Target.ID == "" || len(limited.Files) != 1 || len(limited.Functions) != 0 {
		t.Fatalf("cap did not fail closed: %+v %v", limited, err)
	}
}

func TestStateDirectFieldShadowingAndScalarConstants(t *testing.T) {
	sources := stateSource(t, `class Base { int state; }
class Sample extends Base {
  static final int ZERO=0, ONE=1, ALSO_ONE=0x1, UNUSED=7;
  int state=ZERO;
  void entry(int state) { state=ONE; this.state=ONE; super.state=ONE; }
  void other(Sample target) { target.state=0x1; target.state=1; target.state=unknown(); }
  void mutate() { state+=ONE; state++; state=ONE+1; state=-1; }
  int unknown() { return 3; }
  class Nested { int state; void write() { state=ONE; Sample.this.state=UNUSED; } }
}`)
	a := analyzeState(t, sources, "Sample", "state")
	byName := map[string]state.Value{}
	for _, value := range a.Values {
		byName[value.Name] = value
	}
	if byName["ONE"].Key == "" || byName["ONE"].Key != byName["ALSO_ONE"].Key || byName["ONE"].Key != byName["0x1"].Key || byName["ONE"].Key != byName["1"].Key || byName["UNUSED"].ID == "" {
		t.Fatalf("scalar binding: %+v", a.Values)
	}
	if len(a.Updates) != 10 {
		t.Fatalf("shadowing or direct-field write loss: %+v", a.Updates)
	}
	var compound, increment, unknownExpression, negative bool
	for _, update := range a.Updates {
		compound = compound || update.Kind == "compound" && update.ValueKey == ""
		increment = increment || update.Kind == "increment" && update.ValueKey == ""
		unknownExpression = unknownExpression || update.Expression == "ONE+1" && update.ValueKey == ""
		negative = negative || update.Expression == "-1" && update.ValueKey == "scalar:integer:-1"
		if update.Receiver == "super" {
			t.Fatal("inherited shadow field merged")
		}
	}
	if !compound || !increment || !unknownExpression || !negative {
		t.Fatalf("unproved writes suppressed: %+v", a.Updates)
	}
}

func TestStateSelectionFailuresPreserveMetadataAndPartialSources(t *testing.T) {
	requireJDK(t)
	sources := stateSource(t, `class Sample { int state; void valid() { state=1; Missing.call(); } void broken() { state=; } } class Other { Object state; }`)
	a := analyzeState(t, sources, "Sample", "state")
	if len(a.Updates) != 1 || len(a.Diagnostics) == 0 {
		t.Fatalf("partial compiler evidence lost: %+v", a)
	}
	for _, selection := range [][2]string{{"Missing", "state"}, {"Sample", "missing"}, {"Other", "state"}} {
		failed, err := AnalyzeState(context.Background(), sources, selection[0], selection[1], 500, Options{})
		if !errors.Is(err, state.ErrSelection) || len(failed.Files) != 1 || len(failed.Diagnostics) == 0 {
			t.Fatalf("selection failure metadata: %+v %v", failed, err)
		}
	}
	root := t.TempDir()
	for _, item := range []struct{ pkg, sub string }{{"one", "one"}, {"two", "two"}} {
		sources = append(sources, sequence.Source{Root: root, Path: filepath.Join(root, item.sub, "Twin.java"), Content: []byte("package " + item.pkg + "; class Twin { int state; void write() { state=1; } }")})
	}
	failed, err := AnalyzeState(context.Background(), sources, "Twin", "state", 500, Options{})
	if !errors.Is(err, state.ErrSelection) || !errors.Is(err, state.ErrAmbiguous) || len(failed.Files) != 3 {
		t.Fatalf("ambiguous target: %+v %v", failed, err)
	}
	selected := analyzeState(t, sources, root+"::two.Twin", "state")
	if selected.Target.Qualified != "two.Twin" || len(selected.Updates) != 1 || !strings.Contains(selected.Updates[0].FunctionID, "two.Twin") {
		t.Fatalf("same-name types merged: %+v", selected)
	}
	invalid := stateSource(t, "class Sample {}")
	invalid[0].Content = []byte{0xff}
	if _, err := AnalyzeState(context.Background(), invalid, "Sample", "state", 50, Options{}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

func TestStateEnumsAndSameNamedConstantsNeverMerge(t *testing.T) {
	a := analyzeState(t, stateSource(t, `class Sample {
 enum Phase { OPEN, CLOSED, UNUSED }
 enum Other { OPEN, CLOSED }
 Phase state;
 void entry() { state=Phase.OPEN; }
}`), "Sample", "state")
	if len(a.Values) != 3 || len(a.Updates) != 1 {
		t.Fatalf("same-name enum constants merged: %+v", a)
	}
	for _, value := range a.Values {
		if strings.Contains(value.Qualified, "Other") {
			t.Fatal("foreign enum accepted")
		}
	}
}

func TestStateCompilerBoundPlatformScalarAndLiteralShareExactValue(t *testing.T) {
	a := analyzeState(t, stateSource(t, `class Sample { int state; void entry() { state=Integer.MAX_VALUE; state=2147483647; } }`), "Sample", "state")
	if len(a.Values) != 2 || len(a.Updates) != 2 || a.Values[0].Key == "" || a.Values[0].Key != a.Values[1].Key || a.Updates[0].ValueKey != a.Updates[1].ValueKey {
		t.Fatalf("compiler-bound exact scalar value identity lost: %+v", a)
	}
	if a.Values[0].ID != "jdk::java.lang.Integer.MAX_VALUE" || a.Values[0].File != a.Target.File {
		t.Fatalf("platform value must locate its captured-source use: %+v", a.Values[0])
	}
}

func TestStateParenthesizedReceiversKeepOtherInstanceIdentity(t *testing.T) {
	a := analyzeState(t, stateSource(t, `class Sample {
int state;
void write(Sample other) { ((other.state)) = 1; (other.state)++; }
}`), "Sample", "state")
	if len(a.Updates) != 2 {
		t.Fatalf("parenthesized writes lost: %+v", a.Updates)
	}
	for _, update := range a.Updates {
		if update.Receiver != "other" {
			t.Fatalf("explicit other-instance write became %q", update.Receiver)
		}
	}
}
