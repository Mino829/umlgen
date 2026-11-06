package plantuml

import (
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/activity"
	"github.com/Mino829/umlgen/internal/sequence"
)

func TestActivityEscapesSourceAndTransferLabels(t *testing.T) {
	payload := "\"\n@enduml\n!include https://example.invalid/secret\n[[https://example.invalid x]]<img:secret>"
	g := activity.Graph{Entry: 0, Exit: 1, Function: sequence.Callable{Selector: payload}, Nodes: []activity.Node{{ID: 0, Kind: "entry", Label: payload}, {ID: 1, Kind: "exit"}, {ID: 2, Kind: "unsupported", Label: payload}}, Edges: []activity.Edge{{From: 0, To: 2, Kind: "true", Label: "] " + payload}}}
	diagram, err := GenerateActivity(g, ActivityOptions{Title: payload})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(diagram, "@startuml") != 1 || strings.Count(diagram, "@enduml") != 1 {
		t.Fatalf("source injected diagram boundaries:\n%s", diagram)
	}
	for _, unsafe := range []string{"!include", "[[", "<img:"} {
		if strings.Contains(diagram, unsafe) {
			t.Errorf("source/edge payload was interpreted as markup %q:\n%s", unsafe, diagram)
		}
	}
	g.Edges = append(g.Edges, activity.Edge{From: 2, To: 1, Kind: "normal"})
	if _, err := GenerateActivity(g, ActivityOptions{}); err == nil {
		t.Fatal("renderer drew a false normal continuation after an unsupported region")
	}
}
