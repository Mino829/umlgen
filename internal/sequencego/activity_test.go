package sequencego

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/activity"
	"github.com/Mino829/umlgen/internal/sequence"
)

func activityFor(t *testing.T, body string) activity.Graph {
	t.Helper()
	sources := selected(t, map[string]string{"flow.go": "package flow\n" + body})
	g, err := AnalyzeActivity(context.Background(), sources, "Run", 500)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Validate(500); err != nil {
		t.Fatal(err)
	}
	if g.Function.Body != nil || g.Function.ID == "" || len(g.Files) != 1 {
		t.Fatalf("missing metadata-only declaration: %#v", g)
	}
	for _, n := range g.Nodes {
		if n.File != sources[0].Path || n.Span.Start.Line == 0 || n.Span.End.Offset < n.Span.Start.Offset {
			t.Errorf("node has no source location: %#v", n)
		}
	}
	return g
}

func activityNode(t *testing.T, g activity.Graph, kind, label string) int {
	t.Helper()
	var ids []int
	for _, n := range g.Nodes {
		if n.Kind == kind && n.Label == label {
			ids = append(ids, n.ID)
		}
	}
	if len(ids) != 1 {
		t.Fatalf("%s %q: expected one node, got %v; nodes: %#v", kind, label, ids, g.Nodes)
	}
	return ids[0]
}

func activityPath(g activity.Graph, from, to int, avoid ...int) bool {
	seen := map[int]bool{}
	for _, id := range avoid {
		seen[id] = true
	}
	queue := []int{from}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		if id == to {
			return true
		}
		seen[id] = true
		for _, e := range g.Edges {
			if e.From == id {
				queue = append(queue, e.To)
			}
		}
	}
	return false
}

func activityEdge(t *testing.T, g activity.Graph, from, to int, kind string) {
	t.Helper()
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.Kind == kind {
			return
		}
	}
	t.Errorf("missing %s edge %d -> %d; edges: %#v", kind, from, to, g.Edges)
}

func TestActivityNestedLoopSwitchTransfers(t *testing.T) {
	g := activityFor(t, `func Run(n int) int {
outer:
 for i := 0; i < n; i++ {
  switch i {
  case 0: continue outer
  case 1:
   for j := 0; j < n; j++ { if j > 2 { break }; n-- }
   break
  case 2: break outer
  default: n += i
  }
  if n < 0 { return n }
 }
 return n + 1
}`)
	update := activityNode(t, g, "action", "i++")
	cont := activityNode(t, g, "action", "continue outer")
	outerBreak := activityNode(t, g, "action", "break outer")
	lastReturn := activityNode(t, g, "return", "return n + 1")
	if !activityPath(g, cont, update) || activityPath(g, cont, lastReturn, update) {
		t.Error("labeled continue must execute the outer update before any exit")
	}
	if !activityPath(g, outerBreak, lastReturn) || activityPath(g, outerBreak, update) {
		t.Error("labeled break must leave the outer loop")
	}
	targets := map[string]int{}
	for _, n := range g.Nodes {
		if n.Kind != "action" || n.Label != "break" {
			continue
		}
		for _, e := range g.Edges {
			if e.From == n.ID && e.Kind == "break" {
				for _, target := range g.Nodes {
					if target.ID == e.To {
						targets[target.Label]++
					}
				}
			}
		}
	}
	if targets["after for"] != 1 || targets["after switch"] != 1 {
		t.Errorf("nested break targets were not scope-resolved: %v", targets)
	}
}

func TestActivityEmptyAndInfiniteLoops(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		exits        bool
	}{
		{"infinite", `func Run() { for {} }`, false},
		{"infiniteContinue", `func Run() { for { continue } }`, false},
		{"conditionalEmpty", `func Run(ok bool) { for ok {} }`, true},
		{"break", `func Run() { for { break } }`, true},
		{"rangeZero", `func Run(xs []int) { for range xs {} }`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := activityFor(t, tc.source)
			if got := activityPath(g, g.Entry, g.Exit); got != tc.exits {
				t.Fatalf("entry-to-exit path = %v, want %v", got, tc.exits)
			}
			if tc.name != "break" {
				found := false
				for _, e := range g.Edges {
					found = found || e.Kind == "back" || e.Kind == "continue"
				}
				if !found {
					t.Error("missing loop cycle")
				}
			}
		})
	}
}

