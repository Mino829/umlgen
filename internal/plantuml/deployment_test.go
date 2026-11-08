package plantuml

import (
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/deployment"
)

func TestDeploymentPropertiesCannotActivatePlantUML(t *testing.T) {
	payload := "\"\n@enduml\n!include https://example.invalid/private\n[[https://example.invalid link]]<img:private>"
	app := deployment.ID{Kind: "compose-service", Name: "api"}
	image := deployment.ID{Kind: "image", Name: "image"}
	graph := deployment.Graph{Nodes: []deployment.Node{{ID: app, Name: payload, Ports: []deployment.Port{{Published: payload, Target: payload, Protocol: payload}}}, {ID: image, Name: payload}}, Edges: []deployment.Edge{{From: app, To: image, Kind: "uses-image", Detail: payload}}, Sources: []deployment.Origin{{File: payload, Hash: payload, Format: payload}}, Diagnostics: []deployment.Diagnostic{{Code: payload, Message: payload, Origin: deployment.Origin{File: payload}}}}
	output, err := GenerateDeployment(graph, DeploymentOptions{Title: payload})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(output, "@startuml") != 1 || strings.Count(output, "@enduml") != 1 {
		t.Fatal("properties introduced active diagram boundaries")
	}
	for _, active := range []string{"!include", "[[", "<img:"} {
		if strings.Contains(output, active) {
			t.Fatalf("active markup %q escaped property boundary", active)
		}
	}
}
