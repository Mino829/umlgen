package packagediff

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/Mino829/umlgen/internal/java"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/relations"
)

var (
	appID = packagegraph.PackageID{Language: "java", Context: "/repo", Name: "app"}
	depID = packagegraph.PackageID{Language: "java", Context: "/repo", Name: "dep"}
)

func testImport(path, name string, line int) packagegraph.ImportEvidence {
	return packagegraph.ImportEvidence{
		File:   packagegraph.File{Path: path, ContentHash: "hash"},
		Import: model.Import{Name: name, Span: model.SourceSpan{Start: model.SourcePosition{Offset: line * 10, Line: line, Column: 1}, End: model.SourcePosition{Offset: line*10 + 5, Line: line, Column: 6}}},
	}
}

func testRelation(member string, line int) relations.Relation {
	return relations.Relation{
		From: "app.App", To: "dep.Target", Kind: relations.Field, Label: "target", Multiplicity: "*",
		Evidence: []relations.Evidence{{Source: "/repo/App.java", Member: member, Type: "Target[]", Line: line, Language: "java", OwnerKind: model.Class}},
	}
}

func testSnapshot(dep *packagegraph.Dependency) Snapshot {
	graph := packagegraph.Graph{Source: packagegraph.Both, Packages: []packagegraph.Package{
		{ID: appID, Kind: packagegraph.InternalPackage, Files: []packagegraph.File{{Path: "/repo/App.java", ContentHash: "app"}}},
		{ID: depID, Kind: packagegraph.InternalPackage, Files: []packagegraph.File{{Path: "/repo/Target.java", ContentHash: "target"}}},
	}}
	if dep != nil {
		graph.Dependencies = []packagegraph.Dependency{*dep}
	}
	return Snapshot{Revision: "revision", Complete: true, View: packagegraph.View{Graph: graph, Scope: packagegraph.AnalysisScope{Source: graph.Source, PackageCount: 2, DependencyCount: len(graph.Dependencies), ShowExternal: true, Cycles: packagegraph.CyclesMark}}}
}

func testDependency(imports ...packagegraph.ImportEvidence) *packagegraph.Dependency {
	return &packagegraph.Dependency{From: appID, To: depID, Imports: imports}
}

func onlyDependency(t *testing.T, result Result, kind Kind) DependencyChange {
	t.Helper()
	if len(result.Dependencies) != 1 || result.Dependencies[0].Kind != kind {
		t.Fatalf("dependency changes=%#v; want one %s", result.Dependencies, kind)
	}
	return result.Dependencies[0]
}

func TestImportEvidenceMultisetAndLastProof(t *testing.T) {
	first := testImport("/repo/First.java", "dep.Target", 2)
	second := testImport("/repo/Second.java", "dep.Target", 2)
	before := testSnapshot(testDependency(first, second))
	change := onlyDependency(t, Compare(before, testSnapshot(testDependency(second))), Modified)
	if len(change.RemovedEvidence) != 1 || change.RemovedEvidence[0].Import.File.Path != first.File.Path || len(change.AddedEvidence) != 0 {
		t.Fatalf("non-last file removal=%#v", change)
	}
	if change.Before == nil || change.After == nil {
		t.Fatal("non-last proof removal removed the dependency")
	}
	change = onlyDependency(t, Compare(testSnapshot(testDependency(second)), testSnapshot(nil)), Removed)
	if change.After != nil || len(change.RemovedEvidence) != 1 {
		t.Fatalf("last proof removal=%#v", change)
	}

	duplicate := first
	duplicate.Import.Span.Start.Line = 7
	change = onlyDependency(t, Compare(testSnapshot(testDependency(first, duplicate)), testSnapshot(testDependency(first))), Modified)
	if len(change.RemovedEvidence) != 1 || change.RemovedEvidence[0].Import.Import.Span.Start.Line != 7 {
		t.Fatalf("duplicate import count or original span lost: %#v", change)
	}
	change = onlyDependency(t, Compare(testSnapshot(testDependency(first)), testSnapshot(testDependency(first, duplicate))), Modified)
	if len(change.AddedEvidence) != 1 {
		t.Fatalf("duplicate import addition=%#v", change)
	}
}

