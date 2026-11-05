package explain

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/packagediff"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/relations"
)

func explanationDiffSnapshot(t *testing.T, revision string, graph packagegraph.Graph, complete bool) packagediff.Snapshot {
	t.Helper()
	view, err := packagegraph.Analyze(graph, packagegraph.ViewOptions{ShowExternal: true, Cycles: packagegraph.CyclesMark})
	if err != nil {
		t.Fatal(err)
	}
	return packagediff.Snapshot{Revision: revision, View: view, Complete: complete}
}

func explanationDiffFixture(t *testing.T) PackageDiffReport {
	t.Helper()
	path := "/captured-only/no-working-tree/A.java"
	a := packagegraph.Package{ID: packagegraph.PackageID{Language: "java", Context: "/repo", Name: "app"}, Kind: packagegraph.InternalPackage, Files: []packagegraph.File{{Path: path, ContentHash: "before-hash"}}}
	b := packagegraph.Package{ID: packagegraph.PackageID{Language: "java", Context: "/repo", Name: "dep"}, Kind: packagegraph.InternalPackage}
	relation := relations.Relation{From: "app.A", To: "dep.Target", Kind: relations.Field, Label: "oldField", Evidence: []relations.Evidence{{Source: path, Line: 4, Member: "oldField", Type: "Target", Language: "java"}}}
	imported := packagegraph.ImportEvidence{File: a.Files[0], Import: model.Import{Name: "dep.Target", Span: model.SourceSpan{Start: model.SourcePosition{Line: 2, Column: 1, Offset: 13}, End: model.SourcePosition{Line: 2, Column: 19, Offset: 31}}}}
	beforeGraph := packagegraph.Graph{Source: packagegraph.Both, Packages: []packagegraph.Package{a, b}, Dependencies: []packagegraph.Dependency{{From: a.ID, To: b.ID, TypeRelations: []relations.Relation{relation}, Imports: []packagegraph.ImportEvidence{imported}}}}
	afterA := a
	afterA.Files = []packagegraph.File{{Path: path, ContentHash: "after-hash"}}
	afterRelation := relation
	afterRelation.Label = "newField"
	afterRelation.Evidence = []relations.Evidence{{Source: path, Line: 5, Member: "newField", Type: "Target", Language: "java"}}
	afterImport := imported
	afterImport.File = afterA.Files[0]
	afterImport.Import.Static = true
	afterGraph := packagegraph.Graph{Source: packagegraph.Both, Packages: []packagegraph.Package{afterA, b}, Dependencies: []packagegraph.Dependency{{From: a.ID, To: b.ID, TypeRelations: []relations.Relation{afterRelation}, Imports: []packagegraph.ImportEvidence{afterImport}}}}
	before := explanationDiffSnapshot(t, strings.Repeat("a", 40), beforeGraph, true)
	after := explanationDiffSnapshot(t, strings.Repeat("b", 40), afterGraph, true)
	return PackageDiffReport{
		Diff:   packagediff.Compare(before, after),
		Before: PackageReport{Known: beforeGraph, View: before.View, Scope: Scope{Files: 1, Language: "java"}, Sources: map[string]Source{path: {Content: []byte("package app;\nimport dep.Target;\nclass A {\n Target oldField;\n}\n"), Origin: "Git " + before.Revision}}},
		After:  PackageReport{Known: afterGraph, View: after.View, Scope: Scope{Files: 1, Language: "java"}, Sources: map[string]Source{path: {Content: []byte("package app;\nimport static dep.Target;\n\nclass A {\n Target newField;\n}\n"), Origin: "Git " + after.Revision}}},
	}
}

func TestPackageDiffExplanationUsesCorrectCapturedSideAndLocations(t *testing.T) {
	report := explanationDiffFixture(t)
	text := FormatPackageDiff(report)
	removed := strings.Index(text, "Removed evidence (before Git snapshot)")
	added := strings.Index(text, "Added evidence (after Git snapshot)")
	cycles := strings.Index(text, "Cycle changes in both full selected graphs")
	if removed < 0 || added <= removed || cycles <= added {
		t.Fatal("located semantic evidence sections missing")
	}
	beforeEvidence, afterEvidence := text[removed:added], text[added:cycles]
	requireContains(t, beforeEvidence, "4 |  Target oldField;", "2 | import dep.Target;", "Git "+report.Diff.Before.Revision, ":4", ":2:1-2:19", "before-hash")
	requireContains(t, afterEvidence, "5 |  Target newField;", "2 | import static dep.Target;", "Git "+report.Diff.After.Revision, ":5", "after-hash")
	if strings.Contains(beforeEvidence, "newField") || strings.Contains(afterEvidence, "oldField") {
		t.Fatal("evidence read from the wrong Git snapshot")
	}
	requireContains(t, text, "modified evidence; existing edge retained", "Before: type relations=1; import declarations=1", "After: type relations=1; import declarations=1", "dirty, staged and untracked", "Build tags", "classpath")
}

