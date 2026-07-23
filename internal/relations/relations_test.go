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
		"User":           "1",
		"Optional<User>": "0..1",
		"List<User>":     "*",
		"User[]":         "*",
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
		found, parseErr := java.ParseFile(file)
		if parseErr != nil {
			t.Fatalf("ParseFile(%s): %v", file, parseErr)
		}
		types = append(types, found...)
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
