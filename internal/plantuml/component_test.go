package plantuml

import (
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/component"
	"github.com/Mino829/umlgen/internal/packagegraph"
)

func TestComponentLabelsCannotIntroduceActivePlantUML(t *testing.T) {
	payload := "\"\n@enduml\n!include https://example.invalid/secret\n[[https://example.invalid x]]<img:secret>"
	definition := component.Definition{Version: 1, Source: payload, Components: []component.Boundary{{Name: payload, Match: []component.Selector{{Language: "java", SourceRoot: "root"}}, Provides: []string{payload}, Requires: []string{payload}}}}
	source := packagegraph.Graph{Source: packagegraph.Both, Packages: []packagegraph.Package{{ID: packagegraph.PackageID{Language: "java", Context: "root", Name: "sample"}, Kind: packagegraph.InternalPackage}}, Diagnostics: []packagegraph.Diagnostic{{Code: payload, Reference: payload, File: packagegraph.File{Path: payload}}}}
	graph, err := component.Project(source, definition, nil, component.Options{})
	if err != nil {
		t.Fatal(err)
	}
	content, err := GenerateComponent(graph, ComponentOptions{Title: payload})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(content, "@startuml") != 1 || strings.Count(content, "@enduml") != 1 {
		t.Fatalf("source/config introduced active diagram boundaries:\n%s", content)
	}
	for _, unsafe := range []string{"!include", "[[", "<img:"} {
		if strings.Contains(content, unsafe) {
			t.Errorf("active markup %q in component labels:\n%s", unsafe, content)
		}
	}
}
