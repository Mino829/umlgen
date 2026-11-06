package plantuml

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/packagediff"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/relations"
)

func diffDiagramSnapshot(t *testing.T, revision string, graph packagegraph.Graph, complete bool) packagediff.Snapshot {
	t.Helper()
	return packagediff.Snapshot{Revision: revision, Complete: complete, View: diagramView(t, graph, packagegraph.ViewOptions{ShowExternal: true, Cycles: packagegraph.CyclesMark})}
}

func diffDiagramText(t *testing.T, result packagediff.Result) string {
	t.Helper()
	text, err := GeneratePackageDiff(result, PackageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestPackageDiffEdgeStatusesCountsAndBoundEndpoints(t *testing.T) {
	a, b := diagramPackage("rootA", "same"), diagramPackage("rootB", "same")
	before := packagegraph.Dependency{From: b.ID, To: a.ID, TypeRelations: []relations.Relation{diagramRelation(false)}, Imports: []packagegraph.ImportEvidence{{Import: model.Import{Name: "a.A"}}}}
	after := before
	after.TypeRelations = []relations.Relation{diagramRelation(false), diagramRelation(true)}
	after.Imports = nil
	graph := packagegraph.Graph{Source: packagegraph.Both, Packages: []packagegraph.Package{b, a}, Dependencies: []packagegraph.Dependency{before}}
	snapshot := diffDiagramSnapshot(t, strings.Repeat("a", 40), graph, true)
	for _, kind := range []packagediff.Kind{packagediff.Added, packagediff.Removed, packagediff.Modified, packagediff.Unchanged, packagediff.Uncertain} {
		t.Run(string(kind), func(t *testing.T) {
			change := packagediff.DependencyChange{From: b.ID, To: a.ID, Before: &before, After: &after, Kind: kind}
			if kind == packagediff.Added {
				change.Before = nil
			}
			if kind == packagediff.Removed {
				change.After = nil
			}
			if kind == packagediff.Modified {
				change.AddedEvidence = []packagediff.Evidence{{Relation: &after.TypeRelations[1]}}
				change.RemovedEvidence = []packagediff.Evidence{{Import: &before.Imports[0]}}
			}
			result := packagediff.Result{Before: snapshot, After: snapshot, Packages: []packagediff.PackageChange{{ID: b.ID, Before: &b, After: &b, Kind: packagediff.Unchanged}, {ID: a.ID, Before: &a, After: &a, Kind: packagediff.Unchanged}}, Dependencies: []packagediff.DependencyChange{change}}
			text := diffDiagramText(t, result)
			nodes := regexp.MustCompile(`(?m)^package "([^"]*)" as (PKG_\d+)`).FindAllStringSubmatch(text, -1)
			aliases := map[string]string{}
			for _, node := range nodes {
				for _, context := range []string{"rootA", "rootB"} {
					if strings.Contains(node[1], context) {
						aliases[context] = node[2]
					}
				}
			}
			if len(nodes) != 2 || aliases["rootA"] == aliases["rootB"] {
				t.Fatalf("context identities lost: %v", nodes)
			}
			edgePattern := regexp.QuoteMeta(aliases["rootB"]) + ` -\[[^]]+\]-> ` + regexp.QuoteMeta(aliases["rootA"]) + ` : `
			if !regexp.MustCompile(edgePattern).MatchString(text) {
				t.Fatal("edge endpoints were merged or reversed")
			}
			if kind == packagediff.Modified && (!strings.Contains(text, "before: types=1; imports=1; after: types=2; imports=0") || !strings.Contains(text, "evidence +1 / -1")) {
				t.Fatal("separate counting units or semantic evidence delta missing")
			}
		})
	}
}

func TestPackageDiffPackageStatusesAndExternalIdentity(t *testing.T) {
	a, b, c := diagramPackage("root", "old"), diagramPackage("root", "new"), diagramPackage("external", "library")
	c.Kind = packagegraph.ExternalPackage
	before := diffDiagramSnapshot(t, strings.Repeat("a", 40), packagegraph.Graph{Source: packagegraph.Imports, Packages: []packagegraph.Package{a, c}}, true)
	after := diffDiagramSnapshot(t, strings.Repeat("b", 40), packagegraph.Graph{Source: packagegraph.Imports, Packages: []packagegraph.Package{b, c}}, true)
	text := diffDiagramText(t, packagediff.Compare(before, after))
	if strings.Count(text, "\npackage ") != 3 || !strings.Contains(text, "[external import path;") {
		t.Fatal("union or external identity annotation lost")
	}
	for _, sha := range []string{before.Revision, after.Revision} {
		if !strings.Contains(text, sha) {
			t.Fatal("immutable provenance omitted")
		}
	}
}

func TestPackageDiffEscapesEveryUserTextBoundary(t *testing.T) {
	attack := "\"\n@enduml\n!include secret\nlegend\n<img:secret> [[file:///secret]] %load_json(\"secret\") $secret"
	a := diagramPackage(attack, attack)
	graph := packagegraph.Graph{Source: packagegraph.DeclaredTypes, Packages: []packagegraph.Package{a}}
	snapshot := diffDiagramSnapshot(t, attack, graph, true)
	// Exercise the renderer boundary independently of Analyze's language enum
	// validation; all displayed metadata must remain inert even if malformed.
	snapshot.View.Graph.Packages[0].ID.Language = attack
	a.ID.Language = attack
	result := packagediff.Compare(snapshot, snapshot)
	result.Before.View.Scope.Relations = []relations.Kind{relations.Kind(attack)}
	result.After.View.Scope.Relations = []relations.Kind{relations.Kind(attack)}
	result.Cycles = []packagediff.CycleChange{{Before: &packagegraph.Cycle{Kind: packagegraph.DeclaredCycle, Packages: []packagegraph.PackageID{a.ID}}, After: &packagegraph.Cycle{Kind: packagegraph.DeclaredCycle, Packages: []packagegraph.PackageID{a.ID}}, Kind: packagediff.Unchanged}}
	text, err := GeneratePackageDiff(result, PackageOptions{Title: attack})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(text, "@startuml") != 1 || strings.Count(text, "@enduml") != 1 || strings.Count(text, "\nlegend ") != 1 || strings.Count(text, "\npackage ") != 1 {
		t.Fatal("untrusted text changed diagram structure")
	}
	for _, unsafe := range []string{"!include", "<img:", "[[file:", "%load_json", "$secret"} {
		if strings.Contains(text, unsafe) {
			t.Fatalf("unescaped input: %q", unsafe)
		}
	}
}

func TestPackageDiffRejectsInconsistentModelsAndUnsafeLayout(t *testing.T) {
	a, b := diagramPackage("root", "a"), diagramPackage("root", "b")
	edge := packagegraph.Dependency{From: a.ID, To: b.ID}
	snapshot := diffDiagramSnapshot(t, "abc", packagegraph.Graph{Source: packagegraph.Imports, Packages: []packagegraph.Package{a, b}}, true)
	base := packagediff.Compare(snapshot, snapshot)
	for _, corrupt := range []func(*packagediff.Result){
		func(r *packagediff.Result) {
			r.Dependencies = []packagediff.DependencyChange{{From: b.ID, To: a.ID, Before: &edge, Kind: packagediff.Removed}}
		},
		func(r *packagediff.Result) {
			r.Dependencies = []packagediff.DependencyChange{{From: a.ID, To: packagegraph.PackageID{Name: "missing"}, Before: &edge, Kind: packagediff.Removed}}
		},
		func(r *packagediff.Result) { r.Packages = append(slices.Clone(r.Packages), r.Packages[0]) },
		func(r *packagediff.Result) {
			r.Packages = slices.Clone(r.Packages)
			r.Packages[0].Kind = packagediff.Kind("!include secret")
		},
		func(r *packagediff.Result) {
			r.Packages = slices.Clone(r.Packages)
			r.Packages[0].Before = nil
			r.Packages[0].After = nil
		},
	} {
		result := base
		corrupt(&result)
		if text, err := GeneratePackageDiff(result, PackageOptions{}); err == nil || text != "" {
			t.Fatal("inconsistent model produced output")
		}
	}
	if text, err := GeneratePackageDiff(base, PackageOptions{SkinParams: map[string]string{"linetype": "ortho\n!include secret"}}); err == nil || text != "" {
		t.Fatal("unsafe layout produced output")
	}
}

func TestPackageDiffUncertainCyclesAndHiddenProjection(t *testing.T) {
	a, b := diagramPackage("root", "a"), diagramPackage("root", "b")
	graph := packagegraph.Graph{Source: packagegraph.DeclaredTypes, Packages: []packagegraph.Package{a, b}, Dependencies: []packagegraph.Dependency{{From: a.ID, To: b.ID, TypeRelations: []relations.Relation{diagramRelation(false)}}, {From: b.ID, To: a.ID, TypeRelations: []relations.Relation{diagramRelation(false)}}}}
	before := diffDiagramSnapshot(t, "before", graph, true)
	graph.Dependencies = nil
	after := diffDiagramSnapshot(t, "after", graph, false)
	result := packagediff.Project(packagediff.Compare(before, after), true, true)
	text := diffDiagramText(t, result)
	if !strings.Contains(text, "Cycle group: declared; uncertain") || strings.Contains(text, "Cycle group: declared; removed") {
		t.Fatal("incomplete after snapshot claimed a resolved cycle")
	}
	if !strings.Contains(text, "before=1; after=0") || len(result.Cycles) != 1 {
		t.Fatal("projection lost full-scope cycle results")
	}
	result.Before.View.Scope.Cycles = packagegraph.CyclesOff
	result.After.View.Scope.Cycles = packagegraph.CyclesOff
	text = diffDiagramText(t, result)
	if strings.Contains(text, "Cycle group:") || strings.Contains(text, "Full selected-scope cycles:") {
		t.Fatal("cycles-off emitted cycle findings")
	}
}

func TestPackageDiffEmptyAndDeterministicWithoutMutation(t *testing.T) {
	empty := diffDiagramSnapshot(t, "empty", packagegraph.Graph{Source: packagegraph.Imports}, true)
	text := diffDiagramText(t, packagediff.Compare(empty, empty))
	if !strings.Contains(text, "note \"") || strings.Contains(text, "\npackage ") || strings.Contains(text, "]->") {
		t.Fatal("empty result invented graph contents")
	}
	a, b := diagramPackage("root", "a"), diagramPackage("root", "b")
	snapshot := diffDiagramSnapshot(t, "commit", packagegraph.Graph{Source: packagegraph.Imports, Packages: []packagegraph.Package{a, b}}, true)
	result := packagediff.Compare(snapshot, snapshot)
	original, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	want := diffDiagramText(t, result)
	shuffled := result
	shuffled.Packages = slices.Clone(result.Packages)
	slices.Reverse(shuffled.Packages)
	if got := diffDiagramText(t, shuffled); got != want {
		t.Fatal("enumeration order changed output")
	}
	current, err := json.Marshal(result)
	if err != nil || string(current) != string(original) {
		t.Fatal("renderer mutated comparison")
	}
}
