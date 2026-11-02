package packagegraph

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Mino829/umlgen/internal/relations"
)

func cycleTestID(name string) PackageID {
	return PackageID{Language: "java", Context: "root", Name: name}
}

func cycleTestEdge(from, to string, inferred bool) Dependency {
	return Dependency{
		From: cycleTestID(from), To: cycleTestID(to),
		TypeRelations: []relations.Relation{{
			Kind:     relations.Field,
			Evidence: []relations.Evidence{{Source: from + ".java", Line: 3, Inferred: inferred}},
		}},
	}
}

func cycleTestGraph(names []string, dependencies ...Dependency) Graph {
	graph := Graph{Source: Both, Dependencies: dependencies}
	for _, name := range names {
		graph.Packages = append(graph.Packages, Package{ID: cycleTestID(name), Kind: InternalPackage})
	}
	return graph
}

func cycleTestGroup(kind CycleKind, names ...string) Cycle {
	cycle := Cycle{Kind: kind}
	for _, name := range names {
		cycle.Packages = append(cycle.Packages, cycleTestID(name))
	}
	return cycle
}

func TestFindCyclesClassifiesEvidenceAndComponents(t *testing.T) {
	mixedProof := cycleTestEdge("b", "a", true)
	mixedProof.TypeRelations[0].Evidence = append(mixedProof.TypeRelations[0].Evidence, relations.Evidence{Inferred: false})
	mixedRecords := cycleTestEdge("b", "a", true)
	mixedRecords.TypeRelations = append(mixedRecords.TypeRelations, relations.Relation{Kind: relations.Return})
	importProof := cycleTestEdge("b", "a", true)
	importProof.Imports = []ImportEvidence{{File: File{Path: "b.java", ContentHash: "snapshot"}}}
	ordinaryRecord := Dependency{From: cycleTestID("b"), To: cycleTestID("a"), TypeRelations: []relations.Relation{{Kind: relations.Field}}}
	tests := []struct {
		name  string
		graph Graph
		want  []Cycle
	}{
		{name: "empty"},
		{
			name: "DAG with isolate",
			graph: cycleTestGraph([]string{"a", "b", "c", "d", "isolate"},
				cycleTestEdge("a", "b", false), cycleTestEdge("a", "c", true),
				cycleTestEdge("b", "d", false), cycleTestEdge("c", "d", false)),
		},
		{
			name: "two node declared cycle",
			graph: cycleTestGraph([]string{"a", "b"},
				cycleTestEdge("a", "b", false), cycleTestEdge("b", "a", false)),
			want: []Cycle{cycleTestGroup(DeclaredCycle, "a", "b")},
		},
		{
			name: "disconnected SCCs with one way bridge and isolate",
			graph: cycleTestGraph([]string{"a", "b", "c", "d", "e", "isolate"},
				cycleTestEdge("a", "b", false), cycleTestEdge("b", "a", false),
				cycleTestEdge("b", "c", false), cycleTestEdge("c", "d", false),
				cycleTestEdge("d", "e", false), cycleTestEdge("e", "c", false)),
			want: []Cycle{cycleTestGroup(DeclaredCycle, "a", "b"), cycleTestGroup(DeclaredCycle, "c", "d", "e")},
		},
		{
			name: "self edges are not cycles",
			graph: cycleTestGraph([]string{"a", "b", "isolate"},
				cycleTestEdge("a", "a", false), cycleTestEdge("b", "b", true), cycleTestEdge("a", "b", false)),
		},
		{
			name: "inferred closing edge",
			graph: cycleTestGraph([]string{"a", "b"},
				cycleTestEdge("a", "b", false), cycleTestEdge("b", "a", true)),
			want: []Cycle{cycleTestGroup(CandidateCycle, "a", "b")},
		},
		{
			name: "entirely inferred cycle",
			graph: cycleTestGraph([]string{"a", "b"},
				cycleTestEdge("a", "b", true), cycleTestEdge("b", "a", true)),
			want: []Cycle{cycleTestGroup(CandidateCycle, "a", "b")},
		},
		{
			name:  "mixed evidence on one relation is declared",
			graph: cycleTestGraph([]string{"a", "b"}, cycleTestEdge("a", "b", false), mixedProof),
			want:  []Cycle{cycleTestGroup(DeclaredCycle, "a", "b")},
		},
		{
			name:  "ordinary relation independently establishes declared edge",
			graph: cycleTestGraph([]string{"a", "b"}, cycleTestEdge("a", "b", false), mixedRecords),
			want:  []Cycle{cycleTestGroup(DeclaredCycle, "a", "b")},
		},
		{
			name:  "import independently establishes declared edge",
			graph: cycleTestGraph([]string{"a", "b"}, cycleTestEdge("a", "b", false), importProof),
			want:  []Cycle{cycleTestGroup(DeclaredCycle, "a", "b")},
		},
		{
			name:  "empty relation evidence is ordinary",
			graph: cycleTestGraph([]string{"a", "b"}, cycleTestEdge("a", "b", false), ordinaryRecord),
			want:  []Cycle{cycleTestGroup(DeclaredCycle, "a", "b")},
		},
		{
			name: "evidence free dependency still participates in full graph",
			graph: cycleTestGraph([]string{"a", "b"}, cycleTestEdge("a", "b", false),
				Dependency{From: cycleTestID("b"), To: cycleTestID("a")}),
			want: []Cycle{cycleTestGroup(CandidateCycle, "a", "b")},
		},
		{
			name: "declared subgroup survives inferred expansion",
			graph: cycleTestGraph([]string{"a", "b", "c"},
				cycleTestEdge("a", "b", false), cycleTestEdge("b", "a", false),
				cycleTestEdge("b", "c", true), cycleTestEdge("c", "a", false)),
			want: []Cycle{cycleTestGroup(DeclaredCycle, "a", "b"), cycleTestGroup(CandidateCycle, "a", "b", "c")},
		},
		{
			name: "multiple declared subgroups inside candidate",
			graph: cycleTestGraph([]string{"a", "b", "c", "d"},
				cycleTestEdge("a", "b", false), cycleTestEdge("b", "a", false),
				cycleTestEdge("c", "d", false), cycleTestEdge("d", "c", false),
				cycleTestEdge("b", "c", true), cycleTestEdge("d", "a", true)),
			want: []Cycle{cycleTestGroup(DeclaredCycle, "a", "b"), cycleTestGroup(DeclaredCycle, "c", "d"), cycleTestGroup(CandidateCycle, "a", "b", "c", "d")},
		},
		{
			name: "redundant inferred edge creates no duplicate candidate",
			graph: cycleTestGraph([]string{"a", "b", "c"},
				cycleTestEdge("a", "b", false), cycleTestEdge("b", "c", false),
				cycleTestEdge("c", "a", false), cycleTestEdge("a", "c", true)),
			want: []Cycle{cycleTestGroup(DeclaredCycle, "a", "b", "c")},
		},
		{
			name: "unknown endpoints cannot manufacture vertices",
			graph: cycleTestGraph([]string{"a"},
				cycleTestEdge("a", "missing", false), cycleTestEdge("missing", "a", false)),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := findCycles(test.graph); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("cycles=%#v, want %#v", got, test.want)
			}
		})
	}
}