func TestSameCountImportReplacementAndSemanticFields(t *testing.T) {
	original := testImport("/repo/App.java", "dep.Target", 2)
	cases := map[string]func(*packagegraph.ImportEvidence){
		"name":          func(e *packagegraph.ImportEvidence) { e.Import.Name = "dep.Other" },
		"alias":         func(e *packagegraph.ImportEvidence) { e.Import.Alias = "alternate" },
		"static":        func(e *packagegraph.ImportEvidence) { e.Import.Static = true },
		"wildcard":      func(e *packagegraph.ImportEvidence) { e.Import.Wildcard = true },
		"original file": func(e *packagegraph.ImportEvidence) { e.File.Path = "/repo/Renamed.java" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			replacement := original
			mutate(&replacement)
			change := onlyDependency(t, Compare(testSnapshot(testDependency(original)), testSnapshot(testDependency(replacement))), Modified)
			if len(change.AddedEvidence) != 1 || len(change.RemovedEvidence) != 1 {
				t.Fatalf("same count replacement=%#v", change)
			}
			if !reflect.DeepEqual(*change.AddedEvidence[0].Import, replacement) || !reflect.DeepEqual(*change.RemovedEvidence[0].Import, original) {
				t.Fatal("replacement evidence lost original metadata")
			}
		})
	}
}

func TestRelationEvidenceSemanticFields(t *testing.T) {
	cases := map[string]func(*relations.Relation){
		"from":         func(r *relations.Relation) { r.From = "app.Other" },
		"to":           func(r *relations.Relation) { r.To = "dep.Other" },
		"kind":         func(r *relations.Relation) { r.Kind = relations.Return },
		"label":        func(r *relations.Relation) { r.Label = "other" },
		"multiplicity": func(r *relations.Relation) { r.Multiplicity = "1" },
		"member":       func(r *relations.Relation) { r.Evidence[0].Member = "other" },
		"reference":    func(r *relations.Relation) { r.Evidence[0].Type = "List<Target>" },
		"source":       func(r *relations.Relation) { r.Evidence[0].Source = "/repo/Renamed.java" },
		"inferred":     func(r *relations.Relation) { r.Evidence[0].Inferred = true },
		"language":     func(r *relations.Relation) { r.Evidence[0].Language = "go" },
		"owner kind":   func(r *relations.Relation) { r.Evidence[0].OwnerKind = model.Interface },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			original, replacement := testRelation("field", 5), testRelation("field", 5)
			mutate(&replacement)
			before, after := testDependency(), testDependency()
			before.TypeRelations, after.TypeRelations = []relations.Relation{original}, []relations.Relation{replacement}
			change := onlyDependency(t, Compare(testSnapshot(before), testSnapshot(after)), Modified)
			if len(change.AddedEvidence) != 1 || len(change.RemovedEvidence) != 1 {
				t.Fatalf("relation replacement=%#v", change)
			}
			if !reflect.DeepEqual(*change.AddedEvidence[0].Relation, replacement) || !reflect.DeepEqual(*change.RemovedEvidence[0].Relation, original) {
				t.Fatal("relation identity or declaration evidence lost")
			}
		})
	}
}

func TestRelationProofRemovalRetainsOtherDeclarationAndImport(t *testing.T) {
	relation := testRelation("first", 5)
	relation.Evidence = append(relation.Evidence, testRelation("second", 6).Evidence[0])
	before := testDependency(testImport("/repo/App.java", "dep.Target", 2))
	before.TypeRelations = []relations.Relation{relation}
	after := testDependency(before.Imports...)
	after.TypeRelations = []relations.Relation{testRelation("second", 6)}
	change := onlyDependency(t, Compare(testSnapshot(before), testSnapshot(after)), Modified)
	if len(change.RemovedEvidence) != 1 || change.RemovedEvidence[0].Relation.Evidence[0].Member != "first" || len(change.RemovedEvidence[0].Relation.Evidence) != 1 {
		t.Fatalf("declaration delta=%#v", change)
	}
	change = onlyDependency(t, Compare(testSnapshot(after), testSnapshot(testDependency(before.Imports...))), Modified)
	if change.After == nil || len(change.RemovedEvidence) != 1 {
		t.Fatal("import proof did not retain edge")
	}

	bare := testRelation("ignored", 5)
	bare.Evidence = nil
	before.TypeRelations = []relations.Relation{bare}
	change = onlyDependency(t, Compare(testSnapshot(before), testSnapshot(testDependency(before.Imports...))), Modified)
	if len(change.RemovedEvidence) != 1 || change.RemovedEvidence[0].Relation == nil || len(change.RemovedEvidence[0].Relation.Evidence) != 0 {
		t.Fatal("bare relation proof lost")
	}
}

