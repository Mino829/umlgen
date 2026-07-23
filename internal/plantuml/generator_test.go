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
			Fields:     []model.Field{{Name: "repository", Type: "Repository", Visibility: model.Private}},
			Methods: []model.Method{{
				Name: "find", ReturnType: "User", Visibility: model.Public,
				Parameters: []model.Parameter{{Name: "id", Type: "long"}},
			}},
		},
		{Package: "sample", Name: "UseCase", Kind: model.Interface},
		{Package: "sample", Name: "User", Kind: model.Class},
		{Package: "sample", Name: "UserId", Kind: model.Record},
	}}
	got := Generate(project, Options{
		ShowFields: true, ShowMethods: true, ShowPrivate: true, ShowPublic: true,
		ShowProtected: true, ShowPackage: true, ShowRelations: true,
		Inheritance: true, Implementation: true, FieldDependency: true,
		ParamDependency: true, ReturnDependency: true,
	})
	for _, want := range []string{
		`class "Service"`, `-repository: Repository`, `+find(id: long): User`,
		`T_sample_UseCase <|.. T_sample_Service`,
		`T_sample_Service --> T_sample_Repository`,
		`T_sample_Service --> T_sample_User`,
		`class "UserId" as T_sample_UserId <<record>>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output does not contain %q:\n%s", want, got)
		}
	}
}
