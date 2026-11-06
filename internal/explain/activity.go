package explain

import (
	"fmt"
	"strings"

	"github.com/Mino829/umlgen/internal/activity"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

type ActivityReport struct {
	Graph    activity.Graph
	MaxNodes int
	Sources  map[string]Source
}

// FormatActivity uses captured bytes only, including for malformed/partial
// selections; explanation must not reopen source files after analysis.
func FormatActivity(report ActivityReport) string {
	var out strings.Builder
	g := report.Graph
	fmt.Fprintln(&out, "Static activity CFG explanation (not a runtime trace):")
	fmt.Fprintf(&out, "Function: %s\nCanonical ID: %s\nBackend: %s; language: %s\n", specified(g.Function.Selector, "unavailable"), specified(g.Function.ID, "unavailable"), g.Backend, g.Language)
	fmt.Fprintf(&out, "Selected source files: %d; nodes: %d; edges: %d; hard node cap: %d\n", len(g.Files), len(g.Nodes), len(g.Edges), report.MaxNodes)
	fmt.Fprintln(&out, "Edges describe syntactic possibilities, not runtime values, observed paths, iteration counts or a reachability proof. Calls, implicit initialization and implicit exceptional transfers are not expanded.")
	fmt.Fprintln(&out, "Unsupported nodes end ANALYSIS, not necessarily execution: their continuation is unknown, never silently treated as a normal exit. Selected-source errors do not establish build correctness.")
	source := sourceRenderer{sources: report.Sources, lines: make(map[string][]string, len(report.Sources))}
	location := func(file string, span model.SourceSpan) {
		fmt.Fprint(&out, specified(file, "(synthetic node)"))
		if span.Start.Line > 0 {
			fmt.Fprintf(&out, ":%d:%d-%d:%d [bytes %d:%d, half-open]", span.Start.Line, span.Start.Column, span.End.Line, span.End.Column, span.Start.Offset, span.End.Offset)
		}
		out.WriteByte('\n')
		source.snippet(&out, relations.Evidence{Source: file, Line: span.Start.Line})
	}
	out.WriteString("\nFunction declaration: ")
	location(g.Function.File, g.Function.Span)
	out.WriteString("\nCFG nodes:\n")
	for _, n := range g.Nodes {
		fmt.Fprintf(&out, "  n%d [%s]: %s\n    ", n.ID, n.Kind, n.Label)
		location(n.File, n.Span)
	}
	out.WriteString("\nCFG edges:\n")
	for _, e := range g.Edges {
		fmt.Fprintf(&out, "  n%d -> n%d [%s] %s\n", e.From, e.To, e.Kind, e.Label)
	}
	fmt.Fprintf(&out, "\nSelected-source diagnostics (%d):\n", len(g.Diagnostics))
	for _, d := range g.Diagnostics {
		severity := "limitation"
		if d.Error {
			severity = "error"
		}
		fmt.Fprintf(&out, "  %s [%s]: %s\n    ", severity, d.Code, d.Message)
		location(d.File, d.Span)
	}
	return out.String()
}
