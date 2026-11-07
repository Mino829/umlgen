package plantuml

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Mino829/umlgen/internal/state"
)

type StateOptions struct{ Title string }

// GenerateState never promotes source writes into configured transitions. Source
// candidates only originate at a distinct unknown-previous-state evidence node.
func GenerateState(view state.View, opts StateOptions) (string, error) {
	definition := view.Definition
	if err := definition.Validate(); err != nil {
		return "", err
	}
	if len(view.States) != len(definition.States) {
		return "", fmt.Errorf("state view is missing its configured/value bindings")
	}
	aliases := make(map[string]string, len(view.States))
	var out strings.Builder
	out.WriteString("@startuml\n")
	if opts.Title != "" {
		fmt.Fprintf(&out, "title %s\n", packageText(opts.Title))
	}
	for i, bound := range view.States {
		name := bound.State.Name
		if name != definition.States[i].Name {
			return "", fmt.Errorf("state view does not match its definition")
		}
		alias := "ST_" + strconv.Itoa(i)
		aliases[name] = alias
		label := packageText(name)
		if bound.Value != nil {
			label += "\\n" + packageText("value: "+bound.State.Value)
		}
		fmt.Fprintf(&out, "state \"%s\" as %s\n", label, alias)
	}
	fmt.Fprintf(&out, "[*] --> %s : configured initial\n", aliases[definition.Initial])
	for _, t := range definition.Transitions {
		label := "configured: " + t.Event
		if t.Guard != "" {
			label += " [" + t.Guard + "]"
		}
		if t.Action != "" {
			label += " / " + t.Action
		}
		fmt.Fprintf(&out, "%s --> %s : %s\\n%s\n", aliases[t.From], aliases[t.To], packageText(label), packageText(fmt.Sprintf("[%s:%d]", filepath.Base(definition.Source), t.Line)))
	}
	paths := sequenceSourcePaths(view.Analysis.Files)
	// Group candidates by destination without merging their source evidence.
	// One parallel arrow per write quickly obscures the configured machine.
	groups := make(map[string][]int, len(view.States))
	for i := range view.Candidates {
		candidate := &view.Candidates[i]
		if candidate.Destination == "" {
			continue
		}
		if _, found := aliases[candidate.Destination]; !found {
			return "", fmt.Errorf("source candidate names undeclared state %q", candidate.Destination)
		}
		groups[candidate.Destination] = append(groups[candidate.Destination], i)
	}
	if len(groups) > 0 {
		out.WriteString("state \"Unknown previous state\\nsource evidence only\" as UNKNOWN_PREVIOUS\n")
	}
	for _, bound := range view.States {
		indices := groups[bound.State.Name]
		if len(indices) == 0 {
			continue
		}
		var label strings.Builder
		label.WriteString("assignment candidates")
		for _, i := range indices {
			candidate := &view.Candidates[i]
			u := &candidate.Update
			loc := sequenceLocation(u.File, u.Span.Start.Line, u.Span.Start.Column, paths)
			label.WriteString("\\n")
			label.WriteString(packageText("CFG " + candidate.Coverage + " [" + loc + "]"))
		}
		fmt.Fprintf(&out, "UNKNOWN_PREVIOUS -[dashed]-> %s : %s\n", aliases[bound.State.Name], label.String())
	}
	out.WriteString("\nlegend bottom\nExplicit configured machine; NOT a reconstructed runtime state machine\n")
	fmt.Fprintf(&out, "Target: %s.%s\nBackend: %s; language: %s\nDefinition: %s; SHA256 %s\n", packageText(view.Analysis.Target.Qualified), packageText(view.Analysis.Target.Field), packageText(view.Analysis.Backend), packageText(view.Analysis.Language), packageText(definition.Source), packageText(definition.Hash))
	out.WriteString("Solid edges and initial marker are configured declarations, not source-verified behavior\nDashed edges are source assignment candidates from UNKNOWN previous state\nGuards/actions are display text; no code or configuration expressions are executed\nCFG connectivity is syntactic coverage, not occurrence, reachability or same-instance proof\nIndirect/alias/concurrent updates and implicit initialization can remain unknown\n")
	for _, candidate := range view.Candidates {
		if candidate.Destination != "" {
			continue
		}
		u := candidate.Update
		fmt.Fprintf(&out, "Unmapped candidate (%s, CFG %s): %s [%s]\n", packageText(u.Kind), packageText(candidate.Coverage), packageText(u.Expression), packageText(sequenceLocation(u.File, u.Span.Start.Line, u.Span.Start.Column, paths)))
	}
	for _, d := range view.Analysis.Diagnostics {
		fmt.Fprintf(&out, "Diagnostic %s: %s [%s]\n", packageText(d.Code), packageText(d.Message), packageText(sequenceLocation(d.File, d.Span.Start.Line, d.Span.Start.Column, paths)))
	}
	out.WriteString("endlegend\n@enduml\n")
	return out.String(), nil
}