func TestLocationHashAndOrderNoiseIsUnchanged(t *testing.T) {
	beforeDep := testDependency(testImport("/repo/App.java", "dep.Target", 2), testImport("/repo/App.java", "dep.Target", 4))
	beforeDep.TypeRelations = []relations.Relation{testRelation("field", 5), testRelation("other", 7)}
	afterDep := testDependency(testImport("/repo/App.java", "dep.Target", 20), testImport("/repo/App.java", "dep.Target", 40))
	afterDep.TypeRelations = []relations.Relation{testRelation("other", 70), testRelation("field", 50)}
	for i := range afterDep.Imports {
		afterDep.Imports[i].File.ContentHash = "different"
	}
	before, after := testSnapshot(beforeDep), testSnapshot(afterDep)
	after.Revision = "new revision"
	for i := range after.View.Graph.Packages {
		after.View.Graph.Packages[i].Files[0].ContentHash = "different"
	}
	slices.Reverse(after.View.Graph.Packages)
	change := onlyDependency(t, Compare(before, after), Unchanged)
	if len(change.AddedEvidence) != 0 || len(change.RemovedEvidence) != 0 {
		t.Fatal("location/hash/order generated evidence noise")
	}
	for _, pkg := range Compare(before, after).Packages {
		if pkg.Kind != Unchanged {
			t.Fatalf("hash-only package edit=%#v", pkg)
		}
	}
}

func TestPackageIdentityAndMembership(t *testing.T) {
	for _, name := range []string{"module name", "module directory", "language"} {
		t.Run(name, func(t *testing.T) {
			before, after := testSnapshot(nil), testSnapshot(nil)
			before.View.Graph.Packages = before.View.Graph.Packages[:1]
			after.View.Graph.Packages = after.View.Graph.Packages[:1]
			switch name {
			case "module name":
				after.View.Graph.Packages[0].ID.Name = "renamed/app"
			case "module directory":
				after.View.Graph.Packages[0].ID.Context = "/repo/moved"
			case "language":
				after.View.Graph.Packages[0].ID.Language = "go"
			}
			result := Compare(before, after)
			if len(result.Packages) != 2 {
				t.Fatalf("rename merged identities: %#v", result.Packages)
			}
			counts := map[Kind]int{}
			for _, pkg := range result.Packages {
				counts[pkg.Kind]++
			}
			if counts[Added] != 1 || counts[Removed] != 1 {
				t.Fatalf("rename kinds=%v", counts)
			}
		})
	}
	before, after := testSnapshot(nil), testSnapshot(nil)
	after.View.Graph.Packages[0].Files[0].Path = "/repo/Renamed.java"
	if got := Compare(before, after).Packages[0].Kind; got != Modified {
		t.Fatalf("file membership rename=%s", got)
	}
	after = testSnapshot(nil)
	after.View.Graph.Packages[0].Kind = packagegraph.ExternalPackage
	if got := Compare(before, after).Packages[0].Kind; got != Modified {
		t.Fatalf("package kind edit=%s", got)
	}

	secondContext := before.View.Graph.Packages[0]
	secondContext.ID.Context = "/repo/other-module"
	before.View.Graph.Packages = append(before.View.Graph.Packages, secondContext)
	after = before
	result := Compare(before, after)
	if len(result.Packages) != 3 || result.Packages[0].ID == result.Packages[2].ID {
		t.Fatal("identical display names in separate contexts merged")
	}
}

