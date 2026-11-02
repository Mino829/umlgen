package packagegraph

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/Mino829/umlgen/internal/java"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

func focusID(name string) PackageID {
	return PackageID{Language: "java", Context: "root", Name: name}
}

func focusGraph(names ...string) Graph {
	graph := Graph{Source: DeclaredTypes}
	for _, name := range names {
		graph.Packages = append(graph.Packages, Package{ID: focusID(name), Kind: InternalPackage})
	}
	return graph
}

func focusTypeEdge(from, to string, kind relations.Kind) Dependency {
	return Dependency{From: focusID(from), To: focusID(to), TypeRelations: []relations.Relation{{
		From: from + ".Type", To: to + ".Type", Kind: kind,
	}}}
}

func focusImportEdge(from, to string) Dependency {
	return Dependency{From: focusID(from), To: focusID(to), Imports: []ImportEvidence{{
		File: File{Path: from + ".java"}, Import: model.Import{Name: to + ".Type"},
	}}}
}

func analyzeFocus(t *testing.T, graph Graph, opts ViewOptions) View {
	t.Helper()
	view, err := Analyze(graph, opts)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func expectFocusPackages(t *testing.T, view View, want ...PackageID) {
	t.Helper()
	got := make([]PackageID, 0, len(view.Graph.Packages))
	for _, pkg := range view.Graph.Packages {
		got = append(got, pkg.ID)
	}
	slices.SortFunc(want, comparePackageID)
	if !slices.Equal(got, want) {
		t.Fatalf("visible packages = %v, want %v", got, want)
	}
	visible := make(map[PackageID]bool, len(got))
	for _, id := range got {
		visible[id] = true
	}
	for _, dep := range view.Graph.Dependencies {
		if !visible[dep.From] || !visible[dep.To] {
			t.Fatalf("dependency has a hidden endpoint: %+v", dep)
		}
	}
}

func TestAnalyzeFocusDirectionsAndExactHops(t *testing.T) {
	graph := focusGraph("e", "d", "isolated", "b", "a", "c")
	graph.Dependencies = []Dependency{
		focusTypeEdge("a", "b", relations.Field),
		focusTypeEdge("e", "a", relations.Field),
		focusTypeEdge("b", "c", relations.Field),
		focusTypeEdge("c", "d", relations.Field),
	}
	seed := focusID("b")
	for _, test := range []struct {
		name      string
		direction Direction
		depth     int
		want      []PackageID
		edges     int
	}{
		{"out zero", DirectionOut, 0, []PackageID{seed}, 0},
		{"out one", DirectionOut, 1, []PackageID{seed, focusID("c")}, 1},
		{"out two", DirectionOut, 2, []PackageID{seed, focusID("c"), focusID("d")}, 2},
		{"in one", DirectionIn, 1, []PackageID{focusID("a"), seed}, 1},
		{"in two", DirectionIn, 2, []PackageID{focusID("e"), focusID("a"), seed}, 2},
		{"both zero", DirectionBoth, 0, []PackageID{seed}, 0},
		{"both one", DirectionBoth, 1, []PackageID{focusID("a"), seed, focusID("c")}, 2},
		{"both two", DirectionBoth, 2, []PackageID{focusID("e"), focusID("a"), seed, focusID("c"), focusID("d")}, 4},
		{"default both", "", 1, []PackageID{focusID("a"), seed, focusID("c")}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := analyzeFocus(t, graph, ViewOptions{Focus: &seed, Depth: test.depth, Direction: test.direction})
			expectFocusPackages(t, view, test.want...)
			if len(view.Graph.Dependencies) != test.edges {
				t.Fatalf("visible dependency count = %d, want %d", len(view.Graph.Dependencies), test.edges)
			}
			if view.Scope.PackageCount != 6 || view.Scope.DependencyCount != 4 || view.Scope.Focus == nil || *view.Scope.Focus != seed || view.Scope.Depth != test.depth || view.Scope.Cycles != CyclesMark {
				t.Fatalf("selected scope changed with visibility: %+v", view.Scope)
			}
		})
	}
	unfocused := analyzeFocus(t, graph, ViewOptions{Depth: 0})
	expectFocusPackages(t, unfocused, focusID("a"), focusID("b"), focusID("c"), focusID("d"), focusID("e"), focusID("isolated"))
	isolated := focusID("isolated")
	expectFocusPackages(t, analyzeFocus(t, graph, ViewOptions{Focus: &isolated, Depth: 10}), isolated)
}

