package relations

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/java"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/scanner"
)

func TestBuildResolvesExplicitAndWildcardImports(t *testing.T) {
	types := []model.Type{
		{Package: "one", Name: "User"},
		{Package: "two", Name: "User"},
		{
			Package: "app", Name: "Service",
			Imports: []model.Import{{Name: "two.User"}, {Name: "one", Wildcard: true}},
			Fields:  []model.Field{{Name: "users", Type: "List<User>"}},
		},
	}
	got := Build(types)
	if len(got) != 1 {
		t.Fatalf("relations = %#v", got)
	}
	if got[0].From != "app.Service" || got[0].To != "two.User" ||
		got[0].Kind != Field || got[0].Multiplicity != "*" {
		t.Fatalf("relation = %#v", got[0])
	}
}

func TestBuildResolvesGoImportAliases(t *testing.T) {
	types := []model.Type{
		{Package: "example.com/app/domain", Name: "User", Kind: model.Struct},
		{
			Package: "example.com/app/service", Name: "Service", Kind: model.Struct,
			Imports: []model.Import{{Name: "example.com/app/domain", Alias: "entities"}},
			Fields:  []model.Field{{Name: "user", Type: "*entities.User"}},
		},
	}
	got := Build(types)
	if len(got) != 1 || got[0].To != "example.com/app/domain.User" {
		t.Fatalf("relations = %#v", got)
	}
}

func TestResolveDoesNotUseImportedTypeForDifferentQualifiedName(t *testing.T) {
	types := []model.Type{
		{Package: "com.acme", Name: "Bar"},
		{
			Package: "app", Name: "Service",
			Imports: []model.Import{{Name: "com.acme.Bar"}},
			Fields:  []model.Field{{Name: "external", Type: "other.Bar"}},
		},
	}
	if got := Build(types); len(got) != 0 {
		t.Fatalf("unresolved qualified type gained a relation: %#v", got)
	}
}

func TestResolveDoesNotUseUnrelatedUniqueType(t *testing.T) {
	types := []model.Type{
		{Package: "unrelated", Name: "Bar"},
		{Package: "app", Name: "Service", Fields: []model.Field{{Name: "external", Type: "Bar"}}},
	}
	if got := Build(types); len(got) != 0 {
		t.Fatalf("unimported type gained a relation: %#v", got)
	}
}

func TestResolveNestedTypeThroughExplicitImport(t *testing.T) {
	types := []model.Type{
		{Package: "com.acme", Name: "Outer"},
		{Package: "com.acme", Enclosing: []string{"Outer"}, Name: "Inner"},
		{
			Package: "app", Name: "Service",
			Imports: []model.Import{{Name: "com.acme.Outer"}},
			Fields:  []model.Field{{Name: "nested", Type: "Outer.Inner"}},
		},
	}
	got := Build(types)
	if len(got) != 1 || got[0].To != "com.acme.Outer.Inner" {
		t.Fatalf("relations = %#v", got)
	}
}

func TestResolveNestedType(t *testing.T) {
	types := []model.Type{
		{Package: "app", Name: "Outer"},
		{Package: "app", Enclosing: []string{"Outer"}, Name: "Inner"},
		{
			Package: "app", Enclosing: []string{"Outer"}, Name: "Worker",
			Fields: []model.Field{{Name: "inner", Type: "Inner"}},
		},
	}
	got := Build(types)
	if len(got) != 1 || got[0].To != "app.Outer.Inner" {
		t.Fatalf("relations = %#v", got)
	}
}