func TestActivityShortCircuitSkipsRHS(t *testing.T) {
	g := activityFor(t, `func rhs() bool { return true }
func Run(a, b bool) int {
 if a && (b || rhs()) { return 1 }
 return 0
}`)
	a := activityNode(t, g, "decision", "a")
	b := activityNode(t, g, "decision", "b")
	rhs := activityNode(t, g, "action", "rhs()")
	yes := activityNode(t, g, "return", "return 1")
	no := activityNode(t, g, "return", "return 0")
	activityEdge(t, g, a, no, "false")
	activityEdge(t, g, a, b, "true")
	activityEdge(t, g, b, rhs, "false")
	activityEdge(t, g, b, yes, "true")
	if activityPath(g, g.Entry, rhs, a) || activityPath(g, a, rhs, b) {
		t.Error("RHS is reachable without evaluating required left operands")
	}
	g = activityFor(t, `func rhs() bool { return true }; func Run(a bool) { x := a && rhs(); _ = x }`)
	if !activityPath(g, g.Entry, g.Exit, activityNode(t, g, "action", "rhs()")) {
		t.Error("assignment short circuit cannot bypass RHS")
	}
}

func TestActivitySwitchFallthroughAndCaseEvaluation(t *testing.T) {
	g := activityFor(t, `func first() int { return 0 }; func second() int { return 1 }; func hit() {}; func done() {}
func Run(n int) { switch n { default: done(); case first(): hit(); fallthrough; case second(): done() } }`)
	fall := activityNode(t, g, "action", "fallthrough")
	secondBody := activityNode(t, g, "merge", "case second()")
	secondEval := activityNode(t, g, "action", "second()")
	activityEdge(t, g, fall, secondBody, "fallthrough")
	if activityPath(g, fall, secondEval) {
		t.Error("fallthrough incorrectly evaluates next case expression")
	}
	firstEval := activityNode(t, g, "action", "first()")
	defaultBody := activityNode(t, g, "merge", "default")
	if !activityPath(g, firstEval, defaultBody) || activityPath(g, g.Entry, defaultBody, secondEval) {
		t.Error("default must wait until all explicit case tests fail")
	}
}

func TestActivityLabelsGotoAndNestedScopes(t *testing.T) {
	g := activityFor(t, `func Run(n int) {
 goto work
unused: n += 10
work:
 for n > 0 {
  switch n { case 2: n--; continue; default: break }
  if n == 3 { goto end }
  n--
 }
end: return
}`)
	jump := activityNode(t, g, "action", "goto work")
	work := activityNode(t, g, "merge", "label work")
	activityEdge(t, g, jump, work, "goto")
	if activityPath(g, g.Entry, activityNode(t, g, "action", "n += 10")) {
		t.Error("forward goto did not skip the preceding label")
	}
	cont := activityNode(t, g, "action", "continue")
	head := activityNode(t, g, "merge", "for condition")
	activityEdge(t, g, cont, head, "continue")
	activityEdge(t, g, activityNode(t, g, "action", "goto end"), activityNode(t, g, "merge", "label end"), "goto")
	g = activityFor(t, `func Run(n int) { again: n--; if n > 0 { goto again } }`)
	activityEdge(t, g, activityNode(t, g, "action", "goto again"), activityNode(t, g, "merge", "label again"), "goto")
}

func TestActivityReturnAndBuiltinPanicSinks(t *testing.T) {
	g := activityFor(t, `func after() {}; func Run(ok bool) { if ok { panic("stop") }; after() }`)
	panicID := activityNode(t, g, "throw", `panic("stop")`)
	after := activityNode(t, g, "action", "after()")
	activityEdge(t, g, panicID, g.Exit, "panic")
	if activityPath(g, panicID, after) {
		t.Error("panic path continues normally")
	}
	g = activityFor(t, `func after() {}; func Run() { panic := func(string) {}; panic("ordinary"); after() }`)
	call := activityNode(t, g, "action", `panic("ordinary")`)
	if !activityPath(g, call, activityNode(t, g, "action", "after()")) {
		t.Error("shadowed panic treated as builtin sink")
	}
	for _, n := range g.Nodes {
		if n.Kind == "throw" {
			t.Error("shadowed panic emitted throw")
		}
	}
	g = activityFor(t, `func side() int { panic("callee") }; func Run() int { return side() }`)
	activityEdge(t, g, activityNode(t, g, "return", "return side()"), g.Exit, "return")
	for _, n := range g.Nodes {
		if n.Kind == "throw" {
			t.Error("callee body expanded")
		}
	}
}