func TestAnalyzeFocusUsesFullPackageIdentity(t *testing.T) {
	first := PackageID{Language: "java", Context: "first", Name: "same"}
	second := PackageID{Language: "java", Context: "second", Name: "same"}
	left, right := focusID("left"), focusID("right")
	graph := Graph{Source: Imports, Packages: []Package{
		{ID: first, Kind: InternalPackage}, {ID: second, Kind: InternalPackage},
		{ID: left, Kind: InternalPackage}, {ID: right, Kind: InternalPackage},
	}, Dependencies: []Dependency{
		{From: first, To: left, Imports: []ImportEvidence{{Import: model.Import{Name: "left.Type"}}}},
		{From: second, To: right, Imports: []ImportEvidence{{Import: model.Import{Name: "right.Type"}}}},
	}}
	expectFocusPackages(t, analyzeFocus(t, graph, ViewOptions{Focus: &second, Depth: 1}), second, right)
	missing := PackageID{Language: "java", Context: "absent", Name: "same"}
	if _, err := Analyze(graph, ViewOptions{Focus: &missing}); err == nil {
		t.Fatal("same display name in another context must not satisfy focus")
	}
}

func TestAnalyzeRelationSelectionChangesReachabilityAndCycles(t *testing.T) {
	sources := []Source{
		aggregateJava(t, "root", "A.java", `package a; class A {
 b.B field; missing.Field absent;
 void use(c.C value, missing.Parameter absent) {}
}`),
		aggregateJava(t, "root", "B.java", "package b; class B { a.A field; }"),
		aggregateJava(t, "root", "C.java", "package c; class C {}"),
	}
	graph, err := Build(sources, DeclaredTypes)
	if err != nil {
		t.Fatal(err)
	}
	seed := focusID("a")
	for _, test := range []struct {
		name        string
		kinds       []relations.Kind
		want        []PackageID
		edges       int
		cycles      int
		diagnostics []relations.Kind
	}{
		{"all", nil, []PackageID{seed, focusID("b"), focusID("c")}, 3, 1, []relations.Kind{relations.Field, relations.Parameter}},
		{"field", []relations.Kind{relations.Field}, []PackageID{seed, focusID("b")}, 2, 1, []relations.Kind{relations.Field}},
		{"parameter", []relations.Kind{relations.Parameter}, []PackageID{seed, focusID("c")}, 1, 0, []relations.Kind{relations.Parameter}},
		{"none", []relations.Kind{}, []PackageID{seed}, 0, 0, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := analyzeFocus(t, graph, ViewOptions{Relations: test.kinds, Focus: &seed, Depth: 2, Direction: DirectionOut})
			expectFocusPackages(t, view, test.want...)
			if view.Scope.Source != DeclaredTypes || view.Scope.PackageCount != 3 || view.Scope.DependencyCount != test.edges || len(view.Cycles) != test.cycles {
				t.Fatalf("selected scope/cycles = %+v / %+v", view.Scope, view.Cycles)
			}
			if !reflect.DeepEqual(view.Scope.Relations, test.kinds) {
				t.Fatalf("relation selection = %#v, want %#v", view.Scope.Relations, test.kinds)
			}
			var got []relations.Kind
			for _, diagnostic := range view.Graph.Diagnostics {
				got = append(got, diagnostic.Kind)
			}
			slices.Sort(got)
			if !slices.Equal(got, test.diagnostics) {
				t.Fatalf("diagnostic kinds = %v, want %v", got, test.diagnostics)
			}
			if len(view.Cycles) != 0 && (view.Cycles[0].Kind != DeclaredCycle || !slices.Equal(view.Cycles[0].Packages, []PackageID{seed, focusID("b")})) {
				t.Fatalf("cycle does not follow selected relation evidence: %+v", view.Cycles)
			}
		})
	}
}

