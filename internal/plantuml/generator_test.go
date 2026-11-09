package plantuml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
)

func TestGenerate(t *testing.T) {
	project := model.Project{Types: []model.Type{
		{Package: "sample", Name: "Repository", Kind: model.Interface},
		{
			Package: "sample", Name: "Service", Kind: model.Class,
			Implements: []model.TypeReference{{Name: "UseCase"}},
			Fields:     []model.Field{{Name: "repositories", Type: "List<Repository>", Visibility: model.Private}},
			Methods: []model.Method{{
				Name: "find", ReturnType: "User", Visibility: model.Public,
				Parameters: []model.Parameter{{Name: "id", Type: "long"}},
			}},
		},
		{Package: "sample", Name: "UseCase", Kind: model.Interface},
		{Package: "sample", Name: "User", Kind: model.Class},
		{Package: "sample", Name: "GoUser", Kind: model.Struct},
		{Package: "sample", Name: "UserId", Kind: model.Record, Change: model.Added},
	}}
	got := Generate(project, Options{
		ShowFields: true, ShowMethods: true, ShowPrivate: true, ShowPublic: true,
		ShowProtected: true, ShowPackage: true, ShowRelations: true,
		Inheritance: true, Implementation: true, FieldDependency: true,
		ParamDependency: true, ReturnDependency: true, ShowRelationLabels: true,
	})
	for _, want := range []string{
		`class "Service"`, `-repositories: List<Repository>`, `+find(id: long): User`,
		`T_sample_UseCase <|.. T_sample_Service`,
		`T_sample_Service --> "*" T_sample_Repository : field repositories`,
		`T_sample_Service ..> "1" T_sample_User : returns find`,
		`class "UserId" as T_sample_UserId <<record>> #palegreen`,
		`class "GoUser" as T_sample_GoUser <<struct>>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not contain %q:\n%s", want, got)
		}
	}
}

func TestGenerateCommonJavaModifiersAndMembers(t *testing.T) {
	project := model.Project{Types: []model.Type{
		{
			Package: "sample", Name: "Base", Kind: model.Class, Abstract: true, Sealed: true,
			TypeParameters: "<T extends Entity>",
			Fields:         []model.Field{{Name: "KIND", Type: "String", Visibility: model.Public, Static: true}},
			Methods:        []model.Method{{Name: "load", ReturnType: "T", Visibility: model.Public, Abstract: true}},
		},
		{
			Package: "sample", Name: "Status", Kind: model.Enum,
			EnumValues: []string{"ACTIVE", "INACTIVE"},
		},
		{
			Package: "sample", Name: "Configuration", Kind: model.Annotation,
			Methods: []model.Method{{Name: "value", ReturnType: "String", Visibility: model.Public, Abstract: true}},
		},
	}}
	got := Generate(project, Options{
		ShowFields: true, ShowMethods: true, ShowPrivate: true, ShowPublic: true,
		ShowProtected: true, ShowPackage: true,
	})
	for _, want := range []string{
		`abstract class "Base<T extends Entity>" as T_sample_Base <<sealed>>`,
		`{static} +KIND: String`,
		`{abstract} +load(): T`,
		`enum "Status"`,
		`ACTIVE`,
		`INACTIVE`,
		`annotation "Configuration"`,
		`+value(): String`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not contain %q:\n%s", want, got)
		}
	}
}

func TestGenerateUsesDistinctAliasesForCollidingNames(t *testing.T) {
	types := []model.Type{
		{Package: "a_b.c", Name: "Widget", Kind: model.Class},
		{Package: "a.b_c", Name: "Widget", Kind: model.Class},
		{Package: "a.b_c", Name: "Widget_2", Kind: model.Class},
		{
			Package: "app", Name: "Consumer", Kind: model.Class,
			Fields: []model.Field{{Name: "widget", Type: "a_b.c.Widget", Visibility: model.Private}},
		},
	}
	aliases := aliasesFor(types)
	seen := map[string]bool{}
	for _, typ := range types {
		alias := aliases[typ.QualifiedName()]
		if seen[alias] {
			t.Fatalf("duplicate alias %q: %#v", alias, aliases)
		}
		seen[alias] = true
	}
	if aliases["a.b_c.Widget_2"] != "T_a_b_c_Widget_2" {
		t.Fatalf("an existing base alias was reused: %#v", aliases)
	}

	for _, grouping := range []string{"flat", "hierarchical", "none"} {
		t.Run(grouping, func(t *testing.T) {
			got := Generate(model.Project{Types: types}, Options{
				PackageGrouping: grouping,
				ShowRelations:   true, FieldDependency: true, ShowRelationLabels: true,
			})
			for _, typ := range types {
				declaration := `as ` + aliases[typ.QualifiedName()]
				if !strings.Contains(got, declaration) {
					t.Errorf("missing declaration %q:\n%s", declaration, got)
				}
			}
			want := `T_app_Consumer --> "1" ` + aliases["a_b.c.Widget"] + ` : field widget`
			if !strings.Contains(got, want) {
				t.Errorf("missing relation %q:\n%s", want, got)
			}
		})
	}
}

func TestGeneratePackageLayoutGolden(t *testing.T) {
	java := model.Project{Types: []model.Type{
		{Package: "org.example.api", Name: "Service", Kind: model.Class, Change: model.Modified},
		{Package: "com.acme.store", Name: "Repository", Kind: model.Interface, Change: model.Deleted},
		{
			Package: "com.acme.api", Name: "Service", Kind: model.Class, Change: model.Added,
			Implements: []model.TypeReference{{Name: "UseCase"}},
			Fields: []model.Field{{
				Name: "repository", Type: "com.acme.store.Repository", Visibility: model.Private,
			}},
		},
		{Name: "Bootstrap", Kind: model.Class},
		{Package: "com.acme.api", Name: "UseCase", Kind: model.Interface},
	}}
	goProject := model.Project{Types: []model.Type{
		{Package: "example.com", Name: "Root", Kind: model.Struct, Source: "root.go"},
		{Package: "example.com/other/api", Name: "Service", Kind: model.Struct},
		{Package: "example.com/acme/store", Name: "Repository", Kind: model.Struct, Change: model.Deleted},
		{
			Package: "example.com/acme/api", Name: "Service", Kind: model.Struct, Change: model.Added,
			Imports: []model.Import{{Name: "example.com/acme/store"}},
			Fields:  []model.Field{{Name: "repository", Type: "*store.Repository", Visibility: model.Private}},
		},
	}}
	base := Options{
		Title: "Release #13", ShowFields: true, ShowPrivate: true, ShowRelations: true,
		Implementation: true, FieldDependency: true, ShowRelationLabels: true,
	}
	hierarchical := base
	hierarchical.PackageGrouping = "hierarchical"
	hierarchical.LayoutDirection = "left-to-right"
	hierarchical.SkinParams = map[string]string{
		"shadowing": "false", "linetype": "ortho", "classAttributeIconSize": "0",
	}
	goOptions := base
	goOptions.Title = ""
	goOptions.PackageGrouping = "hierarchical"
	goOptions.LayoutDirection = "top-to-bottom"
	none := base
	none.PackageGrouping = "none"
	for _, tc := range []struct {
		name    string
		fixture string
		project model.Project
		options Options
	}{
		{"java_hierarchical", "java_hierarchical.puml", java, hierarchical},
		{"go_hierarchical", "go_hierarchical.puml", goProject, goOptions},
		{"none", "java_none.puml", java, none},
		{"default", "java_flat.puml", java, base},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.options.ValidateLayout(); err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", tc.fixture))
			if err != nil {
				t.Fatal(err)
			}
			got := Generate(tc.project, tc.options)
			if got != string(want) {
				t.Fatalf("output differs from %s:\nwant:\n%s\ngot:\n%s", tc.fixture, want, got)
			}
			reversed := append([]model.Type(nil), tc.project.Types...)
			for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
				reversed[i], reversed[j] = reversed[j], reversed[i]
			}
			if reordered := Generate(model.Project{Types: reversed}, tc.options); reordered != got {
				t.Fatalf("output depends on input order:\n%s", reordered)
			}
		})
	}
}

func TestValidateLayout(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options Options
	}{
		{"unknown_grouping", Options{PackageGrouping: "nested"}},
		{"grouping_case", Options{PackageGrouping: "Flat"}},
		{"grouping_directive", Options{PackageGrouping: "flat\n!include secret"}},
		{"unknown_direction", Options{LayoutDirection: "right-to-left"}},
		{"direction_whitespace", Options{LayoutDirection: " left-to-right"}},
		{"direction_directive", Options{LayoutDirection: "left-to-right\n@enduml"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.options.ValidateLayout(); err == nil {
				t.Fatal("expected an invalid layout error")
			}
		})
	}
	for key, values := range map[string][]string{
		"linetype":               {"", "ORTHO", "spline", "ortho ", "ortho\n!include secret", "ortho\r@enduml", "ortho; shadowing true"},
		"shadowing":              {"", "True", "1", "false\n@enduml", "false\x00", "false\u2028!include secret"},
		"classAttributeIconSize": {"", "-1", "+1", "1.0", "2147483648", "１２", "0\n!include secret", "0}"},
		"backgroundColor":        {"white"},
		"defaultFontName":        {"file:///secret"},
		"LineType":               {"ortho"},
		"linetype\n!include":     {"ortho"},
		"":                       {"ortho"},
	} {
		for _, value := range values {
			t.Run(key+"="+value, func(t *testing.T) {
				opts := Options{SkinParams: map[string]string{key: value}}
				if err := opts.ValidateLayout(); err == nil {
					t.Fatal("expected an invalid skinparam error")
				}
				// The string-only renderers must not expose raw options even
				// when called without the CLI's validation.
				for _, got := range []string{Generate(model.Project{}, opts), Placeholder("", opts)} {
					if strings.Contains(got, "skinparam ") || strings.Contains(got, "!include") ||
						strings.Count(got, "@enduml") != 1 {
						t.Fatalf("unsafe setting reached output:\n%s", got)
					}
				}
			})
		}
	}
}

func TestPlaceholderLayout(t *testing.T) {
	opts := Options{
		LayoutDirection: "left-to-right",
		SkinParams: map[string]string{
			"shadowing": "false", "classAttributeIconSize": "0", "linetype": "polyline",
		},
	}
	got := Placeholder("Review #13\r\nsafe", opts)
	for _, want := range []string{"left to right direction\n",
		"skinparam classAttributeIconSize 0\n", "skinparam linetype polyline\n", "skinparam shadowing false\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing placeholder layout setting %q:\n%s", want, got)
		}
	}
}
