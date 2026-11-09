package packagegraph

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Mino829/umlgen/internal/java"
)

func javaSource(t *testing.T, context, path, text string) Source {
	t.Helper()
	unit, err := java.ParseSource(path, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return Source{Context: context, Unit: unit}
}

func javaBuild(t *testing.T, sources []Source, mode DependencySource) Graph {
	t.Helper()
	graph, err := Build(sources, mode)
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

func javaDependency(t *testing.T, graph Graph, from, to PackageID) Dependency {
	t.Helper()
	for _, dependency := range graph.Dependencies {
		if dependency.From == from && dependency.To == to {
			return dependency
		}
	}
	t.Fatalf("missing dependency %v -> %v: %+v", from, to, graph)
	return Dependency{}
}

func TestJavaImportOwnership(t *testing.T) {
	tests := []struct {
		name        string
		declaration string
		field       string
	}{
		{"nested type", "import dep.Outer.Inner;", "Inner"},
		{"static nested type", "import static dep.Outer.Inner;", "Inner"},
		{"static member", "import static dep.Outer.VALUE;", "int"},
		{"static wildcard", "import static dep.Outer.*;", "Inner"},
		{"type wildcard", "import dep.Outer.*;", "Inner"},
		{"package wildcard", "import dep.*;", "Outer"},
		{"commented spelling", "import dep /* namespace */ . Outer /* owner */ . Inner;", "Inner"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := javaSource(t, "root", "App.java", "package app;\n"+test.declaration+"\nclass App { "+test.field+" value; }")
			dep := javaSource(t, "root", "Outer.java", "package dep; class Outer { static int VALUE; static class Inner {} }")
			graph := javaBuild(t, []Source{app, dep}, Both)
			edge := javaDependency(t, graph, packageID(app), packageID(dep))
			if len(graph.Dependencies) != 1 || len(edge.Imports) != 1 {
				t.Fatalf("dependencies = %+v", graph.Dependencies)
			}
			if got := edge.Imports[0]; got.File.Path != "App.java" || got.Import.Span.Start.Line != 2 {
				t.Fatalf("import evidence = %+v", got)
			}
			wantRelations := 1
			if test.field == "int" {
				wantRelations = 0
			}
			if len(edge.TypeRelations) != wantRelations {
				t.Fatalf("type relations = %+v", edge.TypeRelations)
			}
		})
	}
}

func TestJavaUnknownOwnerDoesNotInventPackage(t *testing.T) {
	for _, declaration := range []string{
		"import missing.Outer.Inner;",
		"import static missing.Outer.VALUE;",
		"import static missing.Outer.*;",
		"import missing.Outer.*;",
	} {
		t.Run(declaration, func(t *testing.T) {
			app := javaSource(t, "root", "App.java", "package app; "+declaration+" class App {}")
			graph := javaBuild(t, []Source{app}, Imports)
			if len(graph.Packages) != 1 || len(graph.Dependencies) != 0 {
				t.Fatalf("invented external ownership: %+v", graph)
			}
			if len(graph.Diagnostics) != 1 || graph.Diagnostics[0].Status != Unresolved {
				t.Fatalf("diagnostics = %+v", graph.Diagnostics)
			}
		})
	}
}

func TestJavaWildcardTypeAmbiguity(t *testing.T) {
	app := javaSource(t, "root", "App.java", "package app; import one.*; import two.*; class App { Item value; }")
	one := javaSource(t, "root", "One.java", "package one; class Item {}")
	two := javaSource(t, "root", "Two.java", "package two; class Item {}")
	graph := javaBuild(t, []Source{app, one, two}, DeclaredTypes)
	if len(graph.Dependencies) != 0 {
		t.Fatalf("ambiguous type produced edge: %+v", graph.Dependencies)
	}
	if len(graph.Diagnostics) != 1 || graph.Diagnostics[0].Status != Ambiguous || len(graph.Diagnostics[0].Candidates) != 2 {
		t.Fatalf("diagnostics = %+v", graph.Diagnostics)
	}
}

func TestJavaPackageAndTypeWildcardPrefixAmbiguity(t *testing.T) {
	app := javaSource(t, "root", "App.java", "package app; import dep.Outer.*; class App { Inner value; }")
	owner := javaSource(t, "root", "Outer.java", "package dep; class Outer { static class Inner {} }")
	packageMember := javaSource(t, "root", "Inner.java", "package dep.Outer; class Inner {}")
	graph := javaBuild(t, []Source{app, owner, packageMember}, Both)
	if len(graph.Dependencies) != 0 {
		t.Fatalf("ambiguous owner produced edges: %+v", graph.Dependencies)
	}
	if len(graph.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %+v", graph.Diagnostics)
	}
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Status != Ambiguous || len(diagnostic.Candidates) != 2 {
			t.Fatalf("diagnostic = %+v", diagnostic)
		}
	}
}

