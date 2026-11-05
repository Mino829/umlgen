package sequence

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
)

func projectionFixture() Program {
	return Program{Language: "go", Backend: "go/types", Callables: []Callable{
		{ID: "ctx::p.main()", Selector: "p.main()", Qualified: "p.main", Name: "main", Participant: "p", File: "captured.go"},
		{ID: "ctx::p.Worker.run()", Selector: "p.Worker.run()", Qualified: "p.Worker.run", Name: "run", Participant: "p.Worker", File: "captured.go"},
	}}
}

func sourceEvent(kind Kind, label string) Event {
	return Event{Kind: kind, Label: label, File: "captured.go", Span: model.SourceSpan{
		Start: model.SourcePosition{Offset: 10, Line: 2, Column: 1},
		End:   model.SourcePosition{Offset: 20, Line: 2, Column: 11},
	}}
}

func directEvent(target, label string) Event {
	event := sourceEvent(Call, label)
	event.Dispatch, event.Target = "direct", target
	return event
}

func walkProjection(steps []Step, visit func(Step)) {
	for _, step := range steps {
		visit(step)
		walkProjection(step.Children, visit)
		walkProjection(step.Else, visit)
		for _, branch := range step.Branches {
			walkProjection(branch.Steps, visit)
		}
	}
}