func TestAnalyzeBothRetainsImportsAndNonTypeDiagnostics(t *testing.T) {
	failed, parseErr := java.ParseSource("Broken.java", []byte("package broken; class Broken {"))
	if parseErr == nil {
		t.Fatal("invalid fixture was accepted")
	}
	graph, err := Build([]Source{
		aggregateJava(t, "root", "A.java", `package a;
import b.B; import unknown.Missing;
class A { B field; missing.Field absent; void use(missing.Parameter value) {} }`),
		aggregateJava(t, "root", "B.java", "package b; import a.A; class B {}"),
		{Context: "root", Unit: failed},
	}, Both)
	if err != nil {
		t.Fatal(err)
	}
	seed := focusID("a")
	for _, kinds := range [][]relations.Kind{{}, {relations.Parameter}} {
		view := analyzeFocus(t, graph, ViewOptions{Relations: kinds, Focus: &seed, Depth: 1, Direction: DirectionOut})
		expectFocusPackages(t, view, seed, focusID("b"))
		if view.Scope.PackageCount != 2 || view.Scope.DependencyCount != 2 || len(view.Cycles) != 1 || view.Cycles[0].Kind != DeclaredCycle {
			t.Fatalf("import-only cycle was lost under relation filtering: %+v", view)
		}
		for _, dep := range view.Graph.Dependencies {
			if len(dep.Imports) != 1 || len(dep.TypeRelations) != 0 {
				t.Fatalf("import survival/type selection failed: %+v", dep)
			}
		}
		parser, imports, types := 0, 0, 0
		for _, diagnostic := range view.Graph.Diagnostics {
			switch {
			case diagnostic.Code == "syntax-error":
				parser++
			case diagnostic.Kind == "":
				imports++
			default:
				types++
				if diagnostic.Kind != relations.Parameter {
					t.Fatalf("unselected type diagnostic survived: %+v", diagnostic)
				}
			}
		}
		if parser != 1 || imports != 1 || types != len(kinds) {
			t.Fatalf("diagnostics parser/import/type = %d/%d/%d, want 1/1/%d", parser, imports, types, len(kinds))
		}
	}
}

func TestAnalyzeFocusTraversesSelectedGraphBeforeHidingExternalPackages(t *testing.T) {
	graph := focusGraph("seed", "outside", "reachable")
	graph.Packages[1].Kind = ExternalPackage
	graph.Dependencies = []Dependency{
		focusTypeEdge("seed", "outside", relations.Field),
		focusTypeEdge("outside", "reachable", relations.Field),
	}
	seed := focusID("seed")
	view := analyzeFocus(t, graph, ViewOptions{Focus: &seed, Depth: 2, Direction: DirectionOut})
	expectFocusPackages(t, view, seed, focusID("reachable"))
	if len(view.Graph.Dependencies) != 0 || view.Scope.PackageCount != 3 || view.Scope.DependencyCount != 2 {
		t.Fatalf("hidden intermediate changed selected-graph reachability: %+v", view)
	}
}

func TestAnalyzeVisibilityDoesNotReclassifySelectedCycles(t *testing.T) {
	graph := focusGraph("seed", "a", "b", "outside", "isolated")
	graph.Packages[3].Kind = ExternalPackage
	graph.Dependencies = []Dependency{
		focusTypeEdge("seed", "a", relations.Field),
		focusTypeEdge("a", "b", relations.Field),
		focusTypeEdge("b", "a", relations.Field),
		focusTypeEdge("seed", "outside", relations.Field),
		focusTypeEdge("outside", "seed", relations.Field),
	}
	graph.Diagnostics = []Diagnostic{{From: focusID("b"), Kind: relations.Field, Reference: "Missing", Status: Unresolved}}
	seed := focusID("seed")
	full := analyzeFocus(t, graph, ViewOptions{ShowExternal: true})
	if len(full.Cycles) != 2 || full.Scope.PackageCount != 5 || full.Scope.DependencyCount != 5 {
		t.Fatalf("unexpected selected graph: %+v", full)
	}
	for _, mode := range []CycleMode{CyclesOff, CyclesMark, CyclesOnly} {
		view := analyzeFocus(t, graph, ViewOptions{Focus: &seed, Depth: 1, Direction: DirectionOut, Cycles: mode})
		expectFocusPackages(t, view, seed, focusID("a"))
		wantCycles := full.Cycles
		if mode == CyclesOff {
			wantCycles = nil
		}
		if len(view.Graph.Dependencies) != 1 || !reflect.DeepEqual(view.Cycles, wantCycles) || !reflect.DeepEqual(view.Graph.Diagnostics, full.Graph.Diagnostics) {
			t.Fatalf("focus/external visibility changed selected cycles or diagnostics: %+v", view)
		}
		if view.Scope.PackageCount != 5 || view.Scope.DependencyCount != 5 {
			t.Fatalf("scope describes display instead of selected analysis: %+v", view.Scope)
		}
	}
	outside := focusID("outside")
	if _, err := Analyze(graph, ViewOptions{Focus: &outside}); err == nil {
		t.Fatal("hidden external focus must be rejected")
	}
	expectFocusPackages(t, analyzeFocus(t, graph, ViewOptions{Focus: &outside, ShowExternal: true}), outside)
	isolated := focusID("isolated")
	empty := analyzeFocus(t, graph, ViewOptions{Focus: &isolated, Cycles: CyclesOnly})
	expectFocusPackages(t, empty)
	if !reflect.DeepEqual(empty.Cycles, full.Cycles) || !reflect.DeepEqual(empty.Graph.Diagnostics, full.Graph.Diagnostics) {
		t.Fatal("cycles-only empty view lost selected analysis")
	}
}