func TestJavaStaticWildcardRequiresTypeOwner(t *testing.T) {
	app := javaSource(t, "root", "App.java", "package app; import static dep.Outer.*; class App { Inner value; }")
	owner := javaSource(t, "root", "Outer.java", "package dep; class Outer { static class Inner {} }")
	packageMember := javaSource(t, "root", "Inner.java", "package dep.Outer; class Inner {}")
	graph := javaBuild(t, []Source{app, owner, packageMember}, Both)
	edge := javaDependency(t, graph, packageID(app), packageID(owner))
	if len(graph.Dependencies) != 1 || len(edge.Imports) != 1 || len(edge.TypeRelations) != 1 {
		t.Fatalf("graph = %+v", graph)
	}
}

func TestJavaTypeLookupPrecedence(t *testing.T) {
	tests := []struct {
		name  string
		app   string
		extra string
		want  string
	}{
		{"same package", "package app; import dep.Item; class App { Item value; } class Item {}", "package unused; class Other {}", ""},
		{"enclosing", "package app; import dep.Item; class App { static class Item {} class Child { Item value; } }", "package unused; class Other {}", ""},
		{"explicit before wildcard", "package app; import dep.Item; import other.*; class App { Item value; }", "package other; class Item {}", "dep"},
		{"imported qualifier", "package app; import dep.Outer; class App { Outer.Inner value; }", "package unused; class Other {}", "dep"},
		{"fully qualified without import", "package app; class App { dep.Outer.Inner value; }", "package unused; class Other {}", "dep"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := javaSource(t, "root", "App.java", test.app)
			dep := javaSource(t, "root", "Dep.java", "package dep; class Item {} class Outer { static class Inner {} }")
			extra := javaSource(t, "root", "Extra.java", test.extra)
			graph := javaBuild(t, []Source{app, dep, extra}, DeclaredTypes)
			if test.want == "" {
				if len(graph.Dependencies) != 0 || len(graph.Diagnostics) != 0 {
					t.Fatalf("local type did not take precedence: %+v", graph)
				}
				return
			}
			edge := javaDependency(t, graph, packageID(app), packageID(dep))
			if len(graph.Dependencies) != 1 || len(edge.TypeRelations) != 1 {
				t.Fatalf("graph = %+v", graph)
			}
		})
	}
}

func TestJavaUnknownImportsCannotProveUniqueWildcardTarget(t *testing.T) {
	for _, imports := range []string{
		"import missing.Item; import known.*;",
		"import missing.*; import known.*;",
		"import known.Item; import missing.Item;",
	} {
		t.Run(imports, func(t *testing.T) {
			app := javaSource(t, "root", "App.java", "package app; "+imports+" class App { Item value; }")
			known := javaSource(t, "root", "Item.java", "package known; class Item {}")
			graph := javaBuild(t, []Source{app, known}, DeclaredTypes)
			if len(graph.Dependencies) != 0 || len(graph.Diagnostics) != 1 {
				t.Fatalf("unknown import incorrectly established an edge: %+v", graph)
			}
			if graph.Diagnostics[0].Status != Unresolved && graph.Diagnostics[0].Status != Ambiguous {
				t.Fatalf("diagnostic = %+v", graph.Diagnostics[0])
			}
		})
	}
}

