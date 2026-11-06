package activity

import (
	"errors"
	"testing"
)

func TestValidateGraphLimitsAndTerminalBarriers(t *testing.T) {
	valid := func() Graph {
		return Graph{Entry: 0, Exit: 1, Nodes: []Node{{ID: 0, Kind: "entry"}, {ID: 1, Kind: "exit"}, {ID: 2, Kind: "unsupported"}}, Edges: []Edge{{From: 0, To: 2, Kind: "normal"}}}
	}
	g := valid()
	if err := g.Validate(2); !errors.Is(err, ErrLimit) {
		t.Fatalf("node cap should reject rather than truncate: %v", err)
	}
	if err := g.Validate(3); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		alter func(*Graph)
	}{
		{"unknown continuation", func(g *Graph) { g.Edges = append(g.Edges, Edge{From: 2, To: 1, Kind: "normal"}) }},
		{"exit continuation", func(g *Graph) { g.Edges = append(g.Edges, Edge{From: 1, To: 0, Kind: "back"}) }},
		{"missing destination", func(g *Graph) { g.Edges[0].To = 99 }},
		{"duplicate identity", func(g *Graph) { g.Nodes[2].ID = 0 }},
		{"invalid identity", func(g *Graph) { g.Nodes[2].ID = -1 }},
		{"unsupported node kind", func(g *Graph) { g.Nodes[2].Kind = "future" }},
		{"unsupported transfer kind", func(g *Graph) { g.Edges[0].Kind = "future" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := valid()
			tt.alter(&g)
			if err := g.Validate(3); err == nil {
				t.Fatal("unsafe CFG accepted")
			}
		})
	}
}