func TestActivityFunctionLiteralsAreNotExecuted(t *testing.T) {
	g := activityFor(t, `func Run() { f := func() { panic("unexecuted"); hidden: goto hidden }; _ = f }`)
	for _, n := range g.Nodes {
		if n.Kind == "throw" || n.Kind == "unsupported" || n.Label == "label hidden" {
			t.Errorf("literal body was executed: %#v", n)
		}
	}
	if !activityPath(g, g.Entry, g.Exit) {
		t.Error("literal declaration terminated execution")
	}
}

func TestActivityUnsupportedTransfersAreTerminal(t *testing.T) {
	for _, source := range []string{
		`func cleanup() {}; func Run() { defer cleanup(); cleanup() }`,
		`func Run() { recover(); println("after") }`,
		`func Run() { go println("async"); println("after") }`,
		`func Run() { select {}; println("after") }`,
		`func Run(c chan int) { <-c; println("after") }`,
		`func Run(c chan int) { c <- 1; println("after") }`,
		`func Run(c chan int) { for range c {}; println("after") }`,
		`func iter(yield func(int) bool) {}; func Run() { for range iter {}; println("after") }`,
	} {
		g := activityFor(t, source)
		found := false
		for _, n := range g.Nodes {
			if n.Kind != "unsupported" {
				continue
			}
			found = true
			if !activityPath(g, g.Entry, n.ID) {
				t.Errorf("barrier unreachable: %s", source)
			}
			for _, e := range g.Edges {
				if e.From == n.ID {
					t.Errorf("barrier has outgoing edge: %#v", e)
				}
			}
		}
		if !found || activityPath(g, g.Entry, g.Exit) {
			t.Errorf("unsupported transfer flattened: %s", source)
		}
		located := false
		for _, d := range g.Diagnostics {
			located = located || d.Code == "activity-unsupported" && d.File != "" && d.Span.Start.Line > 0
		}
		if !located {
			t.Error("missing located unsupported diagnostic")
		}
	}
}

func TestActivitySelectionSnapshotsAndDamage(t *testing.T) {
	sources := selected(t, map[string]string{
		"good.go": "package flow\nfunc Run() {}\n",
		"bad.go":  "package flow\nfunc Broken() { if }\n",
	})
	g, err := AnalyzeActivity(context.Background(), sources, "Run", 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Files) != 2 || g.Function.Body != nil {
		t.Fatalf("metadata lost: %#v", g)
	}
	for _, source := range sources {
		want := fmt.Sprintf("%x", sha256.Sum256(source.Content))
		found := false
		for _, f := range g.Files {
			found = found || f.Path == source.Path && f.Hash == want
		}
		if !found {
			t.Errorf("snapshot hash missing for %s", source.Path)
		}
	}
	g, err = AnalyzeActivity(context.Background(), sources, "Broken", 500)
	if !errors.Is(err, activity.ErrSelection) || len(g.Files) != 2 || len(g.Diagnostics) == 0 {
		t.Fatalf("damage selection: %#v, %v", g, err)
	}
	sources = selected(t, map[string]string{"one/a.go": "package p; func Run() {}", "two/b.go": "package p; func Run() {}"})
	g, err = AnalyzeActivity(context.Background(), sources, "Run", 500)
	if !errors.Is(err, activity.ErrSelection) || !errors.Is(err, sequence.ErrAmbiguousEntry) || len(g.Files) != 2 {
		t.Fatalf("ambiguity sentinel/metadata: %#v %v", g, err)
	}
	p := analyze(t, sources)
	for _, c := range p.Callables {
		g, err = AnalyzeActivity(context.Background(), sources, c.ID, 500)
		if err != nil || g.Function.ID != c.ID {
			t.Fatalf("canonical selection: %v", err)
		}
	}
}

