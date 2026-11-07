package explain

import (
	"fmt"
	"strings"

	"github.com/Mino829/umlgen/internal/relations"
	"github.com/Mino829/umlgen/internal/state"
)

type StateReport struct {
	View    state.View
	Sources map[string]Source
}

func FormatState(report StateReport) string {
	var out strings.Builder
	view := report.View
	def, analysis := view.Definition, view.Analysis
	fmt.Fprintln(&out, "Explicit state machine explanation (configured contract, not runtime reconstruction):")
	fmt.Fprintf(&out, "Definition: %s\nDefinition SHA256: %s\nConfigured initial: %s\nTarget: %s.%s\nCanonical type ID: %s\nBackend: %s; language: %s\n", def.Source, def.Hash, def.Initial, analysis.Target.Qualified, analysis.Target.Field, analysis.Target.ID, analysis.Backend, analysis.Language)
	fmt.Fprintln(&out, "Configured states/initial/transitions are declarations. Source assignments are candidates with unknown previous state, instance identity, event and runtime execution; no inferred transition is proven.")
	fmt.Fprintln(&out, "Guard/action strings and lexical source guards are not evaluated. Alias, indirect-call, parallel, persistence and implicit-initialization effects can remain unknown. CFG connectivity is syntactic coverage, never an occurrence or reachability proof.")
	fmt.Fprintf(&out, "Selected files: %d; candidate function CFGs: %d; hard node cap per CFG: %d\n", len(analysis.Files), len(analysis.Functions), view.MaxNodes)
	for _, file := range analysis.Files {
		fmt.Fprintf(&out, "  source %s; SHA256 %s\n", file.Path, file.Hash)
	}
	out.WriteString("\nAvailable compiler value selectors:\n")
	for _, value := range analysis.Values {
		fmt.Fprintf(&out, "  %s; qualified=%s; name=%s; literal=%s; key=%s [%s:%d:%d]\n", value.ID, value.Qualified, value.Name, value.Literal, value.Key, value.File, value.Span.Start.Line, value.Span.Start.Column)
	}
	out.WriteString("\nConfigured states:\n")
	for _, s := range view.States {
		fmt.Fprintf(&out, "  %s [configured %s:%d]", s.State.Name, def.Source, s.State.Line)
		if s.Value == nil {
			out.WriteString("; no source value binding\n")
		} else {
			fmt.Fprintf(&out, "; value %s -> %s [key %s]\n", s.State.Value, s.Value.Qualified, s.Value.Key)
		}
	}
	out.WriteString("\nConfigured transitions (not source-proven):\n")
	for _, t := range def.Transitions {
		fmt.Fprintf(&out, "  %s -> %s; event=%s; guard=%s; action=%s [configured %s:%d]\n", t.From, t.To, t.Event, t.Guard, t.Action, def.Source, t.Line)
	}
	source := sourceRenderer{sources: report.Sources, lines: make(map[string][]string, len(report.Sources))}
	out.WriteString("\nSource assignment candidates (previous state UNKNOWN):\n")
	for _, c := range view.Candidates {
		u := c.Update
		fmt.Fprintf(&out, "  %s: %s; receiver=%s; destination=%s; CFG=%s\n", u.Kind, u.Expression, u.Receiver, specified(c.Destination, "unmapped/unknown"), c.Coverage)
		fmt.Fprintf(&out, "    %s:%d:%d [bytes %d:%d, half-open]; function %s; CFG nodes %v\n", u.File, u.Span.Start.Line, u.Span.Start.Column, u.Span.Start.Offset, u.Span.End.Offset, specified(u.FunctionID, "outside function"), u.NodeIDs)
		for _, guard := range u.Guards {
			fmt.Fprintf(&out, "    lexical guard/case (not evaluated): %s\n", guard)
		}
		source.snippet(&out, relations.Evidence{Source: u.File, Line: u.Span.Start.Line})
	}
	fmt.Fprintf(&out, "\nSelected-source diagnostics (%d):\n", len(analysis.Diagnostics))
	for _, d := range analysis.Diagnostics {
		severity := "limitation"
		if d.Error {
			severity = "error"
		}
		fmt.Fprintf(&out, "  %s [%s]: %s [%s:%d:%d]\n", severity, d.Code, d.Message, d.File, d.Span.Start.Line, d.Span.Start.Column)
		source.snippet(&out, relations.Evidence{Source: d.File, Line: d.Span.Start.Line})
	}
	return out.String()
}