func TestCycleOrderIdentityAndInputPreservation(t *testing.T) {
	makeGraph := func() Graph {
		graph := cycleTestGraph([]string{"c", "b", "a", "isolate"},
			cycleTestEdge("b", "c", true), cycleTestEdge("c", "a", false),
			cycleTestEdge("b", "a", false), cycleTestEdge("a", "b", false))
		otherRoot := PackageID{Language: "java", Context: "other-root", Name: "a"}
		otherLanguage := PackageID{Language: "go", Context: "root", Name: "a"}
		graph.Packages = append(graph.Packages, Package{ID: otherRoot}, Package{ID: otherLanguage})
		graph.Dependencies = append(graph.Dependencies,
			Dependency{From: otherRoot, To: otherLanguage, Imports: []ImportEvidence{{File: File{Path: "import.java"}}}},
			Dependency{From: otherLanguage, To: otherRoot, TypeRelations: []relations.Relation{{Kind: relations.Return}}})
		return graph
	}
	graph := makeGraph()
	before := makeGraph()
	want := []Cycle{
		{Kind: DeclaredCycle, Packages: []PackageID{
			{Language: "go", Context: "root", Name: "a"},
			{Language: "java", Context: "other-root", Name: "a"},
		}},
		cycleTestGroup(DeclaredCycle, "a", "b"),
		cycleTestGroup(CandidateCycle, "a", "b", "c"),
	}
	got := findCycles(graph)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("root/language identities or cycle ordering lost: got %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(graph, before) {
		t.Fatal("cycle analysis changed input graph or nested evidence")
	}
	for shift := range graph.Packages {
		reordered := makeGraph()
		slices.Reverse(reordered.Dependencies)
		reordered.Packages = append(reordered.Packages[shift:], reordered.Packages[:shift]...)
		if result := findCycles(reordered); !reflect.DeepEqual(result, want) {
			t.Fatalf("input order changed cycles at rotation %d: %#v", shift, result)
		}
	}
	got[0].Packages[0].Name = "changed"
	if !reflect.DeepEqual(graph, before) {
		t.Fatal("returned cycle package storage aliases the input graph")
	}
}

func TestCycleClassificationFromParsedImportsAndDeclarations(t *testing.T) {
	sources := []Source{
		aggregateJava(t, "root", "/src/A.java", "package a; import b.B; class A {}"),
		aggregateJava(t, "root", "/src/B.java", "package b; class B { a.A value; }"),
	}
	for _, source := range []DependencySource{DeclaredTypes, Imports, Both} {
		t.Run(string(source), func(t *testing.T) {
			graph, err := Build(sources, source)
			if err != nil {
				t.Fatal(err)
			}
			view, err := Analyze(graph, ViewOptions{ShowExternal: true})
			if err != nil {
				t.Fatal(err)
			}
			var want []Cycle
			if source == Both {
				want = []Cycle{cycleTestGroup(DeclaredCycle, "a", "b")}
			}
			if !reflect.DeepEqual(view.Cycles, want) {
				t.Fatalf("selected source %q: cycles=%#v, want %#v", source, view.Cycles, want)
			}
		})
	}
}

func TestStronglyConnectedPackagesLongChain(t *testing.T) {
	const count = 100000
	edges := make([][]int, count)
	for vertex := range edges[:count-1] {
		edges[vertex] = []int{vertex + 1}
	}
	components := stronglyConnectedPackages(edges)
	if len(components.sizes) != count {
		t.Fatalf("chain has %d components, want %d", len(components.sizes), count)
	}
	seen := make([]bool, count)
	for vertex, label := range components.labels {
		if label < 0 || label >= count || seen[label] || components.sizes[label] != 1 {
			t.Fatalf("chain vertex %d has invalid singleton component %d", vertex, label)
		}
		seen[label] = true
	}
	edges[count-1] = []int{0}
	components = stronglyConnectedPackages(edges)
	if len(components.sizes) != 1 || components.sizes[0] != count {
		t.Fatalf("closed chain did not form one SCC: sizes=%v", components.sizes)
	}
	for vertex, label := range components.labels {
		if label != 0 {
			t.Fatalf("closed chain vertex %d has component %d, want 0", vertex, label)
		}
	}
}