func TestActivityCapIsHardAndIDsAreDeterministic(t *testing.T) {
	sources := selected(t, map[string]string{"flow.go": "package flow; func Run(n int) { for n > 0 { n-- }; return }"})
	g, err := AnalyzeActivity(context.Background(), sources, "Run", 500)
	if err != nil {
		t.Fatal(err)
	}
	again, err := AnalyzeActivity(context.Background(), sources, g.Function.Selector, 500)
	if err != nil || !reflect.DeepEqual(g, again) {
		t.Fatalf("nondeterministic activity graph: %v", err)
	}
	exact, err := AnalyzeActivity(context.Background(), sources, "Run", len(g.Nodes))
	if err != nil || len(exact.Nodes) != len(g.Nodes) {
		t.Fatalf("exact node cap: %v", err)
	}
	limited, err := AnalyzeActivity(context.Background(), sources, "Run", len(g.Nodes)-1)
	if !errors.Is(err, activity.ErrLimit) || len(limited.Nodes) != 0 || len(limited.Edges) != 0 || limited.Function.ID == "" {
		t.Fatalf("truncated output or lost limit metadata: %#v %v", limited, err)
	}
	_, err = AnalyzeActivity(context.Background(), sources, "Run", 1)
	if !errors.Is(err, activity.ErrLimit) {
		t.Fatalf("synthetic nodes not counted: %v", err)
	}
}

