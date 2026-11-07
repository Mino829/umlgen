package state

import (
	"fmt"

	"github.com/Mino829/umlgen/internal/activity"
)

type BoundState struct {
	State State
	Value *Value
}

type Candidate struct {
	Update      Update
	Destination string
	Coverage    string
}

type View struct {
	Definition Definition
	Analysis   Analysis
	States     []BoundState
	Candidates []Candidate
	MaxNodes   int
}

// Bind keeps configured transitions and compiler-backed assignment candidates
// separate. Even a constant assignment inside a matching guard does not prove
// its previous state, event, instance identity, or a configured transition.
func Bind(def Definition, analysis Analysis, maxNodes int) (View, error) {
	view := View{Definition: def, Analysis: analysis, MaxNodes: maxNodes}
	if err := def.Validate(); err != nil {
		return view, err
	}
	if analysis.Target.ID == "" || analysis.Target.Field == "" {
		return view, fmt.Errorf("%w: explicit type and field must resolve", ErrSelection)
	}
	keys := make(map[string]string, len(def.States))
	for _, s := range def.States {
		bound := BoundState{State: s}
		if s.Value != "" {
			value, err := selectValue(analysis.Values, s.Value)
			if err != nil {
				return view, fmt.Errorf("state %q: %w", s.Name, err)
			}
			if prior, found := keys[value.Key]; found {
				return view, fmt.Errorf("states %q and %q bind the same source value", prior, s.Name)
			}
			keys[value.Key] = s.Name
			bound.Value = value
		}
		view.States = append(view.States, bound)
	}
	type function struct{ reachable map[int]bool }
	functions := make(map[string]function, len(analysis.Functions))
	for i := range analysis.Functions {
		g := &analysis.Functions[i]
		if err := g.Validate(maxNodes); err != nil {
			return view, err
		}
		if g.Function.ID == "" {
			return view, fmt.Errorf("state evidence CFG is missing its callable identity")
		}
		if _, exists := functions[g.Function.ID]; exists {
			return view, fmt.Errorf("duplicate state evidence CFG %q", g.Function.ID)
		}
		functions[g.Function.ID] = function{reachable: stateReachable(g)}
	}
	for _, u := range analysis.Updates {
		candidate := Candidate{Update: u, Destination: keys[u.ValueKey], Coverage: "outside-function"}
		if u.FunctionID != "" {
			candidate.Coverage = "outside-cfg"
			if f, exists := functions[u.FunctionID]; exists {
				candidate.Coverage = "unmapped-location"
				if len(u.NodeIDs) > 0 {
					candidate.Coverage = "not-connected"
					for _, id := range u.NodeIDs {
						connected, exists := f.reachable[id]
						if !exists {
							return view, fmt.Errorf("state update references missing CFG node %d in %q", id, u.FunctionID)
						}
						if connected {
							candidate.Coverage = "connected"
						}
					}
				}
			}
		}
		view.Candidates = append(view.Candidates, candidate)
	}
	return view, nil
}

func selectValue(values []Value, selector string) (*Value, error) {
	for tier := 0; tier < 4; tier++ {
		var found *Value
		for i := range values {
			v := &values[i]
			if v.Key == "" {
				continue
			}
			if (tier == 0 && v.ID == selector) || (tier == 1 && v.Qualified == selector) || (tier == 2 && v.Name == selector) || (tier == 3 && v.Literal == selector) {
				if found != nil && found.Key != v.Key {
					return nil, fmt.Errorf("ambiguous source value %q; use a qualified/canonical constant selector", selector)
				}
				found = v
			}
		}
		if found != nil {
			return found, nil
		}
	}
	return nil, fmt.Errorf("source value %q was not resolved; use a selected-source constant/enum member or observed literal, or omit value to leave a configured-only state", selector)
}

func stateReachable(g *activity.Graph) map[int]bool {
	reachable := make(map[int]bool, len(g.Nodes))
	adjacent := make(map[int][]int, len(g.Nodes))
	for _, n := range g.Nodes {
		reachable[n.ID] = false
	}
	for _, edge := range g.Edges {
		adjacent[edge.From] = append(adjacent[edge.From], edge.To)
	}
	queue := make([]int, 1, len(g.Nodes))
	queue[0] = g.Entry
	for i := 0; i < len(queue); i++ {
		id := queue[i]
		if reachable[id] {
			continue
		}
		reachable[id] = true
		for _, next := range adjacent[id] {
			if !reachable[next] {
				queue = append(queue, next)
			}
		}
	}
	return reachable
}
