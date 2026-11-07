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
)

func activitySources(t *testing.T, fixture string) []sequence.Source {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "activity", "java", fixture))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	return []sequence.Source{{Path: filepath.Join(root, fixture), Root: root, Content: data}}
}

func javaActivity(t *testing.T, sources []sequence.Source, selector string) activity.Graph {
	t.Helper()
	requireJDK(t)
	g, err := AnalyzeActivity(context.Background(), sources, selector, 500, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Validate(500); err != nil {
		t.Fatal(err)
	}
	if g.Function.Body != nil {
		t.Fatal("activity built sequence events")
	}
	return g
}

func activityNode(t *testing.T, g activity.Graph, kind, label string) activity.Node {
	t.Helper()
	for _, n := range g.Nodes {
		if n.Kind == kind && n.Label == label {
			return n
		}
	}
	t.Fatalf("missing %s node %q: %+v", kind, label, g.Nodes)
	return activity.Node{}
}

func activityEdge(t *testing.T, g activity.Graph, from int, kind string) activity.Edge {
	t.Helper()
	for _, e := range g.Edges {
		if e.From == from && e.Kind == kind {
			return e
		}
	}
	t.Fatalf("missing %s edge from %d", kind, from)
	return activity.Edge{}
}

func activityReachable(g activity.Graph, from, to int) bool {
	seen := map[int]bool{}
	work := []int{from}
	for len(work) > 0 {
		n := work[len(work)-1]
		work = work[:len(work)-1]
		if n == to {
			return true
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		for _, e := range g.Edges {
			if e.From == n {
				work = append(work, e.To)
			}
		}
	}
	return false
}

func TestActivityProcessOrderPathsAndStableSelection(t *testing.T) {
	sources := activitySources(t, "ProcessOrder.java")
	g := javaActivity(t, sources, "processOrder")
	valid := activityNode(t, g, "decision", "order.valid")
	reserve := activityNode(t, g, "action", "reserve(order)")
	if activityReachable(g, activityEdge(t, g, valid.ID, "false").To, reserve.ID) {
		t.Fatal("short-circuited reservation executes")
	}
	if !activityReachable(g, activityEdge(t, g, valid.ID, "true").To, reserve.ID) {
		t.Fatal("reservation missing on valid path")
	}
	loop := activityNode(t, g, "decision", "i < order.attempts")
	send := activityNode(t, g, "action", "send(order)")
	if activityReachable(g, activityEdge(t, g, loop.ID, "false").To, send.ID) {
		t.Fatal("zero-iteration loop sends order")
	}
	update := activityNode(t, g, "merge", "for update")
	cont := activityNode(t, g, "action", "continue;")
	if activityEdge(t, g, cont.ID, "continue").To != update.ID {
		t.Fatal("continue skipped for update")
	}
	audit := activityNode(t, g, "action", "audit(order)")
	notify := activityNode(t, g, "action", "notifyCustomer(order)")
	if !activityReachable(g, audit.ID, notify.ID) {
		t.Fatal("colon switch lost fallthrough")
	}
	for _, selector := range []string{g.Function.Qualified, g.Function.Selector, g.Function.ID} {
		again := javaActivity(t, sources, selector)
		if !reflect.DeepEqual(g, again) {
			t.Fatalf("non-deterministic graph for selector %q", selector)
		}
	}
	hash := sha256.Sum256(sources[0].Content)
	if len(g.Files) != 1 || g.Files[0].Hash != hex.EncodeToString(hash[:]) {
		t.Fatal("snapshot provenance lost")
	}
	limited, err := AnalyzeActivity(context.Background(), sources, "processOrder", 1, Options{})
	if !errors.Is(err, activity.ErrLimit) || len(limited.Nodes) != 0 || limited.Function.ID == "" {
		t.Fatalf("cap published partial graph or lost identity: %+v, %v", limited, err)
	}
}

func TestActivityLoopsLabeledAndNestedTransfers(t *testing.T) {
	g := javaActivity(t, activitySources(t, "Transfers.java"), "loops")
	cont := activityNode(t, g, "action", "continue outer;")
	update := activityNode(t, g, "merge", "for update")
	if activityEdge(t, g, cont.ID, "continue").To != update.ID {
		t.Fatal("labeled continue did not reach outer update")
	}
	stop := activityNode(t, g, "action", "break outer;")
	doBody := activityNode(t, g, "merge", "do body")
	if activityEdge(t, g, stop.ID, "break").To != doBody.ID {
		t.Fatal("labeled break stayed inside loop")
	}
	doHit := activityNode(t, g, "action", `hit("do")`)
	if !activityReachable(g, doBody.ID, doHit.ID) {
		t.Fatal("do loop omitted first iteration")
	}
	var doContinue, enhancedContinue bool
	doTest := activityNode(t, g, "merge", "do condition")
	enhanced := activityNode(t, g, "decision", "enhanced for: next element?")
	for _, n := range g.Nodes {
		if n.Label == "continue;" {
			target := activityEdge(t, g, n.ID, "continue").To
			doContinue = doContinue || target == doTest.ID
			enhancedContinue = enhancedContinue || target == enhanced.ID
		}
	}
	if !doContinue || !enhancedContinue {
		t.Fatal("continue targets lost do-test or enhanced iterator")
	}
	after := activityNode(t, g, "action", `hit("after")`)
	if activityEdge(t, g, activityNode(t, g, "action", "break done;").ID, "break").To != after.ID {
		t.Fatal("labeled block break target incorrect")
	}
	if !activityReachable(g, activityEdge(t, g, enhanced.ID, "false").To, after.ID) {
		t.Fatal("enhanced for has no zero-iteration path")
	}
	if len(g.Diagnostics) == 0 {
		t.Fatal("missing intraprocedural/iterator limits")
	}
}

func TestActivitySwitchColonArrowAndNestedBreak(t *testing.T) {
	g := javaActivity(t, activitySources(t, "Transfers.java"), "switches")
	colon := activityNode(t, g, "action", `hit("colon")`)
	outer := activityNode(t, g, "action", `hit("outer")`)
	if !activityReachable(g, colon.ID, outer.ID) {
		t.Fatal("colon fallthrough lost")
	}
	var innerBreak bool
	for _, e := range g.Edges {
		innerBreak = innerBreak || e.Kind == "break" && e.To == outer.ID
	}
	if !innerBreak {
		t.Fatal("nested switch break escaped outer switch")
	}
	zero := activityNode(t, g, "action", `hit("arrow-zero")`)
	other := activityNode(t, g, "action", `hit("arrow-default")`)
	after := activityNode(t, g, "action", `hit("after")`)
	if activityReachable(g, zero.ID, other.ID) || !activityReachable(g, zero.ID, after.ID) {
		t.Fatal("arrow case falls through")
	}
}

func TestActivityLiteralRegionsAndExplicitThrow(t *testing.T) {
	sources := activitySources(t, "Transfers.java")
	g := javaActivity(t, sources, "literals")
	for _, n := range g.Nodes {
		if n.Kind == "action" && (n.Label == `hit("local-body")` || n.Label == `hit("lambda-body")` || n.Label == `hit("anonymous-body")`) {
			t.Fatalf("declaration body executed inline: %+v", n)
		}
	}
	value := activityNode(t, g, "decision", "value")
	var firstCall, secondCall, firstTest activity.Node
	firstOffset := strings.Index(string(sources[0].Content), "value && test()") + len("value && ")
	for _, n := range g.Nodes {
		if n.Label == "test()" && n.Kind == "action" {
			if n.Span.Start.Offset == firstOffset {
				firstCall = n
			} else {
				secondCall = n
			}
		}
		if n.Label == "test()" && n.Kind == "decision" && n.Span.Start.Offset == firstOffset {
			firstTest = n
		}
	}
	if firstCall.ID == 0 || secondCall.ID == 0 || firstTest.ID == 0 {
		t.Fatal("short-circuit calls/decisions missing")
	}
	skipped := activityEdge(t, g, value.ID, "false").To
	if activityReachable(g, skipped, firstCall.ID) || !activityReachable(g, skipped, secondCall.ID) {
		t.Fatal("&& false path evaluates wrong operand")
	}
	if activityReachable(g, activityEdge(t, g, firstTest.ID, "true").To, secondCall.ID) {
		t.Fatal("|| true path evaluates right operand")
	}
	g = javaActivity(t, sources, "exceptional")
	throw := activityNode(t, g, "throw", `throw new IllegalStateException("explicit");`)
	if activityEdge(t, g, throw.ID, "throw").To != g.Exit {
		t.Fatal("explicit throw does not end path")
	}
	if activityReachable(g, throw.ID, activityNode(t, g, "action", `hit("normal")`).ID) {
		t.Fatal("throw acquired normal successor")
	}
}

func TestActivityUnsupportedRegionsAreTerminal(t *testing.T) {
	sources := activitySources(t, "Transfers.java")
	for _, selector := range []string{"resources", "monitor"} {
		g := javaActivity(t, sources, selector)
		var barrier *activity.Node
		for i := range g.Nodes {
			if g.Nodes[i].Kind == "unsupported" {
				barrier = &g.Nodes[i]
			}
		}
		if barrier == nil || barrier.Span.Start.Offset >= barrier.Span.End.Offset {
			t.Fatal("missing located barrier")
		}
		for _, e := range g.Edges {
			if e.From == barrier.ID {
				t.Fatal("barrier has outgoing edge")
			}
		}
		for _, n := range g.Nodes {
			if strings.Contains(n.Label, "-body") {
				t.Fatalf("unsupported region flattened: %+v", n)
			}
			if n.Label == `hit("unreachable-after-barrier")` && activityReachable(g, g.Entry, n.ID) {
				t.Fatal("barrier permits normal continuation")
			}
		}
	}
}

func TestActivityPartialMalformedUnicodeAndSelectionErrors(t *testing.T) {
	requireJDK(t)
	root := t.TempDir()
	text := `class Sample { static void hit(String s) {} static void entry() { String s="日本😀"; hit("é"); Missing.call(); } static void entry(int i) {} static void broken() { int n = ; } }`
	sources := []sequence.Source{{Path: filepath.Join(root, "Sample.java"), Root: root, Content: []byte(text)}}
	g, err := AnalyzeActivity(context.Background(), sources, "entry", 500, Options{})
	if !errors.Is(err, activity.ErrSelection) || !errors.Is(err, sequence.ErrAmbiguousEntry) || len(g.Files) != 1 || len(g.Diagnostics) == 0 {
		t.Fatalf("selection sentinels/metadata missing: %+v %v", g, err)
	}
	g = javaActivity(t, sources, "Sample.entry()")
	hit := activityNode(t, g, "action", `hit("é")`)
	start := strings.Index(text, `hit("é")`)
	if hit.Span.Start.Offset != start || hit.Span.End.Offset != start+len(`hit("é")`) {
		t.Fatalf("not captured UTF-8 offsets: %+v", hit.Span)
	}
	activityNode(t, g, "action", "Missing.call()")
	for _, selector := range []string{"broken", "absent"} {
		failed, err := AnalyzeActivity(context.Background(), sources, selector, 500, Options{})
		if !errors.Is(err, activity.ErrSelection) || len(failed.Diagnostics) == 0 || len(failed.Nodes) != 0 {
			t.Fatalf("damaged/missing selection: %+v %v", failed, err)
		}
	}
	invalid := []sequence.Source{{Path: filepath.Join(root, "Invalid.java"), Root: root, Content: []byte{0xff}}}
	if _, err := AnalyzeActivity(context.Background(), invalid, "entry", 500, Options{}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

func TestActivitySwitchExpressionAndAssertBarriers(t *testing.T) {
	requireJDK(t)
	root := t.TempDir()
	text := `class Sample { static int entry(int n) { return switch(n) { case 0 -> 1; default -> 2; }; } static void assertion(boolean b) { assert b; } }`
	sources := []sequence.Source{{Path: filepath.Join(root, "Sample.java"), Root: root, Content: []byte(text)}}
	for _, selector := range []string{"entry", "assertion"} {
		g := javaActivity(t, sources, selector)
		var found bool
		for _, n := range g.Nodes {
			if n.Kind == "unsupported" {
				found = true
				for _, e := range g.Edges {
					if e.From == n.ID {
						t.Fatal("unsupported expression has successor")
					}
				}
			}
		}
		if !found {
			t.Fatalf("%s silently flattened", selector)
		}
	}
}

func TestActivityConditionalExpressionsInsideArgumentsAndReturns(t *testing.T) {
	g := javaActivity(t, activitySources(t, "Transfers.java"), "alternatives")
	var choices int
	for _, n := range g.Nodes {
		if n.Kind != "decision" || n.Label != "value" {
			continue
		}
		yes, no := activityEdge(t, g, n.ID, "true"), activityEdge(t, g, n.ID, "false")
		var yesNode, noNode activity.Node
		for _, candidate := range g.Nodes {
			if candidate.ID == yes.To {
				yesNode = candidate
			}
			if candidate.ID == no.To {
				noNode = candidate
			}
		}
		if yesNode.Label == "one()" && noNode.Label == "two()" {
			choices++
			if activityReachable(g, yes.To, no.To) || activityReachable(g, no.To, yes.To) {
				t.Fatal("ternary evaluates both operands")
			}
		}
	}
	if choices != 2 {
		t.Fatalf("assignment/return-argument ternaries flattened: %d", choices)
	}
	var shortCircuit bool
	for _, n := range g.Nodes {
		if n.Kind == "decision" && n.Label == "value" {
			yes := activityEdge(t, g, n.ID, "true").To
			for _, candidate := range g.Nodes {
				shortCircuit = shortCircuit || candidate.ID == yes && candidate.Kind == "action" && candidate.Label == "test()"
			}
		}
	}
	if !shortCircuit {
		t.Fatal("conditional call argument was flattened")
	}
	var returnNode bool
	for _, n := range g.Nodes {
		returnNode = returnNode || n.Kind == "return"
	}
	if !returnNode {
		t.Fatal("return transfer missing after conditional argument evaluation")
	}
}

func TestActivityTypeOnlyQualifiersPreserveContinuation(t *testing.T) {
	for _, declaration := range []string{
		`Class<?> value = int.class`,
		`Class<?> value = String[].class`,
		`java.util.function.IntFunction<String[]> value = String[]::new`,
		`java.util.function.Supplier<java.util.ArrayList<String>> value = java.util.ArrayList<String>::new`,
	} {
		t.Run(declaration, func(t *testing.T) {
			root := t.TempDir()
			source := sequence.Source{Path: filepath.Join(root, "Sample.java"), Root: root, Content: []byte(`class Sample {
static void hit() {}
static void entry() { ` + declaration + `; hit(); }
}`)}
			g := javaActivity(t, []sequence.Source{source}, "entry")
			hit := activityNode(t, g, "action", "hit()")
			if !activityReachable(g, g.Entry, hit.ID) || !activityReachable(g, hit.ID, g.Exit) {
				t.Fatal("type-only qualifier became a runtime evaluation barrier")
			}
			for _, n := range g.Nodes {
				if n.Kind == "unsupported" {
					t.Fatalf("type literal/reference became unsupported: %+v", n)
				}
			}
		})
	}
}
