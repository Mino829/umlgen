package plantuml

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/sequence"
)

func sequenceDiagramFixture() sequence.View {
	return sequence.View{
		Program:      sequence.Program{Language: "go", Backend: "go/types", Files: []sequence.File{{Path: "captured.go"}}},
		Entry:        sequence.Callable{ID: "ctx::p.main()", Selector: "p.main()"},
		Limits:       sequence.DefaultLimits(),
		Participants: []sequence.Participant{{ID: "caller", Label: "p.Main"}, {ID: "callee", Label: "p.Worker"}},
	}
}

func diagramStep(kind sequence.Kind, label string) sequence.Step {
	return sequence.Step{From: "caller", To: "callee", Event: sequence.Event{Kind: kind, Label: label, File: "captured.go", Span: model.SourceSpan{Start: model.SourcePosition{Line: 3, Column: 2}}, Dispatch: "direct"}}
}

func TestGenerateSequenceRendersSourceRegionsAndExplicitUncertainty(t *testing.T) {
	view := sequenceDiagramFixture()
	conditional := diagramStep(sequence.Alt, "if ready")
	conditional.To = ""
	conditional.Children = []sequence.Step{diagramStep(sequence.Call, "ready work")}
	conditional.Else = []sequence.Step{diagramStep(sequence.Return, "return otherwise")}
	loop := diagramStep(sequence.Loop, "for condition; count unknown")
	guard := diagramStep(sequence.Opt, "short circuit rhs")
	guard.Children = []sequence.Step{diagramStep(sequence.Call, "conditional operand")}
	loop.Children = []sequence.Step{guard}
	selection := diagramStep(sequence.Alt, "select source region")
	selection.Branches = []sequence.StepBranch{
		{Label: "case receive", Steps: []sequence.Step{diagramStep(sequence.Receive, "<-ch")}},
		{Label: "default", Steps: []sequence.Step{diagramStep(sequence.Note, "default path")}},
	}
	dynamic := diagramStep(sequence.Call, "invoke virtual")
	dynamic.Event.Dispatch = "virtual"
	dynamic.Event.Candidates = []string{"p.A.run()", "p.B.run()"}
	barrier := diagramStep(sequence.Unsupported, "try transfer not simulated")
	barrier.Branches = []sequence.StepBranch{
		{Label: "try", Steps: []sequence.Step{diagramStep(sequence.Call, "try work")}},
		{Label: "catch", Steps: []sequence.Step{diagramStep(sequence.Note, "handler work")}},
		{Label: "finally", Steps: []sequence.Step{diagramStep(sequence.Note, "cleanup work")}},
	}
	view.Steps = []sequence.Step{conditional, loop, selection, dynamic, diagramStep(sequence.Async, "go work"), diagramStep(sequence.Defer, "defer close"), diagramStep(sequence.Send, "ch <- v"), barrier, diagramStep(sequence.Omitted, "depth budget")}
	text, err := GenerateSequence(view, SequenceOptions{Title: "Source interactions"})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"@startuml", "@enduml", "participant", "alt ", "else ", "opt ", "loop ", "group ", "->>", "-->", "path ends", "virtual", "open", "async launch", "defer registration", "peer unknown", "try transfer", "execution/transfer NOT simulated", "finally", "omitted", "NOT a runtime trace", "Limits:", "Backend:", "captured"} {
		if !strings.Contains(text, required) {
			t.Errorf("diagram lost source semantics %q:\n%s", required, text)
		}
	}
	if strings.Count(text, "invoke virtual") != 1 {
		t.Fatal("dynamic candidates were expanded as sequential invocations")
	}
	if strings.Contains(text, "alt finally") || strings.Contains(text, "else finally") {
		t.Fatal("finally was falsely modeled as an exclusive alternative to try/catch")
	}
}

func TestGenerateSequenceEscapesEveryUntrustedSurfaceAndIsPure(t *testing.T) {
	payload := "\"\n@enduml\n!include https://invalid.example/x\n@startuml\n[[evil]] <img:evil> { } # red"
	view := sequenceDiagramFixture()
	view.Entry.Selector = payload
	view.Program.Backend, view.Program.Language = payload, payload
	view.Participants[0].ID, view.Participants[0].Label = payload, payload
	view.Program.Diagnostics = []sequence.Diagnostic{{Code: payload, Message: payload, File: payload}}
	call := diagramStep(sequence.Call, payload)
	call.From, call.Event.File, call.Event.Dispatch = payload, payload, payload
	call.Event.Candidates = []string{payload}
	control := diagramStep(sequence.Alt, payload)
	control.From, control.Event.File = payload, payload
	control.Branches = []sequence.StepBranch{{Label: payload, Steps: []sequence.Step{call}}}
	barrier := diagramStep(sequence.Unsupported, payload)
	barrier.From, barrier.Event.File = payload, payload
	barrier.Branches = []sequence.StepBranch{{Label: payload, Steps: []sequence.Step{call}}}
	view.Steps = []sequence.Step{control, barrier}
	before, _ := json.Marshal(view)
	text, err := GenerateSequence(view, SequenceOptions{Title: payload})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(text, "@startuml") != 1 || strings.Count(text, "@enduml") != 1 || strings.Contains(text, "!include") || strings.Contains(text, "[[evil]]") || strings.Contains(text, "<img:") {
		t.Fatalf("untrusted data escaped PlantUML text context:\n%s", text)
	}
	if !strings.Contains(text, "<U+") {
		t.Fatal("punctuation was dropped instead of safely represented")
	}
	after, _ := json.Marshal(view)
	if string(before) != string(after) {
		t.Fatal("rendering changed the projection")
	}
	again, err := GenerateSequence(view, SequenceOptions{Title: payload})
	if err != nil || again != text {
		t.Fatalf("rendering aliases/output are not stable: %v", err)
	}
}

func TestGenerateSequenceRejectsDanglingEndpointsAndInventedExpansion(t *testing.T) {
	for _, malformed := range []func(*sequence.View){
		func(v *sequence.View) { v.Steps[0].From = "absent" },
		func(v *sequence.View) { v.Steps[0].To = "absent" },
		func(v *sequence.View) { v.Participants = append(v.Participants, v.Participants[0]) },
		func(v *sequence.View) { v.Steps[0].Event.Kind = sequence.Kind("invented") },
		func(v *sequence.View) {
			v.Steps[0].Event.Kind = sequence.Async
			v.Steps[0].Children = []sequence.Step{diagramStep(sequence.Note, "fake synchronous body")}
		},
	} {
		view := sequenceDiagramFixture()
		view.Steps = []sequence.Step{diagramStep(sequence.Call, "work")}
		malformed(&view)
		if text, err := GenerateSequence(view, SequenceOptions{}); err == nil || text != "" {
			t.Fatalf("malformed projection must fail without a partial diagram: text=%q err=%v", text, err)
		}
	}
}