func TestPartialSnapshotsConservativeChanges(t *testing.T) {
	for _, incomplete := range []string{"before", "after", "both"} {
		t.Run(incomplete, func(t *testing.T) {
			before := testSnapshot(testDependency(testImport("/repo/App.java", "dep.Target", 2)))
			after := testSnapshot(testDependency(testImport("/repo/App.java", "dep.Other", 2)))
			before.Complete, after.Complete = incomplete == "after", incomplete == "before"
			onlyDependency(t, Compare(before, after), Uncertain)
			onlyDependency(t, Compare(before, before), Unchanged)
			left, right := before, after
			left.View.Graph.Dependencies = nil
			want := Added
			if !left.Complete {
				want = Uncertain
			}
			onlyDependency(t, Compare(left, right), want)
			left, right = before, after
			right.View.Graph.Dependencies = nil
			want = Removed
			if !right.Complete {
				want = Uncertain
			}
			onlyDependency(t, Compare(left, right), want)
		})
	}
	before, after := testSnapshot(nil), testSnapshot(nil)
	before.Complete = false
	before.View.Graph.Packages = nil
	for _, change := range Compare(before, after).Packages {
		if change.Kind != Uncertain {
			t.Fatal("package addition claimed against incomplete before")
		}
	}
	before, after = after, before
	for _, change := range Compare(before, after).Packages {
		if change.Kind != Uncertain {
			t.Fatal("package removal claimed against incomplete after")
		}
	}
}

func TestCyclesFullScopeIdentityAndIncompleteRemoval(t *testing.T) {
	before, after := testSnapshot(nil), testSnapshot(nil)
	before.View.Cycles = []packagegraph.Cycle{{Kind: packagegraph.DeclaredCycle, Packages: []packagegraph.PackageID{depID, appID}}}
	after.View.Cycles = []packagegraph.Cycle{{Kind: packagegraph.DeclaredCycle, Packages: []packagegraph.PackageID{appID, depID}}}
	result := Compare(before, after)
	if len(result.Cycles) != 1 || result.Cycles[0].Kind != Unchanged {
		t.Fatal("cycle order changed identity")
	}
	after.View.Cycles[0].Kind = packagegraph.CandidateCycle
	result = Compare(before, after)
	if len(result.Cycles) != 2 {
		t.Fatal("cycle kind replacement not represented separately")
	}
	for _, change := range result.Cycles {
		if change.Kind != Added && change.Kind != Removed {
			t.Fatalf("cycle kind change=%#v", change)
		}
	}
	after.View.Cycles = nil
	result = Compare(before, after)
	if len(result.Cycles) != 1 || result.Cycles[0].Kind != Removed {
		t.Fatal("complete cycle removal missing")
	}
	for _, side := range []string{"before", "after"} {
		left, right := before, after
		if side == "before" {
			left.Complete = false
		} else {
			right.Complete = false
		}
		result = Compare(left, right)
		if result.Cycles[0].Kind != Uncertain || result.Cycles[0].Before == nil || result.Cycles[0].After != nil {
			t.Fatal("incomplete cycle removal falsely resolved")
		}
		result = Compare(right, left)
		if result.Cycles[0].Kind != Uncertain {
			t.Fatal("incomplete cycle addition falsely asserted")
		}
	}
}

