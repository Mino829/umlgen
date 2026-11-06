package explain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/sequence"
)

func TestFormatSequenceUsesCapturedEvidenceAndExplainsStaticLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "captured.go")
	captured := []byte("func captured_entry() int {\n  captured_second()\n  captured_third()\n  fourth_line_must_not_be_shown()\n}\n")
	if err := os.WriteFile(path, []byte("disk_replacement_must_not_be_read"), 0600); err != nil {
		t.Fatal(err)
	}
	span := model.SourceSpan{Start: model.SourcePosition{Offset: 0, Line: 1, Column: 1}, End: model.SourcePosition{Offset: 27, Line: 1, Column: 28}}
	entry := sequence.Callable{ID: "ctx::p.entry()", Selector: "p.entry()", Returns: "int", File: path, Span: span}
	target := sequence.Callable{ID: "ctx::p.run()", Selector: "p.run()", Returns: "(string, error)"}
	event := sequence.Event{Kind: sequence.Call, Label: "invoke selected declaration", Target: target.ID, Dispatch: "direct", File: path, Span: span}
	dynamic := sequence.Event{Kind: sequence.Call, Label: "invoke receiver", Dispatch: "interface", Candidates: []string{"p.A.run()", "p.B.run()"}, File: path, Span: span}
	barrier := sequence.Step{From: "p", Event: sequence.Event{Kind: sequence.Unsupported, Label: "try transfer is not simulated", File: path, Span: span}, Branches: []sequence.StepBranch{
		{Label: "catch", Steps: []sequence.Step{{From: "p", Event: sequence.Event{Kind: sequence.Note, Label: "handler source region", File: path, Span: span}}}},
		{Label: "finally", Steps: []sequence.Step{{From: "p", Event: sequence.Event{Kind: sequence.Note, Label: "cleanup source region", File: path, Span: span}}}},
	}}
	report := SequenceReport{
		Sources: map[string]Source{path: {Content: captured, Origin: "captured analysis snapshot"}},
		View: sequence.View{
			Program: sequence.Program{Language: "go", Backend: "go/types selected-source", Files: []sequence.File{{Path: path}}, Callables: []sequence.Callable{entry, target}, Diagnostics: []sequence.Diagnostic{{Code: "unresolved-import", Message: "external library unavailable", File: path, Span: span, Error: true}}},
			Entry:   entry, Limits: sequence.Limits{MaxCallDepth: 2, MaxParticipants: 5, MaxMessages: 7}, Messages: 2, Omissions: 1,
			Steps: []sequence.Step{
				{From: "p", To: "p.Worker", Event: event}, {From: "p", To: "unknown", Event: dynamic}, barrier,
				{From: "p", Event: sequence.Event{Kind: sequence.Omitted, Label: "depth budget reached", File: path, Span: span}},
			},
		},
	}
	before, _ := json.Marshal(report)
	text := FormatSequence(report)
	for _, required := range []string{"p.entry()", "ctx::p.entry()", "go/types selected-source", "int", "(string, error)", "captured_entry", "captured_second", "captured_third", path + ":1:1", "bytes 0:27", "half-open", "call depth=2", "participants=5", "messages=7", "interface", "p.A.run()", "p.B.run()", "try transfer", "handler source region", "cleanup source region", "depth budget", "unresolved-import", "external library unavailable", "not a runtime trace", "Defer", "Channel", "non-call operand"} {
		if !strings.Contains(text, required) {
			t.Errorf("explanation lost source/semantic evidence %q:\n%s", required, text)
		}
	}
	for _, forbidden := range []string{"disk_replacement_must_not_be_read", "fourth_line_must_not_be_shown", "{Kind:", "SourceSpan{"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("explanation read uncaptured data, exceeded snippet cap, or echoed raw structs: %q", forbidden)
		}
	}
	after, _ := json.Marshal(report)
	if string(before) != string(after) || FormatSequence(report) != text {
		t.Fatal("explanation mutated input or is nondeterministic")
	}
}

func TestFormatSequenceMissingSnapshotsAndUnknownLocationsRemainExplicit(t *testing.T) {
	report := SequenceReport{View: sequence.View{
		Entry:  sequence.Callable{ID: "ctx::p.entry()", Selector: "p.entry()", File: "/unreadable/entry.go"},
		Limits: sequence.DefaultLimits(),
		Steps:  []sequence.Step{{From: "p", Event: sequence.Event{Kind: sequence.Call, Dispatch: "unresolved", Label: "unknown call", File: "/unreadable/call.go"}}},
	}}
	text := FormatSequence(report)
	for _, required := range []string{"/unreadable/entry.go", "/unreadable/call.go", "unresolved", "unknown call", "unavailable", "no captured source"} {
		if !strings.Contains(text, required) {
			t.Errorf("missing-source explanation concealed %q: %s", required, text)
		}
	}
	report.Sources = map[string]Source{"/unreadable/call.go": {Content: []byte("only one line"), Origin: "snapshot"}}
	report.View.Steps[0].Event.Span.Start.Line = 99
	text = FormatSequence(report)
	if !strings.Contains(text, "99") || strings.Contains(text, "1 | only one line") {
		t.Fatal("out-of-range location was silently rebound to another source line")
	}
}
