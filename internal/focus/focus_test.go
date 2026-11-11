package focus

import (
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

func TestApplyByDepth(t *testing.T) {
	types := []model.Type{
		{Package: "sample", Name: "Controller", Fields: []model.Field{{Type: "Service"}}},
		{Package: "sample", Name: "Service", Fields: []model.Field{{Type: "Repository"}}},
		{Package: "sample", Name: "Repository", Methods: []model.Method{{ReturnType: "User"}}},
		{Package: "sample", Name: "User"},
		{Package: "sample", Name: "Unrelated"},
	}
	analysis := relations.Analyze(relations.Input{Types: types}, false)
	got, err := Apply(analysis, "Service", 1, Both)
	if err != nil {
		t.Fatal(err)
	}
	if names(got.Project.Types) != "Controller,Service,Repository" {
		t.Fatalf("depth 1 = %s", names(got.Project.Types))
	}
	got, err = Apply(analysis, "sample.Service", 2, Both)
	if err != nil {
		t.Fatal(err)
	}
	if names(got.Project.Types) != "Controller,Service,Repository,User" {
		t.Fatalf("depth 2 = %s", names(got.Project.Types))
	}
}

func TestApplyReportsAmbiguousSimpleName(t *testing.T) {
	types := []model.Type{{Package: "one", Name: "User"}, {Package: "two", Name: "User"}}
	_, err := Apply(relations.Analyze(relations.Input{Types: types}, false), "User", 1, Both)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyDirection(t *testing.T) {
	types := []model.Type{
		{Package: "sample", Name: "Controller", Fields: []model.Field{{Type: "Service"}}},
		{Package: "sample", Name: "Service", Fields: []model.Field{{Type: "Repository"}}},
		{Package: "sample", Name: "Repository"},
	}
	analysis := relations.Analyze(relations.Input{Types: types}, false)
	incoming, err := Apply(analysis, "Service", 1, Incoming)
	if err != nil {
		t.Fatal(err)
	}
	if names(incoming.Project.Types) != "Controller,Service" {
		t.Fatalf("incoming = %s", names(incoming.Project.Types))
	}
	outgoing, err := Apply(analysis, "Service", 1, Outgoing)
	if err != nil {
		t.Fatal(err)
	}
	if names(outgoing.Project.Types) != "Service,Repository" {
		t.Fatalf("outgoing = %s", names(outgoing.Project.Types))
	}
}

func names(types []model.Type) string {
	var result []string
	for _, t := range types {
		result = append(result, t.Name)
	}
	return strings.Join(result, ",")
}

func TestApplyManyUsesSuppliedGraphAndPreservesHiddenContext(t *testing.T) {
	types := []model.Type{
		{Package: "sample", Name: "A"},
		{Package: "sample", Name: "B", Fields: []model.Field{{Name: "next", Type: "C"}}},
		{Package: "sample", Name: "C"},
	}
	analysis := relations.Analyze(relations.Input{Types: types}, true)
	// A->B is supplied by an upstream analysis, not inferable from A's members.
	analysis.Relations = append(analysis.Relations, relations.Relation{
		From: "sample.A", To: "sample.B", Kind: relations.Parameter,
	})
	selected, err := ApplyMany(analysis, []string{"A", "A"}, 1, Outgoing)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(selected.Project.Types); got != "A,B" {
		t.Fatalf("focus rebuilt or ignored the supplied parameter edge: %s", got)
	}
	if len(selected.Relations) != 1 || selected.Relations[0].Kind != relations.Parameter {
		t.Fatalf("selected edges = %#v", selected.Relations)
	}
	if len(selected.Diagnostics) != 1 || selected.Diagnostics[0].Target != "sample.C" ||
		selected.Diagnostics[0].Status != relations.StatusOutsideDiagram {
		t.Fatalf("focus lost known-hidden context: %#v", selected.Diagnostics)
	}
	if len(analysis.Relations) != 2 || len(analysis.Diagnostics) != 0 {
		t.Fatal("focus mutated upstream analysis")
	}
}