func TestAnalyzeCyclesOnlyIntersectsFocusAndRetainsHiddenCycles(t *testing.T) {
	graph := focusGraph("seed", "a", "b", "c", "d", "isolate")
	graph.Dependencies = []Dependency{
		focusTypeEdge("seed", "a", relations.Field),
		focusTypeEdge("a", "b", relations.Field),
		focusTypeEdge("b", "a", relations.Field),
		focusTypeEdge("c", "d", relations.Field),
		focusTypeEdge("d", "c", relations.Field),
		focusTypeEdge("isolate", "isolate", relations.Field),
	}
	graph.Diagnostics = []Diagnostic{{From: focusID("isolate"), Kind: relations.Field, Reference: "Missing", Status: Unresolved}}
	seed := focusID("seed")
	view := analyzeFocus(t, graph, ViewOptions{Focus: &seed, Depth: 1, Direction: DirectionOut, Cycles: CyclesOnly})
	expectFocusPackages(t, view, focusID("a"))
	if len(view.Graph.Dependencies) != 0 || len(view.Cycles) != 2 || view.Scope.PackageCount != 6 || view.Scope.DependencyCount != 5 {
		t.Fatalf("cycles-only must intersect rather than expand focus: %+v", view)
	}
	unfocused := analyzeFocus(t, graph, ViewOptions{Cycles: CyclesOnly})
	expectFocusPackages(t, unfocused, focusID("a"), focusID("b"), focusID("c"), focusID("d"))
	all := analyzeFocus(t, graph, ViewOptions{Cycles: CyclesOff})
	if len(all.Graph.Dependencies) != 5 || len(all.Graph.Diagnostics) != 1 {
		t.Fatalf("self dependency reached display or its diagnostic was lost: %+v", all)
	}
	for _, dep := range all.Graph.Dependencies {
		if dep.From == dep.To {
			t.Fatalf("same-package dependency reached display: %+v", dep)
		}
	}
}

func TestAnalyzeFailedSourcesRemainDiagnosticsWithoutPackages(t *testing.T) {
	failed, err := java.ParseSource("Broken.java", []byte("class Broken {"))
	if err == nil {
		t.Fatal("invalid fixture was accepted")
	}
	graph, err := Build([]Source{{Context: "root", Unit: failed}}, Both)
	if err != nil {
		t.Fatal(err)
	}
	view := analyzeFocus(t, graph, ViewOptions{Relations: []relations.Kind{}, Cycles: CyclesOnly})
	expectFocusPackages(t, view)
	if view.Scope.PackageCount != 0 || view.Scope.DependencyCount != 0 || len(view.Cycles) != 0 || len(view.Graph.Diagnostics) != 1 || view.Graph.Diagnostics[0].Code != "syntax-error" {
		t.Fatalf("failed source became an empty successful package: %+v", view)
	}
	unnamed := focusGraph("")
	expectFocusPackages(t, analyzeFocus(t, unnamed, ViewOptions{}), focusID(""))
}

func TestAnalyzeRejectsInvalidOptionsAndGraphBoundaries(t *testing.T) {
	valid := focusGraph("a", "b")
	missing := focusID("missing")
	for _, test := range []struct {
		name string
		opts ViewOptions
	}{
		{"negative depth without focus", ViewOptions{Depth: -1}},
		{"direction", ViewOptions{Direction: "sideways"}},
		{"cycles", ViewOptions{Cycles: "maybe"}},
		{"relation", ViewOptions{Relations: []relations.Kind{"calls"}}},
		{"missing focus", ViewOptions{Focus: &missing}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Analyze(valid, test.opts); err == nil {
				t.Fatal("invalid selection was accepted")
			}
		})
	}
	imports := valid
	imports.Source = Imports
	for _, kinds := range [][]relations.Kind{{}, {relations.Field}} {
		if _, err := Analyze(imports, ViewOptions{Relations: kinds}); err == nil {
			t.Fatal("explicit type selection was accepted on imports-only graph")
		}
	}
	analyzeFocus(t, imports, ViewOptions{})
	for _, test := range []struct {
		name   string
		change func(*Graph)
	}{
		{"source", func(g *Graph) { g.Source = "compiler" }},
		{"package kind", func(g *Graph) { g.Packages[0].Kind = "unknown" }},
		{"missing language", func(g *Graph) { g.Packages[0].ID.Language = "" }},
		{"unknown language", func(g *Graph) { g.Packages[0].ID.Language = "rust" }},
		{"missing context", func(g *Graph) { g.Packages[0].ID.Context = "" }},
		{"missing Go path", func(g *Graph) { g.Packages[0].ID.Language = "go"; g.Packages[0].ID.Name = "" }},
		{"duplicate identity", func(g *Graph) { g.Packages = append(g.Packages, g.Packages[0]) }},
		{"absent from endpoint", func(g *Graph) { g.Dependencies = []Dependency{focusTypeEdge("missing", "a", relations.Field)} }},
		{"absent to endpoint", func(g *Graph) { g.Dependencies = []Dependency{focusTypeEdge("a", "missing", relations.Field)} }},
		{"relation kind", func(g *Graph) { g.Dependencies = []Dependency{focusTypeEdge("a", "b", "calls")} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := focusGraph("a", "b")
			test.change(&graph)
			// Even edges discarded by relation selection must not introduce
			// phantom vertices or conceal a malformed graph boundary.
			if _, err := Analyze(graph, ViewOptions{Relations: []relations.Kind{}}); err == nil {
				t.Fatal("invalid graph was accepted")
			}
		})
	}
}