func TestPackageDiffExplanationIsPureAndDoesNotChangeDiagramBytes(t *testing.T) {
	report := explanationDiffFixture(t)
	original, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	before, err := plantuml.GeneratePackageDiff(report.Diff, plantuml.PackageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	first := FormatPackageDiff(report)
	second := FormatPackageDiff(report)
	if first != second {
		t.Fatal("repeated formatting changed explanation")
	}
	after, err := plantuml.GeneratePackageDiff(report.Diff, plantuml.PackageOptions{})
	if err != nil || before != after {
		t.Fatal("explanation changed diagram bytes")
	}
	current, err := json.Marshal(report)
	if err != nil || string(current) != string(original) {
		t.Fatal("formatting mutated captured analysis")
	}
}

func TestPackageDiffExplanationRetainsUnchangedContextAndHiddenExternal(t *testing.T) {
	report := explanationDiffFixture(t)
	external := packagegraph.Package{ID: packagegraph.PackageID{Language: "java", Context: "external", Name: "external.lib"}, Kind: packagegraph.ExternalPackage}
	before := report.Diff.Before
	before.View.Graph.Packages = append(slices.Clone(before.View.Graph.Packages), external)
	before.View.Scope.PackageCount++
	report.Diff = packagediff.Compare(before, before)
	report.Before.Known = before.View.Graph
	report.After.Known = before.View.Graph
	shown := FormatPackageDiff(report)
	requireContains(t, shown, "unchanged context", "External import-path identity only", "external.lib")
	report.Diff = packagediff.Project(report.Diff, false, false)
	hidden := FormatPackageDiff(report)
	if len(report.Diff.Packages) != 2 {
		t.Fatal("fixture external projection failed")
	}
	requireContains(t, hidden, "[external, hidden]", "external.lib", "hidden evidence does not become a removed dependency")
	if strings.Contains(hidden, "[removed]") {
		t.Fatal("display hiding reclassified unchanged evidence as removed")
	}
}

func TestPackageDiffExplanationUncertainCyclesAndFailures(t *testing.T) {
	report := explanationDiffFixture(t)
	graph := report.Diff.Before.View.Graph
	a, b := graph.Packages[0], graph.Packages[1]
	graph.Dependencies = append(slices.Clone(graph.Dependencies), packagegraph.Dependency{From: b.ID, To: a.ID, TypeRelations: []relations.Relation{{From: "dep.Target", To: "app.A", Kind: relations.Field}}})
	before := explanationDiffSnapshot(t, "before-sha", graph, true)
	graph.Dependencies = nil
	graph.Diagnostics = []packagegraph.Diagnostic{{From: a.ID, Code: "syntax-error", File: a.Files[0], Reference: "broken declaration"}}
	after := explanationDiffSnapshot(t, "after-sha", graph, false)
	report.Diff = packagediff.Project(packagediff.Compare(before, after), true, true)
	report.After.Failures = []Failure{{Source: a.Files[0].Path, Origin: "Git after-sha", Message: "captured parse failure"}}
	text := FormatPackageDiff(report)
	requireContains(t, text, "WARNING: incomplete analysis", "declared cycle group [uncertain", "captured parse failure", "syntax-error", "Before groups=1")
	if strings.Contains(text, "cycle group [removed") {
		t.Fatal("failed after analysis claimed cycle resolution")
	}
	report.Diff.Before.View.Scope.Cycles = packagegraph.CyclesOff
	report.Diff.After.View.Scope.Cycles = packagegraph.CyclesOff
	text = FormatPackageDiff(report)
	if strings.Contains(text, "declared cycle group") || strings.Contains(text, "declared cycle group (strongly") {
		t.Fatal("cycle-off explanation emitted cycle findings")
	}
}

func TestPackageDiffExplanationEmptyAndMissingCapturedSource(t *testing.T) {
	empty := explanationDiffSnapshot(t, "empty", packagegraph.Graph{Source: packagegraph.Imports}, true)
	text := FormatPackageDiff(PackageDiffReport{Diff: packagediff.Compare(empty, empty)})
	requireContains(t, text, "0 packages, 0 dependencies", "No packages are visible")
	report := explanationDiffFixture(t)
	report.Before.Sources = nil
	report.After.Sources = nil
	text = FormatPackageDiff(report)
	requireContains(t, text, "Source snippet unavailable: no captured source snapshot", "origin=origin unknown", "newField", "oldField")
}
