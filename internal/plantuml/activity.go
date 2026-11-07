package plantuml

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Mino829/umlgen/internal/activity"
)

type ActivityOptions struct {
	Title    string
	MaxNodes int
}

// GenerateActivity uses explicit legacy activity nodes/edges instead of trying
// to reconstruct structured if/while syntax from arbitrary CFG transfers. A
// drawn edge corresponds to one graph edge; arrow labels retain transfer kinds.
func GenerateActivity(graph activity.Graph, opts ActivityOptions) (string, error) {
	if opts.MaxNodes == 0 {
		opts.MaxNodes = activity.DefaultMaxNodes
	}
	if err := graph.Validate(opts.MaxNodes); err != nil {
		return "", err
	}
	paths := sequenceSourcePaths(graph.Files)
	type nodeText struct {
		label, alias string
		declared     bool
	}
	nodes := make(map[int]nodeText, len(graph.Nodes))
	for _, n := range graph.Nodes {
		label := packageText(n.Kind + ": " + n.Label)
		if loc := sequenceLocation(n.File, n.Span.Start.Line, n.Span.Start.Column, paths); loc != "" {
			label += "\\n" + packageText("["+loc+"]")
		}
		nodes[n.ID] = nodeText{label: label, alias: "a" + strconv.Itoa(n.ID)}
	}
	endpoint := func(id int) string {
		node := nodes[id]
		if node.declared {
			return node.alias
		}
		node.declared = true
		nodes[id] = node
		return "\"" + node.label + "\" as " + node.alias
	}
	var out strings.Builder
	out.WriteString("@startuml\n")
	if opts.Title != "" {
		fmt.Fprintf(&out, "title %s\n", packageText(opts.Title))
	}
	fmt.Fprintf(&out, "(*) --> %s\n", endpoint(graph.Entry))
	fmt.Fprintf(&out, "%s --> (*)\n", endpoint(graph.Exit))
	for _, edge := range graph.Edges {
		label := edge.Kind
		if edge.Label != "" && edge.Label != edge.Kind {
			label += ": " + edge.Label
		}
		from, to := endpoint(edge.From), endpoint(edge.To)
		fmt.Fprintf(&out, "%s -->[%s] %s\n", from, packageText(label), to)
	}
	for _, node := range graph.Nodes {
		if !nodes[node.ID].declared {
			// Isolated (unreachable) blocks still need a declaration in legacy
			// syntax. This invisible self-link is layout only, not a CFG edge.
			fmt.Fprintf(&out, "%s -[hidden]-> a%d\n", endpoint(node.ID), node.ID)
		}
	}
	out.WriteString("\nlegend bottom\nStatic intraprocedural CFG; NOT a runtime trace or reachability proof\n")
	fmt.Fprintf(&out, "Function: %s\nBackend: %s; language: %s; selected files: %d\n", packageText(graph.Function.Selector), packageText(graph.Backend), packageText(graph.Language), len(graph.Files))
	fmt.Fprintf(&out, "CFG nodes: %d; edges: %d; hard node cap: %d\n", len(graph.Nodes), len(graph.Edges), opts.MaxNodes)
	out.WriteString("Labeled edges are source-level possibilities, not executed paths or iteration counts\nCalls, implicit initialization and implicit exceptions/panics are not expanded\nUnsupported nodes terminate ANALYSIS only; subsequent runtime behavior is unknown\nDisconnected blocks are not made reachable; entry/exit markers are presentation only\nSelected-source errors can leave partial analysis; this does not establish build correctness\n")
	for _, d := range graph.Diagnostics {
		fmt.Fprintf(&out, "Diagnostic %s: %s [%s]\n", packageText(d.Code), packageText(d.Message), packageText(sequenceLocation(d.File, d.Span.Start.Line, d.Span.Start.Column, paths)))
	}
	out.WriteString("endlegend\n@enduml\n")
	return out.String(), nil
}