func focusSnapshot(t *testing.T, graph Graph) []byte {
	t.Helper()
	data, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAnalyzeDeterministicAndDoesNotMutateInputs(t *testing.T) {
	graph := focusGraph("b", "a", "isolated")
	graph.Packages[1].Files = []File{{Path: "z.java", ContentHash: "z"}, {Path: "a.java", ContentHash: "a"}}
	forward := focusTypeEdge("a", "b", relations.Parameter)
	forward.TypeRelations[0].Evidence = []relations.Evidence{{Source: "z.java", Member: "z"}, {Source: "a.java", Member: "a"}}
	forward.TypeRelations = append(forward.TypeRelations, focusTypeEdge("a", "b", relations.Field).TypeRelations...)
	forward.Imports = []ImportEvidence{{File: File{Path: "z.java"}, Import: model.Import{Name: "b.Z"}}, {File: File{Path: "a.java"}, Import: model.Import{Name: "b.A"}}}
	graph.Dependencies = []Dependency{focusTypeEdge("b", "a", relations.Field), forward}
	graph.Diagnostics = []Diagnostic{
		{From: focusID("b"), Kind: relations.Field, Reference: "Z", Candidates: []Target{{Package: focusID("b"), Type: "Z"}, {Package: focusID("a"), Type: "A"}}},
		{From: focusID("a"), Reference: "A", Code: "syntax-error"},
	}
	before := focusSnapshot(t, graph)
	seed := focusID("a")
	kinds := []relations.Kind{relations.Parameter, relations.Field, relations.Parameter}
	opts := ViewOptions{Relations: kinds, Focus: &seed, Depth: 2}
	view := analyzeFocus(t, graph, opts)
	if !slices.Equal(before, focusSnapshot(t, graph)) || !slices.Equal(kinds, []relations.Kind{relations.Parameter, relations.Field, relations.Parameter}) || seed != focusID("a") {
		t.Fatal("analysis mutated graph or caller options")
	}
	if !slices.Equal(view.Scope.Relations, []relations.Kind{relations.Field, relations.Parameter}) {
		t.Fatalf("scope relation kinds are not canonical: %v", view.Scope.Relations)
	}

	// Rebuild every nested slice in reversed order; sorting must concern owned
	// data, and must not let input traversal order determine the resulting view.
	var reversed Graph
	if err := json.Unmarshal(before, &reversed); err != nil {
		t.Fatal(err)
	}
	slices.Reverse(reversed.Packages)
	for i := range reversed.Packages {
		slices.Reverse(reversed.Packages[i].Files)
	}
	slices.Reverse(reversed.Dependencies)
	for i := range reversed.Dependencies {
		slices.Reverse(reversed.Dependencies[i].TypeRelations)
		slices.Reverse(reversed.Dependencies[i].Imports)
		for j := range reversed.Dependencies[i].TypeRelations {
			slices.Reverse(reversed.Dependencies[i].TypeRelations[j].Evidence)
		}
	}
	slices.Reverse(reversed.Diagnostics)
	for i := range reversed.Diagnostics {
		slices.Reverse(reversed.Diagnostics[i].Candidates)
	}
	other := analyzeFocus(t, reversed, opts)
	if !reflect.DeepEqual(view, other) {
		t.Fatalf("input order changed analysis:\n%+v\n%+v", view, other)
	}
}