func TestActivitySelectedSourcesOnlyAndFixture(t *testing.T) {
	sources := selected(t, map[string]string{"flow.go": `package flow; import "unselected.invalid/no-download"; func Run() {}`})
	g, err := AnalyzeActivity(context.Background(), sources, "Run", 500)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range g.Diagnostics {
		found = found || strings.Contains(d.Message, "external source loading disabled")
	}
	if !found {
		t.Error("selected-source importer boundary was lost")
	}
	path := filepath.Join("..", "..", "testdata", "activity", "go", "orders.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	g, err = AnalyzeActivity(context.Background(), []sequence.Source{{Path: path, Content: content}}, "ProcessOrder", 500)
	if err != nil {
		t.Fatal(err)
	}
	if !activityPath(g, g.Entry, g.Exit) || g.Function.Name != "ProcessOrder" {
		t.Fatalf("fixture graph: %#v", g)
	}
}

func TestActivityConstantConditionsAndLoopUpdate(t *testing.T) {
	g := activityFor(t, `func rhs() bool { return true }; func Run() { if false && rhs() { return }; for true {} }`)
	if activityPath(g, g.Entry, activityNode(t, g, "action", "rhs()")) {
		t.Error("constant short circuit evaluated the RHS")
	}
	if activityPath(g, g.Entry, g.Exit) {
		t.Error("constant infinite loop acquired a false exit")
	}
	g = activityFor(t, `func Run(n int) { for i := 1; i < n; i += 2 { if i == 3 { continue }; n-- } }`)
	init := activityNode(t, g, "action", "i := 1")
	condition := activityNode(t, g, "decision", "i < n")
	update := activityNode(t, g, "action", "i += 2")
	cont := activityNode(t, g, "action", "continue")
	if !activityPath(g, g.Entry, init) || activityPath(g, g.Entry, condition, init) {
		t.Error("for initializer must execute before the condition")
	}
	if !activityPath(g, cont, update) || activityPath(g, cont, condition, update) {
		t.Error("continue skipped the update")
	}
	activityEdge(t, g, update, activityNode(t, g, "merge", "for condition"), "back")
}

func TestActivityRangeBindingAndZeroIteration(t *testing.T) {
	g := activityFor(t, `func xs() []int { return nil }; func hit(int) {}; func Run() { for _, item := range xs() { hit(item) } }`)
	evaluate := activityNode(t, g, "action", "xs()")
	head := activityNode(t, g, "decision", "range xs()")
	bind := activityNode(t, g, "action", "range bind _, item")
	body := activityNode(t, g, "action", "hit(item)")
	if activityPath(g, head, evaluate) {
		t.Error("range expression is evaluated again on the back edge")
	}
	if !activityPath(g, g.Entry, g.Exit, bind, body) {
		t.Error("range has no zero-iteration path")
	}
	if activityPath(g, head, body, bind) {
		t.Error("range body skipped its iteration variable binding")
	}
	activityEdge(t, g, body, head, "back")
}

func TestActivityInvalidGotoIsLocatedBarrier(t *testing.T) {
	g := activityFor(t, `func Run() { goto target; x := 1; target: println(x) }`)
	found := false
	for _, n := range g.Nodes {
		if n.Kind == "unsupported" && strings.Contains(n.Label, "invalid branch transfer") {
			found = true
			if !activityPath(g, g.Entry, n.ID) {
				t.Error("invalid goto barrier is unreachable")
			}
		}
	}
	if !found || activityPath(g, g.Entry, g.Exit) {
		t.Error("invalid scope-crossing goto was projected as a legal transfer")
	}
}

func TestActivityConditionalReturnAndArguments(t *testing.T) {
	for _, source := range []string{
		`func rhs() bool { return true }; func Run(a bool) bool { return a || rhs() }`,
		`func rhs() bool { return true }; func consume(bool) {}; func Run(a bool) { consume(a || rhs()) }`,
	} {
		g := activityFor(t, source)
		a := activityNode(t, g, "decision", "a")
		rhs := activityNode(t, g, "action", "rhs()")
		activityEdge(t, g, a, rhs, "false")
		if !activityPath(g, g.Entry, g.Exit, rhs) || activityPath(g, g.Entry, rhs, a) {
			t.Error("conditional return/argument did not preserve the RHS bypass")
		}
	}
}

func TestActivityTypeSwitchAndLabeledSwitchBreak(t *testing.T) {
	g := activityFor(t, `func input() any { return nil }; func hit(int) {}; func Run() {
choice:
 switch v := input().(type) {
 case int: hit(v); break choice
 case string, nil: return
 default: return
 }
}`)
	call := activityNode(t, g, "action", "input()")
	first := activityNode(t, g, "decision", "type(input()) is int")
	if !activityPath(g, g.Entry, first) || activityPath(g, g.Entry, first, call) || activityPath(g, first, call) {
		t.Error("type-switch guard must evaluate exactly once before its case tests")
	}
	activityEdge(t, g, activityNode(t, g, "action", "break choice"), activityNode(t, g, "merge", "after switch"), "break")
	for _, n := range g.Nodes {
		if n.Kind == "unsupported" {
			t.Errorf("type switch was not modeled: %#v", n)
		}
	}
}

func TestActivityUnsafeConstantOperandsAreNotEvaluated(t *testing.T) {
	for _, operation := range []string{"unsafe.Sizeof(<-c)", "unsafe.Alignof(<-c)", "unsafe.Offsetof((<-c).Field)"} {
		t.Run(operation, func(t *testing.T) {
			g := activityFor(t, `import "unsafe"
type Row struct { Field int }
func Run(c chan Row) { _ = `+operation+`; println("after") }`)
			after := activityNode(t, g, "action", `println("after")`)
			if !activityPath(g, g.Entry, after) || !activityPath(g, after, g.Exit) {
				t.Fatal("constant unsafe layout operation evaluated its operand and lost the normal continuation")
			}
			for _, n := range g.Nodes {
				if n.Kind == "unsupported" {
					t.Fatalf("unevaluated channel operand became a barrier: %+v", n)
				}
			}
		})
	}
}

func TestActivityGenericChannelRangesRemainUnsupported(t *testing.T) {
	for _, declaration := range []string{
		`func Run[C ~chan int](c C) { for range c {}; println("after") }`,
		`type Channels interface { ~chan int | ~<-chan int }
func Run[C Channels](c C) { for range c {}; println("after") }`,
	} {
		g := activityFor(t, declaration)
		if activityPath(g, g.Entry, g.Exit) {
			t.Fatal("generic channel range became a normal completed iteration")
		}
		barrier := false
		for _, n := range g.Nodes {
			if n.Kind == "unsupported" {
				barrier = true
			}
		}
		if !barrier {
			t.Fatal("generic channel communication was not diagnosed at its range")
		}
	}
}