func TestProjectUsesUnionWithoutReclassification(t *testing.T) {
	forward := testDependency(testImport("/repo/App.java", "dep.Target", 2))
	backward := packagegraph.Dependency{From: depID, To: appID, Imports: []packagegraph.ImportEvidence{testImport("/repo/Target.java", "app.App", 2)}}
	before, after := testSnapshot(forward), testSnapshot(forward)
	before.View.Graph.Dependencies = append(before.View.Graph.Dependencies, backward)
	before.View.Cycles = []packagegraph.Cycle{{Kind: packagegraph.DeclaredCycle, Packages: []packagegraph.PackageID{appID, depID}}}
	after.View.Graph.Packages[1].Kind = packagegraph.ExternalPackage
	result := Compare(before, after)
	projected := Project(result, false, true)
	if len(projected.Packages) != 2 || len(projected.Dependencies) != 2 {
		t.Fatal("before-only visible cycle members or edges lost from union")
	}
	if projected.Dependencies[0].Kind != Unchanged || projected.Dependencies[1].Kind != Removed {
		t.Fatalf("display filtering reclassified edges: %#v", projected.Dependencies)
	}
	if !reflect.DeepEqual(projected.Before, result.Before) || !reflect.DeepEqual(projected.After, result.After) || !reflect.DeepEqual(projected.Cycles, result.Cycles) {
		t.Fatal("projection altered snapshots, scope or full cycle changes")
	}
	if len(Project(result, false, false).Dependencies) != 2 {
		t.Fatal("internal-before/external-after package should remain union-visible")
	}

	before, after = testSnapshot(forward), testSnapshot(forward)
	before.View.Graph.Packages[1].Kind = packagegraph.ExternalPackage
	after.View.Graph.Packages[1].Kind = packagegraph.ExternalPackage
	result = Compare(before, after)
	if got := Project(result, false, false); len(got.Packages) != 1 || len(got.Dependencies) != 0 {
		t.Fatal("always-external package not hidden")
	}
	if result.Dependencies[0].Kind != Unchanged {
		t.Fatal("hidden external edge reclassified")
	}
	if got := Project(result, true, true); len(got.Packages) != 0 || len(got.Dependencies) != 0 {
		t.Fatal("cycle-only projection included non-cycle packages")
	}
}

func TestCompareAndProjectDoNotMutateInputs(t *testing.T) {
	dep := testDependency(testImport("/repo/Z.java", "dep.Target", 7), testImport("/repo/A.java", "dep.Target", 2))
	relation := testRelation("second", 9)
	relation.Evidence = append(relation.Evidence, testRelation("first", 3).Evidence[0])
	dep.TypeRelations = []relations.Relation{relation}
	before, after := testSnapshot(dep), testSnapshot(nil)
	before.View.Cycles = []packagegraph.Cycle{{Kind: packagegraph.DeclaredCycle, Packages: []packagegraph.PackageID{depID, appID}}}
	original, err := json.Marshal([]Snapshot{before, after})
	if err != nil {
		t.Fatal(err)
	}
	result := Compare(before, after)
	Project(result, false, true)
	current, err := json.Marshal([]Snapshot{before, after})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, current) {
		t.Fatal("comparison or projection mutated input")
	}
	change := onlyDependency(t, result, Removed)
	for _, evidence := range change.RemovedEvidence {
		if (evidence.Relation == nil) == (evidence.Import == nil) {
			t.Fatal("evidence must have exactly one proof category")
		}
		if evidence.Relation != nil && len(evidence.Relation.Evidence) != 1 {
			t.Fatal("relation proof was not flattened")
		}
	}
}

func sourceSnapshot(t *testing.T, files map[string]string) Snapshot {
	t.Helper()
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	var sources []packagegraph.Source
	for _, path := range paths {
		unit, err := java.ParseSource(path, []byte(files[path]))
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, packagegraph.Source{Context: "/repo", Unit: unit})
	}
	graph, err := packagegraph.Build(sources, packagegraph.Both)
	if err != nil {
		t.Fatal(err)
	}
	view, err := packagegraph.Analyze(graph, packagegraph.ViewOptions{ShowExternal: true, Cycles: packagegraph.CyclesMark})
	if err != nil {
		t.Fatal(err)
	}
	return Snapshot{Revision: "source", Complete: true, View: view}
}

func TestOriginalSourceFileDeletionRetainsDependencyUntilLastProof(t *testing.T) {
	files := map[string]string{
		"/repo/First.java":  "package app; import dep.Target; class First { Target field; }",
		"/repo/Second.java": "package app; import dep.Target; class Second { Target field; }",
		"/repo/Target.java": "package dep; class Target {}",
	}
	before := sourceSnapshot(t, files)
	delete(files, "/repo/First.java")
	after := sourceSnapshot(t, files)
	change := onlyDependency(t, Compare(before, after), Modified)
	if len(change.RemovedEvidence) != 2 || change.After == nil {
		t.Fatalf("non-last source file deletion=%#v", change)
	}
	delete(files, "/repo/Second.java")
	last := onlyDependency(t, Compare(after, sourceSnapshot(t, files)), Removed)
	if len(last.RemovedEvidence) != 2 || last.After != nil {
		t.Fatalf("last source file deletion=%#v", last)
	}
}

