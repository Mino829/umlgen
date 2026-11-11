package relations

import (
	"reflect"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
)

func TestProjectionRetainsResolvedContextWithoutRebinding(t *testing.T) {
	owner := model.Type{
		Package: "sample", Enclosing: []string{"Outer"}, Name: "Service",
		Imports: []model.Import{{Name: "fallback.Target"}},
		Fields:  []model.Field{{Name: "target", Type: "Target", Source: "Service.java", Line: 3}},
	}
	nested := model.Type{Package: "sample", Enclosing: []string{"Outer"}, Name: "Target"}
	fallback := model.Type{Package: "fallback", Name: "Target"}
	types := []model.Type{owner, nested, fallback}
	for _, details := range []bool{false, true} {
		analysis := Analyze(Input{Types: types, Known: types}, details)
		if len(analysis.Relations) != 1 || analysis.Relations[0].To != nested.QualifiedName() {
			t.Fatalf("initial resolution = %#v", analysis.Relations)
		}
		selected := analysis.Select([]model.Type{owner, fallback})
		if len(selected.Relations) != 0 {
			t.Fatalf("hiding the nested target rebound the reference: %#v", selected.Relations)
		}
		if !details {
			if selected.Diagnostics != nil || selected.references != nil || analysis.Relations[0].Evidence != nil {
				t.Fatalf("lightweight analysis captured details: %#v", selected)
			}
			continue
		}
		if len(selected.Diagnostics) != 1 {
			t.Fatalf("hidden target diagnostics = %#v", selected.Diagnostics)
		}
		diagnostic := selected.Diagnostics[0]
		if diagnostic.Status != StatusOutsideDiagram || diagnostic.Target != nested.QualifiedName() || diagnostic.Reference != "Target" || diagnostic.Evidence.Member != "target" {
			t.Fatalf("lost original binding: %#v", diagnostic)
		}
		if len(analysis.Relations) != 1 || len(analysis.Diagnostics) != 0 {
			t.Fatal("projection mutated its source graph")
		}
		if again := selected.Select([]model.Type{owner}); !reflect.DeepEqual(again.Diagnostics, selected.Diagnostics) {
			t.Fatalf("repeated projection lost hidden context: %#v", again)
		}
	}
}

func TestProjectionKeepsExactReferenceEvidenceAndUncertainty(t *testing.T) {
	owner := model.Type{
		Package: "sample", Name: "Service", Source: "Service.java", Kind: model.Class,
		Fields: []model.Field{{Name: "values", Type: "Map<Left, Right, Left>", Source: "Service.java", Line: 4}},
	}
	left := model.Type{Package: "sample", Name: "Left"}
	right := model.Type{Package: "sample", Name: "Right"}
	types := []model.Type{owner, left, right}
	analysis := Analyze(Input{Types: types}, true)
	selected := analysis.Select([]model.Type{owner, right})
	if len(selected.Relations) != 1 || selected.Relations[0].To != right.QualifiedName() {
		t.Fatalf("visible graph = %#v", selected.Relations)
	}
	if len(selected.Diagnostics) != 2 {
		t.Fatalf("duplicate or missing diagnostics = %#v", selected.Diagnostics)
	}
	for _, diagnostic := range selected.Diagnostics {
		switch diagnostic.Reference {
		case "Left":
			if diagnostic.Status != StatusOutsideDiagram || diagnostic.Target != left.QualifiedName() {
				t.Fatalf("hidden target became unknown: %#v", diagnostic)
			}
		case "Map":
			if diagnostic.Status != StatusUnresolved || diagnostic.Target != "" {
				t.Fatalf("unknown type gained a binding: %#v", diagnostic)
			}
		default:
			t.Fatalf("incorrect reference captured from generic spelling: %#v", diagnostic)
		}
		if diagnostic.Evidence.Type != "Map<Left, Right, Left>" || diagnostic.Evidence.Line != 4 || diagnostic.Evidence.Language != "java" {
			t.Fatalf("lost declaration evidence: %#v", diagnostic)
		}
	}
	// An equivalent initial selection has the same public graph and diagnostics.
	direct := Analyze(Input{Types: []model.Type{owner, right}, Known: types}, true)
	if !reflect.DeepEqual(selected.Relations, direct.Relations) || !reflect.DeepEqual(selected.Diagnostics, direct.Diagnostics) {
		t.Fatalf("projection differs from equivalent known-context selection:\nprojected: %#v\ndirect: %#v", selected, direct)
	}
}

func TestProjectionDoesNotTurnImplementationMethodsIntoHiddenReferences(t *testing.T) {
	owner := model.Type{
		Package: "sample", Name: "Worker", Kind: model.Struct, Source: "worker.go",
		Implements:            []model.TypeReference{{Name: "Runner", Source: "worker.go", Line: 2, Inferred: true}},
		ImplementationMethods: []model.ImplementationMethod{{Interface: "Runner", Signature: "Run()", Source: "methods.go", Line: 7}},
	}
	contract := model.Type{Package: "sample", Name: "Runner", Kind: model.Interface}
	analysis := Analyze(Input{Types: []model.Type{owner, contract}}, true)
	if len(analysis.Relations) != 1 || len(analysis.Relations[0].Evidence) != 2 {
		t.Fatalf("implementation evidence = %#v", analysis.Relations)
	}
	selected := analysis.Select([]model.Type{owner})
	if len(selected.Diagnostics) != 1 || !selected.Diagnostics[0].Evidence.Inferred || selected.Diagnostics[0].Evidence.Member != "" {
		t.Fatalf("implementation witness became an extra reference: %#v", selected.Diagnostics)
	}
}

func TestDetailedAnalysisDoesNotChangeGraphOrOrder(t *testing.T) {
	types := []model.Type{
		{Package: "sample", Name: "Service", Fields: []model.Field{{Name: "b", Type: "B"}, {Name: "a", Type: "A"}}},
		{Package: "sample", Name: "B"},
		{Package: "sample", Name: "A"},
	}
	plain := Analyze(Input{Types: types}, false)
	detailed := Analyze(Input{Types: types}, true)
	reversed := []model.Type{types[2], types[1], types[0]}
	reordered := Analyze(Input{Types: reversed}, true)
	if !reflect.DeepEqual(detailed.Relations, reordered.Relations) || !reflect.DeepEqual(detailed.Diagnostics, reordered.Diagnostics) {
		t.Fatal("analysis depends on declaration enumeration order")
	}
	if len(plain.Relations) != len(detailed.Relations) || plain.Diagnostics != nil || plain.references != nil {
		t.Fatalf("lightweight analysis = %#v", plain)
	}
	for i, relation := range plain.Relations {
		if relation.Evidence != nil || relationKey(relation) != relationKey(detailed.Relations[i]) {
			t.Fatalf("details changed graph identity: %#v versus %#v", relation, detailed.Relations[i])
		}
	}
}
