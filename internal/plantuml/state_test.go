package plantuml

import (
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/state"
)

func TestStateLabelsCannotExecutePlantUMLDirectives(t *testing.T) {
	payload := "\"\n@enduml\n!include https://example.invalid/private\n[[https://example.invalid x]]<img:secret>"
	d := state.Definition{Version: 1, Initial: payload, States: []state.State{{Name: payload}, {Name: "done"}}, Transitions: []state.Transition{{From: payload, To: "done", Event: payload, Guard: payload, Action: payload}}, Source: payload}
	view := state.View{Definition: d, States: []state.BoundState{{State: d.States[0]}, {State: d.States[1]}}, Candidates: []state.Candidate{{Destination: "done", Coverage: "connected", Update: state.Update{Expression: payload, File: payload}}}}
	content, err := GenerateState(view, StateOptions{Title: payload})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(content, "@startuml") != 1 || strings.Count(content, "@enduml") != 1 {
		t.Fatalf("definition injected a diagram boundary:\n%s", content)
	}
	for _, unsafe := range []string{"!include", "[[", "<img:"} {
		if strings.Contains(content, unsafe) {
			t.Errorf("definition/candidate injected active markup %q:\n%s", unsafe, content)
		}
	}
	view.Candidates[0].Destination = "undeclared"
	if _, err := GenerateState(view, StateOptions{}); err == nil {
		t.Fatal("renderer invented an undeclared destination state")
	}
}