func TestUnsupportedSourceCannotProvePackageRemoval(t *testing.T) {
	before := sourceSnapshot(t, map[string]string{
		"/repo/module-info.java": "package app; class App {}",
	})
	after := sourceSnapshot(t, map[string]string{
		"/repo/module-info.java": "module sample { requires dependency; }",
	})
	if !after.Complete || len(after.View.Graph.Packages) != 0 || len(after.View.Graph.Diagnostics) != 1 || after.View.Graph.Diagnostics[0].Code != "unsupported-module-declaration" {
		t.Fatalf("unsupported module did not retain incomplete source evidence: %#v", after)
	}
	for _, pair := range [][2]Snapshot{{before, after}, {after, before}} {
		result := Compare(pair[0], pair[1])
		if len(result.Packages) != 1 || result.Packages[0].Kind != Uncertain {
			t.Fatalf("unsupported source proved package existence change: %#v", result.Packages)
		}
	}
}

func TestEvidenceDeltaDeterministicAcrossInputOrder(t *testing.T) {
	first := testImport("/repo/A.java", "dep.Target", 2)
	second := testImport("/repo/Z.java", "dep.Other", 7)
	beforeDep := testDependency(second, first)
	relation := testRelation("second", 9)
	relation.Evidence = append(relation.Evidence, testRelation("first", 3).Evidence[0])
	beforeDep.TypeRelations = []relations.Relation{relation}
	before := testSnapshot(beforeDep)
	after := testSnapshot(nil)
	original := onlyDependency(t, Compare(before, after), Removed).RemovedEvidence

	reorderedDep := testDependency(first, second)
	reorderedRelation := testRelation("first", 3)
	reorderedRelation.Evidence = append(reorderedRelation.Evidence, testRelation("second", 9).Evidence[0])
	reorderedDep.TypeRelations = []relations.Relation{reorderedRelation}
	reordered := testSnapshot(reorderedDep)
	slices.Reverse(reordered.View.Graph.Packages)
	actual := onlyDependency(t, Compare(reordered, after), Removed).RemovedEvidence
	if !reflect.DeepEqual(original, actual) {
		t.Fatalf("evidence delta depends on input order:\noriginal=%#v\nactual=%#v", original, actual)
	}
}

func TestPackageFileMembershipIgnoresOrderButRetainsFileRenameEvidence(t *testing.T) {
	before := testSnapshot(testDependency(testImport("/repo/App.java", "dep.Target", 2)))
	before.View.Graph.Packages[0].Files = append(before.View.Graph.Packages[0].Files, packagegraph.File{Path: "/repo/Other.java"})
	after := testSnapshot(testDependency(testImport("/repo/App.java", "dep.Target", 2)))
	after.View.Graph.Packages[0].Files = []packagegraph.File{{Path: "/repo/Other.java", ContentHash: "changed"}, {Path: "/repo/App.java", ContentHash: "changed"}}
	if got := Compare(before, after).Packages[0].Kind; got != Unchanged {
		t.Fatalf("file ordering/hash changed membership: %s", got)
	}
	after.View.Graph.Packages[0].Files[1].Path = "/repo/Renamed.java"
	after.View.Graph.Dependencies[0].Imports[0].File.Path = "/repo/Renamed.java"
	result := Compare(before, after)
	if len(result.Packages) != 2 || result.Packages[0].Kind != Modified {
		t.Fatal("same-package file rename changed package identity")
	}
	change := onlyDependency(t, result, Modified)
	if len(change.AddedEvidence) != 1 || len(change.RemovedEvidence) != 1 {
		t.Fatal("file rename lost semantic source-path delta")
	}
}
