package plantuml

import (
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
)

func TestGenerate(t *testing.T) {
	project := model.Project{Types: []model.Type{
		{Package: "sample", Name: "Repository", Kind: model.Interface},
		{
			Package: "sample", Name: "Service", Kind: model.Class,
			Implements: []string{"UseCase"},
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