func TestProjectDirectRecursionDepthAndPurity(t *testing.T) {
	program := projectionFixture()
	program.Callables[0].Body = []Event{directEvent(program.Callables[1].ID, "invoke worker"), sourceEvent(Return, "return main"), sourceEvent(Note, "unreachable main")}
	program.Callables[1].Body = []Event{directEvent(program.Callables[1].ID, "recur"), sourceEvent(Return, "return worker"), sourceEvent(Note, "unreachable worker")}
	before, _ := json.Marshal(program)
	view, err := Project(program, program.Callables[0], DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if view.Messages != 2 || view.Omissions != 1 || len(view.Participants) != 2 {
		t.Fatalf("wrong bounded recursive view: messages=%d omissions=%d participants=%d", view.Messages, view.Omissions, len(view.Participants))
	}
	walkProjection(view.Steps, func(step Step) {
		if step.Event.Label == "unreachable main" || step.Event.Label == "unreachable worker" {
			t.Fatal("events following a return were projected")
		}
		if step.Event.Kind == Omitted && (step.Event.File != "captured.go" || step.Event.Span.Start.Line != 2) {
			t.Fatal("recursion/depth omission lost its source location")
		}
	})
	after, _ := json.Marshal(program)
	if string(before) != string(after) {
		t.Fatal("projection mutated its input")
	}
	again, err := Project(program, program.Callables[0], DefaultLimits())
	if err != nil || !reflect.DeepEqual(view, again) {
		t.Fatalf("projection is not deterministic: %v", err)
	}
	limits := DefaultLimits()
	limits.MaxCallDepth = 0
	shallow, err := Project(program, program.Callables[0], limits)
	if err != nil || shallow.Messages != 1 || shallow.Omissions != 1 {
		t.Fatalf("zero depth must retain root calls but omit callee bodies: messages=%d omissions=%d err=%v", shallow.Messages, shallow.Omissions, err)
	}
}

func TestProjectNeverExpandsUncertainAsyncOrDeferredTargets(t *testing.T) {
	program := projectionFixture()
	program.Callables[1].Body = []Event{sourceEvent(Note, "must not synchronously appear")}
	for _, dispatch := range []string{"external", "interface", "virtual", "function-value", "unresolved", "builtin"} {
		event := sourceEvent(Call, dispatch)
		event.Dispatch, event.Target = dispatch, program.Callables[1].ID
		event.Candidates = []string{program.Callables[1].ID}
		program.Callables[0].Body = append(program.Callables[0].Body, event)
	}
	for _, kind := range []Kind{Async, Defer} {
		event := directEvent(program.Callables[1].ID, string(kind))
		event.Kind = kind
		program.Callables[0].Body = append(program.Callables[0].Body, event)
	}
	program.Callables[0].Body = append(program.Callables[0].Body, sourceEvent(Send, "ch <- v"), sourceEvent(Receive, "<-ch"))
	view, err := Project(program, program.Callables[0], DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if view.Messages != 10 {
		t.Fatalf("uncertain/async/channel operations must remain visible: %d", view.Messages)
	}
	for _, step := range view.Steps {
		if len(step.Children) > 0 || step.To == "" {
			t.Fatalf("uncertain operation expanded or has no explicit endpoint: %v", step.Event.Kind)
		}
	}
	if view.Steps[8].To != view.Steps[9].To {
		t.Fatal("channel operations must use an explicitly unknown peer, not name-guessed partners")
	}
}

func TestProjectControlAlternativesAndPathTermination(t *testing.T) {
	program := projectionFixture()
	call := directEvent(program.Callables[1].ID, "nested call")
	conditional := sourceEvent(Alt, "if cond")
	conditional.Exhaustive = true
	conditional.Children = []Event{sourceEvent(Return, "return left"), call}
	conditional.Else = []Event{sourceEvent(Return, "return right"), call}
	program.Callables[0].Body = []Event{conditional, call}
	view, err := Project(program, program.Callables[0], DefaultLimits())
	if err != nil || len(view.Steps) != 1 || view.Messages != 0 {
		t.Fatalf("exhaustive terminating branches must end the caller block: steps=%d messages=%d err=%v", len(view.Steps), view.Messages, err)
	}
	conditional.Else = nil // explicit empty else continues
	program.Callables[0].Body = []Event{conditional, call}
	view, err = Project(program, program.Callables[0], DefaultLimits())
	if err != nil || view.Messages != 1 {
		t.Fatalf("an empty else is not a terminated path: messages=%d err=%v", view.Messages, err)
	}
	selection := sourceEvent(Alt, "select")
	selection.Exhaustive = true
	selection.Branches = []Branch{{Label: "case receive", Events: []Event{sourceEvent(Return, "return receive")}}, {Label: "default", Events: []Event{sourceEvent(Return, "return default")}}}
	program.Callables[0].Body = []Event{selection, call}
	view, err = Project(program, program.Callables[0], DefaultLimits())
	if err != nil || view.Messages != 0 || len(view.Steps[0].Branches) != 2 {
		t.Fatalf("select alternatives were flattened: messages=%d err=%v", view.Messages, err)
	}
	loop := sourceEvent(Loop, "for condition")
	guard := sourceEvent(Opt, "rhs only when lhs true")
	guard.Children = []Event{call}
	loop.Children = []Event{guard, sourceEvent(Return, "return iteration")}
	program.Callables[0].Body = []Event{loop, call}
	view, err = Project(program, program.Callables[0], DefaultLimits())
	if err != nil || view.Messages != 2 || view.Steps[0].Children[0].Event.Kind != Opt {
		t.Fatalf("loop/short circuit regions or possible post-loop continuation lost: messages=%d err=%v", view.Messages, err)
	}
}

func TestProjectUnsupportedRegionsAreVisibleBarriers(t *testing.T) {
	program := projectionFixture()
	barrier := sourceEvent(Unsupported, "try transfer not simulated")
	barrier.Branches = []Branch{
		{Label: "try", Events: []Event{directEvent(program.Callables[1].ID, "try call")}},
		{Label: "catch", Events: []Event{sourceEvent(Note, "handler body")}},
		{Label: "finally", Events: []Event{sourceEvent(Note, "cleanup body")}},
	}
	program.Callables[0].Body = []Event{barrier, sourceEvent(Note, "must not flatten continuation")}
	view, err := Project(program, program.Callables[0], DefaultLimits())
	if err != nil || len(view.Steps) != 2 || len(view.Steps[0].Branches) != 3 || view.Steps[1].Event.Kind != Omitted {
		t.Fatalf("uncertain regions or barrier omission lost: steps=%d err=%v", len(view.Steps), err)
	}
	if view.Steps[0].Event.Branches != nil {
		t.Fatal("unprojected region bodies leaked into event payload")
	}
}

func TestProjectMarksBackendTrimmedContinuation(t *testing.T) {
	program := projectionFixture()
	barrier := sourceEvent(Unsupported, "exceptional transfer")
	program.Callables[0].Body = []Event{barrier}
	view, err := Project(program, program.Callables[0], DefaultLimits())
	if err != nil || view.Omissions != 1 || len(view.Steps) != 2 {
		t.Fatalf("trimmed backend continuation was not disclosed: omissions=%d steps=%d err=%v", view.Omissions, len(view.Steps), err)
	}
	marker := view.Steps[1].Event
	if marker.Kind != Omitted || marker.File != barrier.File || marker.Span != barrier.Span {
		t.Fatalf("unsupported continuation lost source provenance: %+v", marker)
	}
}

func TestProjectBudgetsHaveNoDanglingParticipantsAndBoundOmissions(t *testing.T) {
	program := projectionFixture()
	for range 100 {
		program.Callables[0].Body = append(program.Callables[0].Body, directEvent(program.Callables[1].ID, "work"))
	}
	limits := DefaultLimits()
	limits.MaxMessages = 2
	view, err := Project(program, program.Callables[0], limits)
	if err != nil || view.Messages != 2 || view.Omissions != 1 || len(view.Steps) != 3 {
		t.Fatalf("global message budget did not bound output: messages=%d omissions=%d steps=%d err=%v", view.Messages, view.Omissions, len(view.Steps), err)
	}
	limits.MaxParticipants = 1
	view, err = Project(program, program.Callables[0], limits)
	if err != nil || len(view.Participants) != 1 || view.Messages != 0 || view.Omissions != 1 || len(view.Steps) != 1 {
		t.Fatalf("root participant must count and repeated participant omissions stay bounded: %+v err=%v", view.Limits, err)
	}
	ids := make(map[string]bool)
	for _, participant := range view.Participants {
		ids[participant.ID] = true
	}
	walkProjection(view.Steps, func(step Step) {
		if !ids[step.From] || (step.To != "" && !ids[step.To]) {
			t.Fatal("budget created a dangling endpoint")
		}
	})
}

func TestProjectEmptyBodyDoesNotReportDepthOmission(t *testing.T) {
	program := projectionFixture()
	program.Callables[0].Body = []Event{
		directEvent(program.Callables[1].ID, "empty worker"),
		sourceEvent(Note, "caller continuation"),
	}
	limits := DefaultLimits()
	limits.MaxCallDepth = 0
	view, err := Project(program, program.Callables[0], limits)
	if err != nil {
		t.Fatal(err)
	}
	if view.Messages != 1 || view.Omissions != 0 || len(view.Steps) != 2 || len(view.Steps[0].Children) != 0 {
		t.Fatalf("an empty callee has no body to omit: messages=%d omissions=%d steps=%d", view.Messages, view.Omissions, len(view.Steps))
	}
}

func TestProjectUnsupportedArmsBlockOnlyExhaustiveContinuation(t *testing.T) {
	for _, kind := range []Kind{Unsupported, Kind("future-transfer")} {
		for _, namedBranches := range []bool{false, true} {
			for _, exhaustive := range []bool{false, true} {
				program := projectionFixture()
				barrier := sourceEvent(kind, "unmodeled transfer")
				barrier.Children = []Event{directEvent(program.Callables[1].ID, "unknown hidden body")}
				conditional := sourceEvent(Alt, "conditional transfer")
				conditional.Exhaustive = exhaustive
				if namedBranches {
					conditional.Branches = []Branch{
						{Label: "case transfer", Events: []Event{barrier}},
						{Label: "case return", Events: []Event{sourceEvent(Return, "return arm")}},
					}
				} else {
					conditional.Children = []Event{barrier}
					if exhaustive {
						conditional.Else = []Event{sourceEvent(Return, "return arm")}
					}
				}
				program.Callables[0].Body = []Event{conditional, sourceEvent(Note, "outer continuation")}
				view, err := Project(program, program.Callables[0], DefaultLimits())
				if err != nil {
					t.Fatal(err)
				}
				wantSteps := 2
				if exhaustive {
					wantSteps = 1
				}
				if len(view.Steps) != wantSteps || view.Omissions != 1 {
					t.Fatalf("kind=%s named=%t exhaustive=%t: steps=%d want=%d omissions=%d", kind, namedBranches, exhaustive, len(view.Steps), wantSteps, view.Omissions)
				}
				if kind != Unsupported && view.Messages != 0 {
					t.Fatal("unknown event expanded its hidden body")
				}
				walkProjection(view.Steps, func(step Step) {
					if step.Event.Kind == Omitted && (step.Event.File != barrier.File || step.Event.Span != barrier.Span) {
						t.Fatal("unsupported barrier omission lost source provenance")
					}
				})
			}
		}
	}
}

func TestProjectRejectsInvalidLimitsEntriesAndDuplicateIDs(t *testing.T) {
	program := projectionFixture()
	for _, limits := range []Limits{{MaxCallDepth: -1, MaxParticipants: 1, MaxMessages: 1}, {MaxParticipants: 0, MaxMessages: 1}, {MaxParticipants: 1, MaxMessages: 0}} {
		if _, err := Project(program, program.Callables[0], limits); err == nil {
			t.Fatalf("invalid limits accepted: %+v", limits)
		}
	}
	if _, err := Project(program, Callable{ID: "unknown"}, DefaultLimits()); err == nil {
		t.Fatal("foreign entry accepted")
	}
	program.Callables = append(program.Callables, program.Callables[0])
	if _, err := Project(program, program.Callables[0], DefaultLimits()); err == nil {
		t.Fatal("duplicate callable IDs silently overwritten")
	}
}
