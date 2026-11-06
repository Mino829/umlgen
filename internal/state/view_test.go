package state

import (
	"testing"

	"github.com/Mino829/umlgen/internal/activity"
	"github.com/Mino829/umlgen/internal/sequence"
)

func boundFixture() (Definition, Analysis) {
	d := Definition{Version: 1, Type: "Connection", Field: "phase", Initial: "open", States: []State{{Name: "closed", Value: "Closed"}, {Name: "open", Value: "Open"}}, Transitions: []Transition{{From: "closed", To: "open", Event: "Connect"}}}
	a := Analysis{Target: Target{ID: "root::Connection", Qualified: "Connection", Field: "phase"}, Values: []Value{{ID: "root::Closed", Name: "Closed", Qualified: "sample.Closed", Key: "0"}, {ID: "root::Open", Name: "Open", Qualified: "sample.Open", Key: "1"}}, Functions: []activity.Graph{{Function: sequence.Callable{ID: "update"}, Entry: 0, Exit: 1, Nodes: []activity.Node{{ID: 0, Kind: "entry"}, {ID: 1, Kind: "exit"}, {ID: 2, Kind: "unsupported"}, {ID: 3, Kind: "action"}}, Edges: []activity.Edge{{From: 0, To: 2, Kind: "normal"}, {From: 3, To: 1, Kind: "normal"}}}}, Updates: []Update{{Kind: "assignment", ValueKey: "1", FunctionID: "update", NodeIDs: []int{3}}, {Kind: "initializer", ValueKey: "0"}, {Kind: "assignment", Expression: "runtimeValue"}}}
	return d, a
}

func TestBindDoesNotInventPreviousStateOrRuntimeConnectivity(t *testing.T) {
	d, a := boundFixture()
	view, err := Bind(d, a, 20)
	if err != nil {
		t.Fatal(err)
	}
	if view.Candidates[0].Destination != "open" || view.Candidates[0].Coverage != "not-connected" {
		t.Fatalf("post-barrier write became connected/proven: %+v", view.Candidates[0])
	}
	if view.Candidates[1].Destination != "closed" || view.Candidates[1].Coverage != "outside-function" {
		t.Fatalf("initializer became a function transition: %+v", view.Candidates[1])
	}
	if view.Candidates[2].Destination != "" {
		t.Fatal("runtime RHS became a known destination")
	}
	if view.Definition.Initial != "open" || len(view.Definition.Transitions) != 1 {
		t.Fatal("source writes/zero initialization changed the explicitly configured machine")
	}
}

func TestBindRejectsAmbiguousAndContradictoryValueMappings(t *testing.T) {
	d, a := boundFixture()
	a.Values = append(a.Values, Value{Name: "Open", Qualified: "other.Open", Key: "2"})
	if _, err := Bind(d, a, 20); err == nil {
		t.Fatal("ambiguous simple constant was accepted")
	}
	d.States[1].Value = "sample.Open"
	if _, err := Bind(d, a, 20); err != nil {
		t.Fatalf("qualified constant did not disambiguate: %v", err)
	}
	d.States[0].Value = "sample.Open"
	if _, err := Bind(d, a, 20); err == nil {
		t.Fatal("two states bound to the same source value")
	}
	d.States[0].Value = "UnknownConstant"
	if _, err := Bind(d, a, 20); err == nil {
		t.Fatal("unresolved explicit value became a fake binding")
	}
	d.States[0].Value = ""
	if _, err := Bind(d, a, 20); err != nil {
		t.Fatalf("configured-only state rejected: %v", err)
	}
}

func TestBindRejectsForeignCFGNodeReferences(t *testing.T) {
	d, a := boundFixture()
	a.Updates[0].NodeIDs = []int{99}
	if _, err := Bind(d, a, 20); err == nil {
		t.Fatal("source evidence referenced a nonexistent CFG node")
	}
	if _, err := Bind(d, a, 2); err == nil {
		t.Fatal("CFG node limit bypassed")
	}
}
