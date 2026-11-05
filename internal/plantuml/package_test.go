package plantuml

import (
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/relations"
)

func diagramPackage(context, name string) packagegraph.Package {
	return packagegraph.Package{ID: packagegraph.PackageID{Language: "java", Context: context, Name: name}, Kind: packagegraph.InternalPackage}
}
func diagramView(t *testing.T, graph packagegraph.Graph, opts packagegraph.ViewOptions) packagegraph.View {
	t.Helper()
	view, err := packagegraph.Analyze(graph, opts)
	if err != nil {
		t.Fatal(err)
	}
	return view
}
func diagramSource(t *testing.T, view packagegraph.View, opts PackageOptions) string {
	t.Helper()
	text, err := GeneratePackage(view, opts)
	if err != nil {
		t.Fatal(err)
	}
	return text
}
func diagramRelation(inferred bool) relations.Relation {
	return relations.Relation{From: "a.A", To: "b.B", Kind: relations.Field, Evidence: []relations.Evidence{{Source: "a.java", Line: 2, Inferred: inferred}}}
}

func TestPackageDiagramSeparatesSameNameContextsAndEdgeEndpoints(t *testing.T) {
	left, right := diagramPackage("rootA", "same"), diagramPackage("rootB", "same")
	graph := packagegraph.Graph{Source: packagegraph.DeclaredTypes, Packages: []packagegraph.Package{right, left}, Dependencies: []packagegraph.Dependency{{From: right.ID, To: left.ID, TypeRelations: []relations.Relation{diagramRelation(false)}}}}
	view := diagramView(t, graph, packagegraph.ViewOptions{})
	text := diagramSource(t, view, PackageOptions{})
	nodes := regexp.MustCompile(`(?m)^package "([^"]*)" as ([A-Za-z0-9_]+)`).FindAllStringSubmatch(text, -1)
	if len(nodes) != 2 {
		t.Fatalf("distinct contextual packages lost:\n%s", text)
	}
	aliases := map[string]string{}
	for _, node := range nodes {
		if strings.Contains(node[1], "rootA") {
			aliases["rootA"] = node[2]
		}
		if strings.Contains(node[1], "rootB") {
			aliases["rootB"] = node[2]
		}
	}
	if aliases["rootA"] == "" || aliases["rootB"] == "" || aliases["rootA"] == aliases["rootB"] {
		t.Fatalf("contextual identities aliased:\n%s", text)
	}
	if !strings.Contains(text, aliases["rootB"]+" --> "+aliases["rootA"]+" : types=1; imports=0") {
		t.Fatalf("dependency endpoints misbound:\n%s", text)
	}
}

func TestPackageDiagramCountsTypeAndImportEvidenceSeparately(t *testing.T) {
	a, b := diagramPackage("root", "a.b"), diagramPackage("root", "a.b.c")
	edge := packagegraph.Dependency{From: a.ID, To: b.ID, TypeRelations: []relations.Relation{diagramRelation(true), diagramRelation(false)}, Imports: []packagegraph.ImportEvidence{{File: packagegraph.File{Path: "A.java"}, Import: model.Import{Name: "a.b.c.C"}}}}
	graph := packagegraph.Graph{Source: packagegraph.Both, Packages: []packagegraph.Package{a, b}, Dependencies: []packagegraph.Dependency{edge}}
	view := diagramView(t, graph, packagegraph.ViewOptions{})
	text := diagramSource(t, view, PackageOptions{})
	if strings.Count(text, " --> ") != 1 || !strings.Contains(text, "types=2; imports=1; inferred-only=1") {
		t.Fatalf("counting units or mixed edge certainty lost:\n%s", text)
	}
	graph.Dependencies[0].Imports = nil
	graph.Dependencies[0].TypeRelations = []relations.Relation{diagramRelation(true)}
	view = diagramView(t, graph, packagegraph.ViewOptions{})
	text = diagramSource(t, view, PackageOptions{})
	if strings.Count(text, " ..> ") != 1 || !strings.Contains(text, "types=1; imports=0; inferred-only=1") {
		t.Fatalf("inferred-only edge presented as ordinary proof:\n%s", text)
	}
}

