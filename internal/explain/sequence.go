package explain

import (
	"fmt"
	"strings"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
	"github.com/Mino829/umlgen/internal/sequence"
)

// SequenceReport contains captured inputs only; formatting never rereads files.
type SequenceReport struct {
	View    sequence.View
	Sources map[string]Source
}

// FormatSequence explains source evidence and the boundaries of the projection.
func FormatSequence(report SequenceReport) string {
	var out strings.Builder
	view := report.View
	fmt.Fprintln(&out, "Static sequence explanation (not a runtime trace):")
	fmt.Fprintf(&out, "Entry: %s\nCanonical ID: %s\nDeclared return types: %s\n", view.Entry.Selector, view.Entry.ID, specified(view.Entry.Returns, "none declared"))
	fmt.Fprintf(&out, "Backend: %s; language: %s\nSelected source files: %d; selected-source callables: %d\n", view.Program.Backend, view.Program.Language, len(view.Program.Files), len(view.Program.Callables))
	fmt.Fprintf(&out, "Limits: call depth=%d, participants=%d, messages=%d\nProjected messages: %d; visible omissions: %d\n", view.Limits.MaxCallDepth, view.Limits.MaxParticipants, view.Limits.MaxMessages, view.Messages, view.Omissions)
	fmt.Fprintln(&out, "Only statically direct selected-source calls can expand. External/unresolved/dynamic bodies are unknown; candidate targets are alternatives, not sequential executions.")
	fmt.Fprintln(&out, "Branches/loops preserve source regions, not observed paths, values, iteration counts or implicit initialization. Return/path-end markers apply to their source callable or region.")
	fmt.Fprintln(&out, "Async launches have no inferred join. Defer registers only; conditional LIFO execution at exit/panic is not simulated. Channel peers and runtime values are not inferred.")
	fmt.Fprintln(&out, "Selected-source semantic diagnostics and unsupported constructs can leave partial analysis; this does not establish build/compiler correctness or absent calls.")
	if view.Program.Language == "go" {
		fmt.Fprintln(&out, "Go ordering covers lexical calls/receives only, not non-call operand evaluation order.")
	}
	renderer := sequenceExplainer{
		out:       &out,
		source:    sourceRenderer{sources: report.Sources, lines: make(map[string][]string, len(report.Sources))},
		callables: make(map[string]*sequence.Callable, len(view.Program.Callables)),
	}
	for i := range view.Program.Callables {
		c := &view.Program.Callables[i]
		renderer.callables[c.ID] = c
	}
	out.WriteString("\nEntry declaration: ")
	renderer.location(view.Entry.File, view.Entry.Span)
	renderer.source.snippet(&out, relations.Evidence{Source: view.Entry.File, Line: view.Entry.Span.Start.Line})
	out.WriteString("\nProjected source regions:\n")
	renderer.steps(view.Steps, "  ")
	fmt.Fprintf(&out, "\nSelected-source diagnostics (%d):\n", len(view.Program.Diagnostics))
	for _, diagnostic := range view.Program.Diagnostics {
		severity := "limitation"
		if diagnostic.Error {
			severity = "error"
		}
		fmt.Fprintf(&out, "  %s [%s]: %s\n    ", severity, diagnostic.Code, diagnostic.Message)
		renderer.location(diagnostic.File, diagnostic.Span)
		renderer.source.snippet(&out, relations.Evidence{Source: diagnostic.File, Line: diagnostic.Span.Start.Line})
	}
	return out.String()
}

type sequenceExplainer struct {
	out       *strings.Builder
	source    sourceRenderer
	callables map[string]*sequence.Callable
}

func (r *sequenceExplainer) location(file string, span model.SourceSpan) {
	fmt.Fprint(r.out, specified(file, "(source unknown)"))
	if span.Start.Line > 0 {
		fmt.Fprintf(r.out, ":%d:%d-%d:%d [bytes %d:%d, half-open]", span.Start.Line, span.Start.Column, span.End.Line, span.End.Column, span.Start.Offset, span.End.Offset)
	}
	r.out.WriteByte('\n')
}

func (r *sequenceExplainer) steps(steps []sequence.Step, indent string) {
	for _, step := range steps {
		event := step.Event
		fmt.Fprintf(r.out, "%s%s: %s\n", indent, event.Kind, event.Label)
		if step.To != "" {
			fmt.Fprintf(r.out, "%s  Participants: %s -> %s\n", indent, step.From, step.To)
		}
		if event.Dispatch != "" {
			fmt.Fprintf(r.out, "%s  Binding: %s\n", indent, event.Dispatch)
		}
		if target := r.callables[event.Target]; target != nil {
			fmt.Fprintf(r.out, "%s  Selected declaration: %s; declared return types: %s\n", indent, target.Selector, specified(target.Returns, "none declared"))
		}
		if len(event.Candidates) > 0 {
			fmt.Fprintf(r.out, "%s  Possible dispatch targets (open-world alternatives, never sequential calls): %s\n", indent, strings.Join(event.Candidates, ", "))
		}
		fmt.Fprint(r.out, indent+"  ")
		r.location(event.File, event.Span)
		r.source.snippet(r.out, relations.Evidence{Source: event.File, Line: event.Span.Start.Line})
		if len(step.Children) > 0 {
			fmt.Fprintln(r.out, indent+"  Nested source region:")
			r.steps(step.Children, indent+"    ")
		}
		if len(step.Else) > 0 {
			fmt.Fprintln(r.out, indent+"  Otherwise:")
			r.steps(step.Else, indent+"    ")
		}
		for _, branch := range step.Branches {
			fmt.Fprintf(r.out, "%s  Alternative: %s\n", indent, branch.Label)
			r.steps(branch.Steps, indent+"    ")
		}
	}
}
