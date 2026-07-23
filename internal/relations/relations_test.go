package relations

import (
	"testing"

	"github.com/Mino829/umlgen/internal/model"
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