func TestPackageDiagramExternalVisibilityAndEmptyProjection(t *testing.T) {
	a, b := diagramPackage("root", "app"), diagramPackage("root", "example.net/external")
	b.Kind = packagegraph.ExternalPackage
	graph := packagegraph.Graph{Source: packagegraph.Imports, Packages: []packagegraph.Package{a, b}, Dependencies: []packagegraph.Dependency{{From: a.ID, To: b.ID, Imports: []packagegraph.ImportEvidence{{Import: model.Import{Name: b.ID.Name}}}}}}
	for _, show := range []bool{false, true} {
		view := diagramView(t, graph, packagegraph.ViewOptions{ShowExternal: show})
		text := diagramSource(t, view, PackageOptions{})
		nodes := regexp.MustCompile(`(?m)^package "`).FindAllString(text, -1)
		want := 1
		if show {
			want = 2
		}
		if len(nodes) != want || strings.Contains(text, " --> ") != show {
			t.Fatalf("external visibility lost (show=%v):\n%s", show, text)
		}
	}
	empty := diagramView(t, packagegraph.Graph{Source: packagegraph.DeclaredTypes}, packagegraph.ViewOptions{})
	text := diagramSource(t, empty, PackageOptions{})
	if strings.Contains(text, "package \"") || strings.Contains(text, " --> ") || !strings.Contains(text, "note \"") {
		t.Fatalf("empty projection invented nodes or edges:\n%s", text)
	}
}

func TestPackageDiagramEscapesUntrustedNamesTitlesAndContexts(t *testing.T) {
	malicious := "evil\"\\n}\n!include https://invalid.example/payload\n@enduml\n<img:https://invalid.example/img> [[file:///secret]] %load_json(\"file:///secret\") $value"
	a, b := diagramPackage(malicious, malicious), diagramPackage("other", malicious)
	graph := packagegraph.Graph{Source: packagegraph.DeclaredTypes, Packages: []packagegraph.Package{a, b}}
	view := diagramView(t, graph, packagegraph.ViewOptions{})
	text := diagramSource(t, view, PackageOptions{Title: malicious})
	if strings.Count(text, "@startuml") != 1 || strings.Count(text, "@enduml") != 1 || strings.Count(text, "\npackage ") != 2 {
		t.Fatalf("untrusted text changed diagram structure:\n%s", text)
	}
	for _, directive := range []string{"!include", "<img:", "[[file:", "%load_json", "$value"} {
		if strings.Contains(text, directive) {
			t.Fatalf("untrusted directive %q survived escaping:\n%s", directive, text)
		}
	}
}

func TestPackageDiagramIsDeterministicWithoutMutatingView(t *testing.T) {
	a, b, c := diagramPackage("root", "a"), diagramPackage("root", "b"), diagramPackage("root", "c")
	graph := packagegraph.Graph{Source: packagegraph.DeclaredTypes, Packages: []packagegraph.Package{c, b, a}, Dependencies: []packagegraph.Dependency{{From: b.ID, To: a.ID, TypeRelations: []relations.Relation{diagramRelation(false)}}, {From: a.ID, To: b.ID, TypeRelations: []relations.Relation{diagramRelation(false)}}}}
	view := diagramView(t, graph, packagegraph.ViewOptions{})
	before, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	want := diagramSource(t, view, PackageOptions{SkinParams: map[string]string{"shadowing": "false", "linetype": "ortho"}})
	shuffled := view
	shuffled.Graph.Packages = slices.Clone(view.Graph.Packages)
	slices.Reverse(shuffled.Graph.Packages)
	shuffled.Graph.Dependencies = slices.Clone(view.Graph.Dependencies)
	slices.Reverse(shuffled.Graph.Dependencies)
	shuffled.Cycles = slices.Clone(view.Cycles)
	for n, cycle := range shuffled.Cycles {
		shuffled.Cycles[n].Packages = slices.Clone(cycle.Packages)
		slices.Reverse(shuffled.Cycles[n].Packages)
	}
	got := diagramSource(t, shuffled, PackageOptions{SkinParams: map[string]string{"linetype": "ortho", "shadowing": "false"}})
	if got != want {
		t.Fatal("package/member enumeration order changed the diagram")
	}
	after, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("diagram generation mutated its source view")
	}
}

func TestPackageDiagramRejectsUnsafeOrClassOnlyLayout(t *testing.T) {
	view := diagramView(t, packagegraph.Graph{Source: packagegraph.DeclaredTypes}, packagegraph.ViewOptions{})
	for _, opts := range []PackageOptions{
		{LayoutDirection: "sideways"},
		{SkinParams: map[string]string{"classAttributeIconSize": "0"}},
		{SkinParams: map[string]string{"linetype": "ortho\n!include secret"}},
		{SkinParams: map[string]string{"shadowing": "false\n@enduml"}},
	} {
		if text, err := GeneratePackage(view, opts); err == nil || text != "" {
			t.Fatalf("unsafe package layout produced a diagram: %#v", opts)
		}
	}
}
