package sequencego

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/sequence"
)

func selected(t *testing.T, files map[string]string) []sequence.Source {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/app\n\ngo 1.25\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var sources []sequence.Source
	for name, content := range files {
		sources = append(sources, sequence.Source{Path: filepath.Join(root, name), Root: root, Content: []byte(content)})
	}
	return sources
}
func analyze(t *testing.T, sources []sequence.Source) sequence.Program {
	t.Helper()
	p, err := Analyze(context.Background(), sources)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func named(t *testing.T, p sequence.Program, name string) sequence.Callable {
	t.Helper()
	var matches []sequence.Callable
	for _, c := range p.Callables {
		if c.Name == name {
			matches = append(matches, c)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("%s: got %d declarations", name, len(matches))
	}
	return matches[0]
}
func clean(t *testing.T, p sequence.Program) {
	t.Helper()
	for _, d := range p.Diagnostics {
		if d.Error {
			t.Errorf("unexpected diagnostic: %s: %s", d.Code, d.Message)
		}
	}
}
func walk(events []sequence.Event, fn func(sequence.Event)) {
	for _, e := range events {
		fn(e)
		walk(e.Children, fn)
		walk(e.Else, fn)
		for _, branch := range e.Branches {
			walk(branch.Events, fn)
		}
	}
}
func calls(events []sequence.Event) []sequence.Event {
	var result []sequence.Event
	walk(events, func(e sequence.Event) {
		if e.Kind == sequence.Call || e.Kind == sequence.Async || e.Kind == sequence.Defer {
			result = append(result, e)
		}
	})
	return result
}

func TestSemanticBindingsAcrossFilesAndSelectedImports(t *testing.T) {
	sources := selected(t, map[string]string{
		"main.go": `package app
import "example.test/app/local"
func Run() { var s Service; s.Handle(local.Value()); Generic[int](1); f := Helper; f(); { Helper := func() {}; Helper() }; Helper() }
`,
		"service.go": `package app
type Service struct{}
func (*Service) Handle(v int) {}
func Generic[T any](v T) T { return v }
func Helper() {}
`,
		"local/local.go": `package local
func Value() int { return 1 }
`,
	})
	p := analyze(t, sources)
	clean(t, p)
	run := named(t, p, "Run")
	events := calls(run.Body)
	if len(events) != 6 {
		t.Fatalf("calls = %#v", events)
	}
	if events[0].Target != named(t, p, "Value").ID || events[1].Target != named(t, p, "Handle").ID || events[2].Target != named(t, p, "Generic").ID {
		t.Fatalf("semantic direct bindings lost: %#v", events[:3])
	}
	if events[3].Dispatch != "function-value" || events[3].Target != "" {
		t.Fatalf("function value bound directly: %#v", events[3])
	}
	if events[4].Dispatch != "function-value" || events[4].Target != "" {
		t.Fatalf("shadowed literal variable bound to named function: %#v", events[4])
	}
	if events[5].Target != named(t, p, "Helper").ID {
		t.Fatalf("outer binding not restored: %#v", events[5])
	}
	if !strings.Contains(named(t, p, "Handle").Selector, ".(*Service).Handle(int)") {
		t.Fatalf("pointer method selector: %s", named(t, p, "Handle").Selector)
	}
	// Input ordering cannot affect identities, candidates, locations or diagnostics.
	for i, j := 0, len(sources)-1; i < j; i, j = i+1, j-1 {
		sources[i], sources[j] = sources[j], sources[i]
	}
	if other := analyze(t, sources); !reflect.DeepEqual(p, other) {
		t.Fatal("analysis depends on source iteration order")
	}
}

func TestReceiverArgumentsAndLiteralBodies(t *testing.T) {
	p := analyze(t, selected(t, map[string]string{"a.go": `package app
type S struct{}
func receiver() *S { return nil }
func argument() int { return 1 }
func (*S) Method(int) {}
func inner() {}
func Run() { receiver().Method(argument()); literal := func() { inner() }; _ = literal; func() { inner() }() }
`}))
	clean(t, p)
	events := calls(named(t, p, "Run").Body)
	if len(events) != 4 {
		t.Fatalf("literal body executed on creation: %#v", events)
	}
	for i, name := range []string{"receiver", "argument", "Method"} {
		if events[i].Target != named(t, p, name).ID {
			t.Fatalf("evaluation order %d: %#v", i, events)
		}
	}
	var target *sequence.Callable
	for i := range p.Callables {
		if p.Callables[i].ID == events[3].Target {
			target = &p.Callables[i]
		}
	}
	if target == nil || target.Name != "func" || len(calls(target.Body)) != 1 {
		t.Fatalf("immediate literal not represented as separate direct body: %#v", events[3])
	}
}

func TestDynamicInterfaceAndFunctionAlternatives(t *testing.T) {
	p := analyze(t, selected(t, map[string]string{"a.go": `package app
type I interface { M(int) }
type A struct{}
type B struct{}
func (*A) M(int) {}
func (B) M(int) {}
func known(int) {}
func Generic[T any](v T) {}
func Run(i I, f func(int)) { i.M(1); f(1); g := Generic[int]; g(1) }
`}))
	clean(t, p)
	events := calls(named(t, p, "Run").Body)
	if len(events) != 3 || events[0].Dispatch != "interface" || events[0].Target != "" || len(events[0].Candidates) != 2 {
		t.Fatalf("interface alternatives = %#v", events)
	}
	for _, e := range events {
		if e.Target != "" {
			t.Fatalf("dynamic dispatch acquired synchronous target: %#v", e)
		}
	}
	if events[1].Dispatch != "function-value" || len(events[1].Candidates) < 1 {
		t.Fatalf("missing function alternatives: %#v", events[1])
	}
	generic := named(t, p, "Generic").ID
	found := false
	for _, id := range events[2].Candidates {
		if id == generic {
			found = true
		}
	}
	if !found {
		t.Fatalf("compiler-instantiated generic candidate missing: %#v", events[2])
	}
}

func TestStructuredBranchesLoopsShortCircuitAndTermination(t *testing.T) {
	p := analyze(t, selected(t, map[string]string{"a.go": `package app
func test() bool { return true }
func next() {}
func start() {}
func Run() { if test() && test() { next() } else { start() }; for start(); test(); next() { if test() { return }; start() }; switch test() { case test(): next(); default: return }; return; next() }
`}))
	clean(t, p)
	body := named(t, p, "Run").Body
	if len(body) != 9 {
		t.Fatalf("unexpected root regions: %#v", body)
	}
	if body[0].Kind != sequence.Call || body[1].Kind != sequence.Opt || len(body[1].Children) != 1 || body[2].Kind != sequence.Alt {
		t.Fatalf("short circuit flattened: %#v", body)
	}
	if len(body[2].Children) != 1 || len(body[2].Else) != 1 {
		t.Fatal("if branches missing")
	}
	if body[3].Target != named(t, p, "start").ID || body[4].Kind != sequence.Loop || body[4].Children[0].Target != named(t, p, "test").ID {
		t.Fatalf("loop condition/init placement: %#v", body)
	}
	if body[6].Kind != sequence.Opt || body[7].Kind != sequence.Alt || len(body[7].Branches) != 2 || !body[7].Exhaustive {
		t.Fatalf("switch alternatives flattened: %#v", body)
	}
	if body[len(body)-1].Kind != sequence.Return {
		t.Fatal("return did not terminate root block")
	}
}

func TestAsyncDeferChannelsSelectAndControlBarriers(t *testing.T) {
	p := analyze(t, selected(t, map[string]string{"a.go": `package app
func value() int { return 1 }
func channel() chan int { return nil }
func work(int) {}
func Run() { go work(value()); defer work(value()); channel() <- value(); x := <-channel(); _ = x; select {case channel() <- value(): work(1); case x := <-channel(): work(x); default: work(0)}; for { break; work(2) }; work(3) }
`}))
	cleanErrors := 0
	for _, d := range p.Diagnostics {
		if d.Error && d.Code != "unsupported" {
			t.Errorf("unexpected diagnostic: %s", d.Message)
		}
		if d.Code == "unsupported" {
			cleanErrors++
		}
	}
	if cleanErrors != 1 {
		t.Fatalf("break barrier diagnostics = %d", cleanErrors)
	}
	body := named(t, p, "Run").Body
	if body[0].Kind != sequence.Call || body[1].Kind != sequence.Async || body[2].Kind != sequence.Call || body[3].Kind != sequence.Defer {
		t.Fatalf("launch/registration evaluation order: %#v", body)
	}
	for _, i := range []int{1, 3} {
		if len(body[i].Children) != 0 {
			t.Fatal("async/defer contains synchronous body")
		}
	}
	var selectEvent *sequence.Event
	for i := range body {
		if body[i].Kind == sequence.Alt {
			selectEvent = &body[i]
		}
	}
	if selectEvent == nil || len(selectEvent.Branches) != 3 {
		t.Fatalf("select branches missing: %#v", body)
	}
	if selectEvent.Branches[0].Events[0].Kind != sequence.Send || selectEvent.Branches[1].Events[0].Kind != sequence.Receive {
		t.Fatalf("select operations not conditional: %#v", selectEvent)
	}
	var loop sequence.Event
	for _, e := range body {
		if e.Kind == sequence.Loop {
			loop = e
		}
	}
	if len(loop.Children) != 1 || loop.Children[0].Kind != sequence.Unsupported {
		t.Fatalf("calls past control barrier leaked: %#v", loop)
	}
}

func TestSnapshotsLocationsInitAndErrors(t *testing.T) {
	sources := selected(t, map[string]string{"a.go": `package app
import "missing.test/dependency"
import "unsafe"
func init() {}
func init() {}
func local() {}
func Run() { /* é */ local(); missing.Nope(); _ = unsafe.Sizeof(0) }
`})
	p := analyze(t, sources)
	run := named(t, p, "Run")
	var localCall sequence.Event
	for _, e := range calls(run.Body) {
		if e.Target == named(t, p, "local").ID {
			localCall = e
		}
	}
	if localCall.Target == "" {
		t.Fatal("external import error erased valid local binding")
	}
	captured := sources[0].Content[localCall.Span.Start.Offset:localCall.Span.End.Offset]
	if string(captured) != "local()" || localCall.Span.Start.Line != 7 {
		t.Fatalf("snapshot position wrong: %q %#v", captured, localCall.Span)
	}
	lineStart := strings.LastIndex(string(sources[0].Content[:localCall.Span.Start.Offset]), "\n") + 1
	if localCall.Span.Start.Column != localCall.Span.Start.Offset-lineStart+1 {
		t.Fatal("column is not a byte column")
	}
	hash := sha256.Sum256(sources[0].Content)
	if p.Files[0].Hash != fmt.Sprintf("%x", hash) {
		t.Fatal("snapshot hash mismatch")
	}
	count, ids := 0, map[string]bool{}
	for _, c := range p.Callables {
		if c.Name == "init" {
			count++
			ids[c.ID] = true
		}
	}
	if count != 2 || len(ids) != 2 {
		t.Fatal("init declarations collapsed")
	}
	external, unresolved := false, false
	for _, d := range p.Diagnostics {
		external = external || strings.Contains(d.Message, "not available in selected sources")
		unresolved = unresolved || d.Code == "unresolved-call"
	}
	if !external || !unresolved {
		t.Fatalf("missing honest dependency diagnostics: %#v", p.Diagnostics)
	}
	if other := analyze(t, sources); !reflect.DeepEqual(p, other) {
		t.Fatal("error diagnostics are nondeterministic")
	}
}

func TestParseRecoveryCancellationAndImportCycles(t *testing.T) {
	t.Run("parse", func(t *testing.T) {
		p := analyze(t, selected(t, map[string]string{"a.go": "package app\nfunc Good() {}\nfunc Broken( {\n"}))
		found := false
		for _, d := range p.Diagnostics {
			found = found || d.Code == "parse-error"
		}
		if !found {
			t.Fatal("missing parser diagnostic")
		}
		named(t, p, "Good")
	})
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := Analyze(ctx, selected(t, map[string]string{"a.go": "package app"})); err != context.Canceled {
			t.Fatalf("cancellation = %v", err)
		}
	})
	t.Run("cycle", func(t *testing.T) {
		p := analyze(t, selected(t, map[string]string{"a/a.go": "package a\nimport \"example.test/app/b\"\nfunc A() { b.B() }", "b/b.go": "package b\nimport \"example.test/app/a\"\nfunc B() { a.A() }"}))
		found := false
		for _, d := range p.Diagnostics {
			found = found || strings.Contains(d.Message, "import cycle")
		}
		if !found {
			t.Fatalf("missing cycle diagnostic: %#v", p.Diagnostics)
		}
	})
}

