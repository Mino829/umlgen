package packagegraph

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Mino829/umlgen/internal/java"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

func aggregateJava(t *testing.T, context, path, text string) Source {
	t.Helper()
	unit, err := java.ParseSource(path, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return Source{Context: context, Unit: unit}
}

func TestDependencyCountingUnitsAndDeclarationEvidence(t *testing.T) {
	app := aggregateJava(t, "root", "/src/App.java", `package app;
import dep.Target;
import unused.Only;
import app.Local;
class Service extends dep.Base implements dep.Contract {
 Target first;
 Target second;
 Local local;
 Target fetch(Target input) { return null; }
 Target fetch(Target input, int count) { return null; }
}
class Other { Target third; }
class Local {}`)
	sources := []Source{
		app,
		aggregateJava(t, "root", "/src/Dep.java", "package dep; class Target {} class Base {} interface Contract {}"),
		aggregateJava(t, "root", "/src/Unused.java", "package unused; class Only {}"),
	}
	for _, mode := range []DependencySource{DeclaredTypes, Imports, Both} {
		t.Run(string(mode), func(t *testing.T) {
			graph, err := Build(sources, mode)
			if err != nil {
				t.Fatal(err)
			}
			wantEdges := 2
			if mode == DeclaredTypes {
				wantEdges = 1
			}
			if len(graph.Dependencies) != wantEdges {
				t.Fatalf("dependencies=%#v", graph.Dependencies)
			}
			var dep *Dependency
			for i := range graph.Dependencies {
				edge := &graph.Dependencies[i]
				if edge.From == edge.To {
					t.Fatal("same-package dependency leaked")
				}
				if edge.To.Name == "dep" {
					dep = edge
				}
				if edge.To.Name == "unused" && (len(edge.TypeRelations) != 0 || len(edge.Imports) != 1) {
					t.Fatalf("unused import confused with type use: %#v", edge)
				}
			}
			if dep == nil {
				t.Fatal("missing dependency on dep")
			}
			wantTypes, wantImports := 7, 1
			if mode == Imports {
				wantTypes = 0
			}
			if mode == DeclaredTypes {
				wantImports = 0
			}
			if len(dep.TypeRelations) != wantTypes || len(dep.Imports) != wantImports {
				t.Fatalf("type relation and import counts must remain separate: %#v", dep)
			}
			if wantImports != 0 {
				evidence := dep.Imports[0]
				if evidence.File.Path != app.Unit.Path || evidence.File.ContentHash != app.Unit.ContentHash ||
					evidence.Import.Span != app.Unit.Imports[0].Span {
					t.Fatalf("import lost its source snapshot/span: %#v", evidence)
				}
			}
			if wantTypes != 0 {
				kinds := make(map[relations.Kind]int)
				for _, relation := range dep.TypeRelations {
					kinds[relation.Kind]++
					wantEvidence := 1
					if relation.Kind == relations.Parameter || relation.Kind == relations.Return {
						wantEvidence = 2
					}
					if len(relation.Evidence) != wantEvidence {
						t.Fatalf("overload evidence lost: %#v", relation)
					}
					for _, evidence := range relation.Evidence {
						if evidence.Source != app.Unit.Path || evidence.Line < 5 || evidence.Line > 12 {
							t.Fatalf("invalid declaration location: %#v", evidence)
						}
					}
				}
				want := map[relations.Kind]int{relations.Inheritance: 1, relations.Implementation: 1, relations.Field: 3, relations.Parameter: 1, relations.Return: 1}
				if !reflect.DeepEqual(kinds, want) {
					t.Fatalf("relation kinds=%v want=%v", kinds, want)
				}
			}
		})
	}
}

func TestRepeatedSourcesAndInputOrderDoNotChangeCounts(t *testing.T) {
	sources := []Source{
		aggregateJava(t, "root", "/src/A.java", "package a; import b.B; class A { b.B first; b.B second; Unknown missing; }"),
		aggregateJava(t, "root", "/src/B.java", "package b; class B {}"),
		aggregateJava(t, "other-root", "/other/A.java", "package a; class A {}"),
	}
	want, err := Build(sources, Both)
	if err != nil {
		t.Fatal(err)
	}
	if len(want.Dependencies) != 1 || len(want.Dependencies[0].TypeRelations) != 2 || len(want.Dependencies[0].Imports) != 1 || len(want.Diagnostics) != 1 {
		t.Fatalf("unexpected initial aggregation: %#v", want)
	}
	reversed := slices.Clone(sources)
	slices.Reverse(reversed)
	reversed = append(reversed, sources[0])
	got, err := Build(reversed, Both)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("input order/repeated scan changed graph:\n got %#v\nwant %#v", got, want)
	}
}

func TestUnresolvedOwnersAtSamePositionRemainDistinct(t *testing.T) {
	source := aggregateJava(t, "root", "/src/Types.java", "package app; class A { Missing value; } class B { Missing value; }")
	graph, err := Build([]Source{source}, DeclaredTypes)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Dependencies) != 0 || len(graph.Diagnostics) != 2 {
		t.Fatalf("lost unresolved declarations: %#v", graph)
	}
	owners := []string{graph.Diagnostics[0].Owner, graph.Diagnostics[1].Owner}
	slices.Sort(owners)
	if !reflect.DeepEqual(owners, []string{"app.A", "app.B"}) {
		t.Fatalf("owners=%v", owners)
	}
}

func TestFailedAndUnsupportedSourcesAreNotEmptyPackages(t *testing.T) {
	failed, err := java.ParseSource("/src/Broken.java", []byte("package broken; class Broken {"))
	if err == nil {
		t.Fatal("invalid fixture was accepted")
	}
	module := aggregateJava(t, "root", "/src/module-info.java", "module sample { requires java.base; }")
	valid := aggregateJava(t, "root", "/src/package-info.java", "package valid;")
	graph, err := Build([]Source{{Context: "root", Unit: failed}, module, valid}, Both)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Packages) != 1 || graph.Packages[0].ID.Name != "valid" || len(graph.Dependencies) != 0 {
		t.Fatalf("failed/module sources invented packages: %#v", graph)
	}
	codes := make([]string, 0, len(graph.Diagnostics))
	for _, diagnostic := range graph.Diagnostics {
		codes = append(codes, diagnostic.Code)
	}
	slices.Sort(codes)
	if !reflect.DeepEqual(codes, []string{"syntax-error", "unsupported-module-declaration"}) {
		t.Fatalf("lost source limitations: %v", codes)
	}
}

func TestGraphRejectsConflictingSnapshotsAndMissingScope(t *testing.T) {
	first := aggregateJava(t, "root", "/src/A.java", "package a; class A {}")
	changed := aggregateJava(t, "root", "/src/A.java", "package a; class A { int added; }")
	goUnit := model.NewSourceUnit("go", "/src/a.go", []byte("package a"))
	goUnit.Package.Name = "a"
	for _, test := range []struct {
		name    string
		sources []Source
		mode    DependencySource
	}{
		{"conflicting snapshots", []Source{first, changed}, Both},
		{"missing context", []Source{{Unit: first.Unit}}, Both},
		{"unbound Go", []Source{{Context: "root", Unit: goUnit}}, DeclaredTypes},
		{"mixed languages", []Source{first, {Context: "root", Unit: goUnit}}, Both},
		{"unknown dependency source", []Source{first}, "uses"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Build(test.sources, test.mode); err == nil {
				t.Fatal("ambiguous input must not produce a successful graph")
			}
		})
	}
}
