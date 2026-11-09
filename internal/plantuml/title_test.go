package plantuml

import (
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
)

func TestClassAndPlaceholderTitlesCannotIntroducePlantUMLSyntax(t *testing.T) {
	payload := "review\r\n@enduml\n!include local.puml\n@startuml\n} [[link]] <img:local.png> %date()"
	for _, diagram := range []string{
		Generate(model.Project{Types: []model.Type{{Name: "Visible", Kind: model.Class}}}, Options{Title: payload}),
		Placeholder(payload, Options{}),
	} {
		if strings.Count(diagram, "@startuml") != 1 || strings.Count(diagram, "@enduml") != 1 {
			t.Fatalf("title changed diagram boundaries:\n%s", diagram)
		}
		for _, syntax := range []string{"!include", "[[link]]", "<img:", "%date()"} {
			if strings.Contains(diagram, syntax) {
				t.Errorf("title retained executable/markup syntax %q:\n%s", syntax, diagram)
			}
		}
	}
}