func TestMultiplicity(t *testing.T) {
	cases := map[string]string{
		"User":            "1",
		"Optional<User>":  "0..1",
		"List<User>":      "*",
		"User[]":          "*",
		"[]User":          "*",
		"map[string]User": "*",
	}
	for input, want := range cases {
		if got := Multiplicity(input); got != want {
			t.Errorf("Multiplicity(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildCompatibilityFixtureRelations(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "java", "compatibility", "project")
	files, err := scanner.JavaFiles([]string{root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var types []model.Type
	for _, file := range files {
		unit, parseErr := java.ParseFile(file)
		if parseErr != nil {
			t.Fatalf("ParseFile(%s): %v", file, parseErr)
		}
		types = append(types, unit.Types...)
	}

	got := Build(types)
	if len(got) != 11 {
		t.Fatalf("relations = %d, want 11: %#v", len(got), got)
	}
	for _, relation := range got {
		if strings.Contains(relation.To, "ExternalAuditClient") {
			t.Fatalf("unresolved external type became a relation: %#v", relation)
		}
	}

	wants := []Relation{
		{
			From: "com.acme.app.CreateUserCommand", To: "com.acme.app.Command",
			Kind: Implementation,
		},
		{
			From: "com.acme.app.UserService", To: "com.acme.support.User",
			Kind: Field, Label: "owner", Multiplicity: "1",
		},
		{
			From: "com.acme.app.UserService", To: "com.acme.sales.User",
			Kind: Field, Label: "salesUser", Multiplicity: "1",
		},
		{
			From: "com.acme.app.UserService", To: "com.acme.sales.Order.LineItem",
			Kind: Field, Label: "lineItems", Multiplicity: "*",
		},
		{
			From: "com.acme.app.UserService", To: "com.acme.sales.User",
			Kind: Return, Label: "findAll", Multiplicity: "*",
		},
		{
			From: "com.acme.sales.Order.LineItem", To: "com.acme.sales.User",
			Kind: Field, Label: "product", Multiplicity: "1",
		},
		{
			From: "com.acme.sales.User", To: "com.acme.shared.Identifiable",
			Kind: Implementation,
		},
	}
	index := make(map[string]Relation, len(got))
	for _, relation := range got {
		index[relationKey(relation)] = relation
	}
	for _, want := range wants {
		if _, ok := index[relationKey(want)]; !ok {
			t.Errorf("missing relation %#v in %#v", want, got)
		}
	}
}

func TestEvidencePreservesOverloadsAndDeduplicatesGenericReferences(t *testing.T) {
	const path = "Service.java"
	unit, err := java.ParseSource(path, []byte(`package sample;
class Target {}
class Service {
    Target load(Target input) { return null; }
    Target load(Target input, int limit) { return null; }
    java.util.Map<Target, Target> values;
}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[Kind][]Evidence{
		Field: {{Member: "values", Type: "java.util.Map<Target, Target>", Source: path, Line: 6}},
		Parameter: {
			{Member: "input of load(Target)", Type: "Target", Source: path, Line: 4},
			{Member: "input of load(Target, int)", Type: "Target", Source: path, Line: 5},
		},
		Return: {
			{Member: "load(Target)", Type: "Target", Source: path, Line: 4},
			{Member: "load(Target, int)", Type: "Target", Source: path, Line: 5},
		},
	}
	got := Analyze(unit.Types, nil).Relations
	if len(got) != len(want) {
		t.Fatalf("relations = %#v", got)
	}
	for _, relation := range got {
		if relation.From != "sample.Service" || relation.To != "sample.Target" {
			t.Fatalf("unexpected resolution: %#v", relation)
		}
		evidence := want[relation.Kind]
		if len(relation.Evidence) != len(evidence) {
			t.Fatalf("%s evidence = %#v, want %#v", relation.Kind, relation.Evidence, evidence)
		}
		for i := range evidence {
			evidence[i].Language = "java"
			evidence[i].OwnerKind = model.Class
			if relation.Evidence[i] != evidence[i] {
				t.Errorf("%s evidence %d = %#v, want %#v", relation.Kind, i, relation.Evidence[i], evidence[i])
			}
		}
	}
}

func TestAnalyzeDistinguishesHiddenDeclarationsAndUnresolvedReferences(t *testing.T) {
	unit, err := java.ParseSource("Service.java", []byte(`package sample;
class Target {}
class Service {
    Target hidden;
    Missing missing;
    java.util.Map<Missing, Missing> repeated;
}`))
	if err != nil {
		t.Fatal(err)
	}
	analysis := Analyze(unit.Types[1:], unit.Types)
	if len(analysis.Relations) != 0 {
		t.Fatalf("hidden or unresolved targets gained edges: %#v", analysis.Relations)
	}
	want := map[string]ReferenceStatus{
		"hidden/Target":          StatusOutsideDiagram,
		"missing/Missing":        StatusUnresolved,
		"repeated/java.util.Map": StatusUnresolved,
		"repeated/Missing":       StatusUnresolved,
	}
	if len(analysis.Diagnostics) != len(want) {
		t.Fatalf("diagnostics = %#v", analysis.Diagnostics)
	}
	for _, diagnostic := range analysis.Diagnostics {
		key := diagnostic.Evidence.Member + "/" + diagnostic.Reference
		status, ok := want[key]
		if !ok || diagnostic.Status != status || diagnostic.From != "sample.Service" || diagnostic.Kind != Field {
			t.Errorf("unexpected diagnostic: %#v", diagnostic)
		}
		if diagnostic.Status == StatusOutsideDiagram && diagnostic.Target != "sample.Target" {
			t.Errorf("wrong known declaration: %#v", diagnostic)
		}
		if diagnostic.Evidence.Language != "java" || diagnostic.Evidence.OwnerKind != model.Class ||
			diagnostic.Evidence.Source != "Service.java" || diagnostic.Evidence.Line < 4 {
			t.Errorf("missing declaration provenance: %#v", diagnostic)
		}
	}
}