func TestUnsafeConstantsDoNotEvaluateOperandsAndBodylessIsNotEntry(t *testing.T) {
	p := analyze(t, selected(t, map[string]string{"a.go": `package app
import "unsafe"
func operand() int { return 1 }
func assembly()
func Run() { _ = unsafe.Sizeof(operand()); assembly() }
`}))
	clean(t, p)
	for _, c := range p.Callables {
		if c.Name == "assembly" {
			t.Fatal("bodyless declaration exposed as entry")
		}
	}
	events := calls(named(t, p, "Run").Body)
	if len(events) != 1 || events[0].Dispatch != "external" || events[0].Target != "" {
		t.Fatalf("constant operand evaluated or assembly body fabricated: %#v", events)
	}
}

func TestMethodExpressionsAndGenericReceiversUseDeclarationIDs(t *testing.T) {
	p := analyze(t, selected(t, map[string]string{"a.go": `package app
type Box[T any] struct{}
func (*Box[T]) Put(T) {}
func Run() { b := &Box[int]{}; b.Put(1); f := (*Box[int]).Put; f(b, 2) }
`}))
	clean(t, p)
	method := named(t, p, "Put")
	events := calls(named(t, p, "Run").Body)
	if len(events) != 2 || events[0].Target != method.ID || events[1].Dispatch != "function-value" || events[1].Target != "" {
		t.Fatalf("generic method binding = %#v", events)
	}
	found := false
	for _, id := range events[1].Candidates {
		found = found || id == method.ID
	}
	if !found {
		t.Fatalf("method expression alternative omitted: %#v", events[1])
	}
}

func TestParserDamagedRegionsDoNotExposeNestedCallables(t *testing.T) {
	for _, damaged := range []string{
		"func Broken() { saved := func() { Local() }; _ = saved; Local( }\n",
		"var Broken = func() { saved := func() { Local() }; _ = saved; Local( }\n",
		"var Broken = func() { saved := func() { Local() }; _ = saved\n",
		"var Broken = (func() { saved := func() { Local() }; _ = saved\n",
	} {
		p := analyze(t, selected(t, map[string]string{"flow.go": "package app\nfunc Local() {}\nfunc Good() { Local() }\n" + damaged}))
		for _, callable := range p.Callables {
			if callable.Name != "Good" && callable.Name != "Local" {
				t.Fatalf("parser-damaged source leaked a callable: %s", callable.Selector)
			}
		}
		good := named(t, p, "Good")
		events := calls(good.Body)
		if len(events) != 1 || events[0].Target != named(t, p, "Local").ID {
			t.Fatalf("parser damage erased a valid sibling binding: %#v", events)
		}
	}
}