func TestJavaUnnamedPackagesRemainContextLocal(t *testing.T) {
	app := javaSource(t, "first", "App.java", "class App { Item value; }")
	local := javaSource(t, "first", "Local.java", "class Item {}")
	other := javaSource(t, "second", "Other.java", "class Item {}")
	graph := javaBuild(t, []Source{app, local, other}, DeclaredTypes)
	if len(graph.Packages) != 2 || len(graph.Dependencies) != 0 || len(graph.Diagnostics) != 0 {
		t.Fatalf("unnamed packages were merged or confused: %+v", graph)
	}
	graph = javaBuild(t, []Source{app, other}, DeclaredTypes)
	if len(graph.Dependencies) != 0 || len(graph.Diagnostics) != 1 || graph.Diagnostics[0].Status != Unresolved {
		t.Fatalf("unnamed package leaked across roots: %+v", graph)
	}
}

func TestJavaDuplicateQualifiedTypesPreserveContexts(t *testing.T) {
	for _, text := range []string{
		"package app; import dep.Item; class App { Item value; }",
		"package app; class App { dep.Item value; }",
	} {
		app := javaSource(t, "app-root", "App.java", text)
		one := javaSource(t, "one", "One.java", "package dep; class Item {}")
		two := javaSource(t, "two", "Two.java", "package dep; class Item {}")
		graph := javaBuild(t, []Source{app, one, two}, DeclaredTypes)
		if len(graph.Dependencies) != 0 || len(graph.Diagnostics) != 1 {
			t.Fatalf("duplicate qualified declarations were merged: %+v", graph)
		}
		diagnostic := graph.Diagnostics[0]
		if diagnostic.Status != Ambiguous || len(diagnostic.Candidates) != 2 || diagnostic.Candidates[0].Package.Context == diagnostic.Candidates[1].Package.Context {
			t.Fatalf("diagnostic = %+v", diagnostic)
		}
	}
}

func TestJavaImportsAreOriginalFileDeclarations(t *testing.T) {
	app := javaSource(t, "root", "App.java", "package app;\nimport dep.Item;\nclass First { Item one; }\nclass Second { Item two; }")
	// Relations must use the source unit's declarations, not type-level copies
	// that another class analysis might have merged or replaced.
	for n := range app.Unit.Types {
		app.Unit.Types[n].Imports = nil
	}
	dep := javaSource(t, "root", "Item.java", "package dep; class Item {}")
	graph := javaBuild(t, []Source{app, dep}, Both)
	edge := javaDependency(t, graph, packageID(app), packageID(dep))
	if len(edge.Imports) != 1 || len(edge.TypeRelations) != 2 {
		t.Fatalf("file-scoped counts = %+v", edge)
	}
	if edge.Imports[0].Import.Span.Start.Line != 2 || edge.Imports[0].File.Path != app.Unit.Path {
		t.Fatalf("original import evidence = %+v", edge.Imports)
	}
}

func TestJavaImportsWithoutTypesAndImplicitJavaLang(t *testing.T) {
	file := javaSource(t, "root", "package-info.java", "package app; import dep.*;")
	dep := javaSource(t, "root", "dep-package-info.java", "package dep;")
	graph := javaBuild(t, []Source{file, dep}, Imports)
	edge := javaDependency(t, graph, packageID(file), packageID(dep))
	if len(edge.Imports) != 1 || len(edge.TypeRelations) != 0 {
		t.Fatalf("package-only import = %+v", edge)
	}

	app := javaSource(t, "root", "App.java", "package app; class App { Thread value; }")
	lang := javaSource(t, "root", "Thread.java", "package java.lang; class Thread {}")
	graph = javaBuild(t, []Source{app, lang}, Both)
	edge = javaDependency(t, graph, packageID(app), packageID(lang))
	if len(edge.Imports) != 0 || len(edge.TypeRelations) != 1 {
		t.Fatalf("implicit java.lang became import evidence: %+v", edge)
	}
}

func TestJavaGraphInputOrderDeterministic(t *testing.T) {
	sources := []Source{
		javaSource(t, "root", "App.java", "package app; import dep.*; import other.*; class App { Item value; dep.Outer.Inner nested; }"),
		javaSource(t, "root", "Dep.java", "package dep; class Item {} class Outer { static class Inner {} }"),
		javaSource(t, "root", "Other.java", "package other; class Item {}"),
		javaSource(t, "second", "Second.java", "package dep; class Item {}"),
	}
	want := javaBuild(t, sources, Both)
	slices.Reverse(sources)
	if got := javaBuild(t, sources, Both); !reflect.DeepEqual(got, want) {
		t.Fatalf("input order changed graph:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestJavaExplicitImportPrecedesOtherCompilationUnit(t *testing.T) {
	app := javaSource(t, "root", "App.java", "package app; import dep.Item; class App { Item value; }")
	local := javaSource(t, "root", "Local.java", "package app; class Item {}")
	dep := javaSource(t, "root", "Dep.java", "package dep; class Item {}")
	graph := javaBuild(t, []Source{app, local, dep}, DeclaredTypes)
	edge := javaDependency(t, graph, packageID(app), packageID(dep))
	if len(graph.Dependencies) != 1 || len(edge.TypeRelations) != 1 || len(graph.Diagnostics) != 0 {
		t.Fatalf("single-type import lost to another compilation unit: %+v", graph)
	}
}

func TestJavaAmbiguousWildcardOwnerDoesNotInferMissingMembers(t *testing.T) {
	tests := []struct {
		name            string
		owner           string
		alternative     string
		alternativeRoot string
	}{
		{"member only under type", "package dep; class Outer { static class Inner {} }", "package dep.Outer;", "root"},
		{"member only under package", "package dep; class Outer {}", "package dep.Outer; class Inner {}", "root"},
		{"member only in one root", "package dep; class Outer { static class Inner {} }", "package dep; class Outer {}", "other-root"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := javaSource(t, "root", "App.java", "package app; import dep.Outer.*; class App { Inner value; }")
			owner := javaSource(t, "root", "Outer.java", test.owner)
			alternative := javaSource(t, test.alternativeRoot, "Alternative.java", test.alternative)
			graph := javaBuild(t, []Source{app, owner, alternative}, Both)
			if len(graph.Dependencies) != 0 || len(graph.Diagnostics) != 2 {
				t.Fatalf("partial member index incorrectly selected an owner: %+v", graph)
			}
			for _, diagnostic := range graph.Diagnostics {
				if diagnostic.Status != Ambiguous || len(diagnostic.Candidates) != 2 {
					t.Fatalf("missing owner candidates: %+v", diagnostic)
				}
			}
		})
	}
}

func TestJavaPackageGraphHonorsDeclarationAndMethodParameterScopes(t *testing.T) {
	app := javaSource(t, "root", "App.java", `package app;
import dep.T;
import dep.U;
import dep.Pair;
class Box<T> {
 T value;
 Pair<T, U> nested;
 <U> U hidden(U input) { return input; }
 U concrete(U input) { return input; }
 class Inner<V> { T enclosing; V own; }
 static class Static { T value; }
}
`)
	dep := javaSource(t, "root", "Dep.java", "package dep; class T {} class U {} class Pair<A, B> {}")
	graph := javaBuild(t, []Source{app, dep}, DeclaredTypes)
	if len(graph.Diagnostics) != 0 || len(graph.Dependencies) != 1 {
		t.Fatalf("generic package graph = %+v", graph)
	}
	dependency := javaDependency(t, graph, packageID(app), packageID(dep))
	got := map[string]bool{}
	for _, relation := range dependency.TypeRelations {
		got[relation.From+"|"+relation.Label+"|"+relation.To] = true
	}
	want := map[string]bool{
		"app.Box|nested|dep.Pair":       true,
		"app.Box|nested|dep.U":          true,
		"app.Box|concrete(input)|dep.U": true,
		"app.Box|concrete|dep.U":        true,
		"app.Box.Static|value|dep.T":    true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scoped package relations = %#v, want %#v", got, want)
	}
}
