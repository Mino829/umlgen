package packagegraph

import (
	"reflect"
	"testing"

	"github.com/Mino829/umlgen/internal/golang"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

func goSource(t *testing.T, context, packagePath, path, text string) Source {
	t.Helper()
	unit, err := golang.ParseSource(path, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	unit.PackagePath = packagePath
	for index := range unit.Types {
		unit.Types[index].Package = packagePath
	}
	return Source{Context: context, Unit: unit}
}

func goBuild(t *testing.T, sources []Source, mode DependencySource) Graph {
	t.Helper()
	graph, err := Build(sources, mode)
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

func goDependency(t *testing.T, graph Graph, from, to PackageID) Dependency {
	t.Helper()
	for _, dependency := range graph.Dependencies {
		if dependency.From == from && dependency.To == to {
			return dependency
		}
	}
	t.Fatalf("missing dependency %v -> %v in %#v", from, to, graph.Dependencies)
	return Dependency{}
}

func goDiagnostic(t *testing.T, graph Graph, file, reference string, status Resolution) Diagnostic {
	t.Helper()
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.File.Path == file && diagnostic.Reference == reference && diagnostic.Status == status {
			return diagnostic
		}
	}
	t.Fatalf("missing %s diagnostic for %s in %s: %#v", status, reference, file, graph.Diagnostics)
	return Diagnostic{}
}

func TestGoPackageGraphRetainsAliasScopesInMethodOnlyFiles(t *testing.T) {
	declaration := goSource(t, "module", "example.com/app", "app/type.go", `package app
import dep "example.com/first"
type Holder struct { Value dep.One }
`)
	methods := goSource(t, "module", "example.com/app", "app/methods.go", `package app
import dep "example.com/second"
func (h *Holder) Take(value dep.Two) {}
`)
	first := goSource(t, "module", "example.com/first", "first/types.go", "package first\ntype One struct{}\n")
	second := goSource(t, "module", "example.com/second", "second/types.go", "package second\ntype Two struct{}\n")
	graph := goBuild(t, []Source{declaration, methods, first, second}, Both)
	if len(graph.Dependencies) != 2 || len(graph.Diagnostics) != 0 {
		t.Fatalf("graph = %#v", graph)
	}
	for _, test := range []struct {
		target Source
		file   string
		name   string
		kind   relations.Kind
	}{
		{first, declaration.Unit.Path, "example.com/first.One", relations.Field},
		{second, methods.Unit.Path, "example.com/second.Two", relations.Parameter},
	} {
		dependency := goDependency(t, graph, packageID(declaration), packageID(test.target))
		if len(dependency.Imports) != 1 || dependency.Imports[0].File.Path != test.file {
			t.Fatalf("imports for %s = %#v", test.name, dependency.Imports)
		}
		if len(dependency.TypeRelations) != 1 {
			t.Fatalf("relations for %s = %#v", test.name, dependency.TypeRelations)
		}
		relation := dependency.TypeRelations[0]
		if relation.To != test.name || relation.Kind != test.kind || len(relation.Evidence) != 1 || relation.Evidence[0].Source != test.file {
			t.Fatalf("relation = %#v", relation)
		}
	}
}

func TestGoPackageGraphUsesDeclaredPackageNameNotPathTail(t *testing.T) {
	for _, path := range []string{"example.com/store/v2", "v2"} {
		t.Run(path, func(t *testing.T) {
			app := goSource(t, "module", "example.com/app", "app.go", `package app
import "`+path+`"
type Holder struct {
 Good storage.Item
 Wrong v2.Item
 Missing storage.Missing
}
`)
			storage := goSource(t, "module", path, "storage.go", "package storage\ntype Item struct{}\n")
			graph := goBuild(t, []Source{app, storage}, DeclaredTypes)
			if len(graph.Dependencies) != 1 {
				t.Fatalf("dependencies = %#v", graph.Dependencies)
			}
			dependency := goDependency(t, graph, packageID(app), packageID(storage))
			if len(dependency.TypeRelations) != 1 || dependency.TypeRelations[0].To != path+".Item" {
				t.Fatalf("relations = %#v", dependency.TypeRelations)
			}
			goDiagnostic(t, graph, app.Unit.Path, "v2.Item", Unresolved)
			goDiagnostic(t, graph, app.Unit.Path, "storage.Missing", Unresolved)
		})
	}
}

func TestGoPackageGraphExternalAliasesAndBlankImports(t *testing.T) {
	app := goSource(t, "module", "example.com/app", "app.go", `package app
import (
 sdk "example.net/sdk/v2"
 "example.net/guess"
 _ "example.net/plugin"
)
type Holder struct {
 Explicit sdk.Client
 Default guess.Client
 Blank _.Client
}
func bodyOnly() { guess.Use() }
`)
	graph := goBuild(t, []Source{app}, Both)
	if len(graph.Dependencies) != 3 {
		t.Fatalf("dependencies = %#v", graph.Dependencies)
	}
	for _, path := range []string{"example.net/sdk/v2", "example.net/guess", "example.net/plugin"} {
		id := PackageID{Language: "go", Context: "module", Name: path}
		dependency := goDependency(t, graph, packageID(app), id)
		if len(dependency.Imports) != 1 || dependency.Imports[0].Import.Name != path {
			t.Fatalf("imports = %#v", dependency.Imports)
		}
		if path == "example.net/sdk/v2" {
			if len(dependency.TypeRelations) != 1 || dependency.TypeRelations[0].To != path+".Client" {
				t.Fatalf("explicit external alias relations = %#v", dependency.TypeRelations)
			}
		} else if len(dependency.TypeRelations) != 0 {
			t.Fatalf("nonbinding import produced type relations: %#v", dependency)
		}
		if path == "example.net/plugin" && dependency.Imports[0].Import.Alias != "_" {
			t.Fatalf("blank import = %#v", dependency.Imports)
		}
		found := false
		for _, pkg := range graph.Packages {
			if pkg.ID == id {
				found = true
				if pkg.Kind != ExternalPackage || len(pkg.Files) != 0 {
					t.Fatalf("external package = %#v", pkg)
				}
			}
		}
		if !found {
			t.Fatalf("missing external package %v", id)
		}
	}
	goDiagnostic(t, graph, app.Unit.Path, "guess.Client", Unresolved)
	goDiagnostic(t, graph, app.Unit.Path, "_.Client", Unresolved)
	declared := goBuild(t, []Source{app}, DeclaredTypes)
	if len(declared.Dependencies) != 1 || len(declared.Dependencies[0].Imports) != 0 || declared.Dependencies[0].To.Name != "example.net/sdk/v2" {
		t.Fatalf("declaration-only graph = %#v", declared)
	}
}

func TestGoPackageGraphExplicitAliasReplacesDeclaredQualifier(t *testing.T) {
	app := goSource(t, "module", "example.com/app", "app.go", `package app
import chosen "storage"
type Holder struct {
 Good chosen.Item
 Wrong storage.Item
}
`)
	storage := goSource(t, "module", "storage", "storage.go", "package storage\ntype Item struct{}\n")
	graph := goBuild(t, []Source{app, storage}, DeclaredTypes)
	if len(graph.Dependencies) != 1 || len(graph.Dependencies[0].TypeRelations) != 1 {
		t.Fatalf("dependencies = %#v", graph.Dependencies)
	}
	goDiagnostic(t, graph, app.Unit.Path, "storage.Item", Unresolved)
}

func TestGoPackageGraphDotImportsAndLocalPrecedence(t *testing.T) {
	for _, local := range []bool{false, true} {
		name := "ambiguous imports"
		text := `package app
import (
 . "example.com/first"
 . "example.com/second"
 . "example.net/unknown"
)
type Holder struct { Value Shared }
`
		if local {
			name = "local declaration wins"
			text += "type Shared struct{}\n"
		}
		t.Run(name, func(t *testing.T) {
			app := goSource(t, "module", "example.com/app", "app.go", text)
			first := goSource(t, "module", "example.com/first", "first.go", "package first\ntype Shared struct{}\n")
			second := goSource(t, "module", "example.com/second", "second.go", "package second\ntype Shared struct{}\n")
			graph := goBuild(t, []Source{app, first, second}, DeclaredTypes)
			if len(graph.Dependencies) != 0 {
				t.Fatalf("dependencies = %#v", graph.Dependencies)
			}
			if local {
				if len(graph.Diagnostics) != 0 {
					t.Fatalf("local declaration diagnostics = %#v", graph.Diagnostics)
				}
				return
			}
			diagnostic := goDiagnostic(t, graph, app.Unit.Path, "Shared", Ambiguous)
			want := []Target{
				{Package: packageID(first), Type: "example.com/first.Shared"},
				{Package: packageID(second), Type: "example.com/second.Shared"},
			}
			if !reflect.DeepEqual(diagnostic.Candidates, want) {
				t.Fatalf("candidates = %#v, want %#v", diagnostic.Candidates, want)
			}
		})
	}
}

func TestGoPackageGraphImportContextAmbiguity(t *testing.T) {
	for _, context := range []string{"consumer", "first"} {
		t.Run(context, func(t *testing.T) {
			app := goSource(t, context, "example.com/app", "app.go", `package app
import shared "example.com/shared"
type Holder struct { Value shared.Item }
`)
			first := goSource(t, "first", "example.com/shared", "first/shared.go", "package shared\ntype Item struct{}\n")
			second := goSource(t, "second", "example.com/shared", "second/shared.go", "package shared\ntype Item struct{}\n")
			graph := goBuild(t, []Source{second, app, first}, Both)
			if len(graph.Packages) != 3 {
				t.Fatalf("packages = %#v", graph.Packages)
			}
			if context == "first" {
				if len(graph.Dependencies) != 1 || len(graph.Diagnostics) != 0 {
					t.Fatalf("same-context graph = %#v", graph)
				}
				dependency := goDependency(t, graph, packageID(app), packageID(first))
				if len(dependency.Imports) != 1 || len(dependency.TypeRelations) != 1 {
					t.Fatalf("same-context dependency = %#v", dependency)
				}
				return
			}
			if len(graph.Dependencies) != 0 {
				t.Fatalf("ambiguous dependencies = %#v", graph.Dependencies)
			}
			for _, reference := range []string{"example.com/shared", "shared.Item"} {
				diagnostic := goDiagnostic(t, graph, app.Unit.Path, reference, Ambiguous)
				if len(diagnostic.Candidates) != 2 || diagnostic.Candidates[0].Package != packageID(first) || diagnostic.Candidates[1].Package != packageID(second) {
					t.Fatalf("candidates for %s = %#v", reference, diagnostic.Candidates)
				}
			}
			reversed := goBuild(t, []Source{first, app, second}, Both)
			if !reflect.DeepEqual(graph, reversed) {
				t.Fatalf("source ordering changed graph:\n%#v\n%#v", graph, reversed)
			}
		})
	}
}

func TestGoPackageGraphNoModulePackageClausesRemainContextual(t *testing.T) {
	left := goSource(t, "left-root", "sample", "left.go", "package sample\ntype Left struct { Other Right }\n")
	right := goSource(t, "right-root", "sample", "right.go", "package sample\ntype Right struct { Other Left }\n")
	graph := goBuild(t, []Source{left, right}, DeclaredTypes)
	if len(graph.Packages) != 2 || len(graph.Dependencies) != 0 {
		t.Fatalf("graph = %#v", graph)
	}
	goDiagnostic(t, graph, left.Unit.Path, "Right", Unresolved)
	goDiagnostic(t, graph, right.Unit.Path, "Left", Unresolved)
}

func TestGoPackageGraphResolvesCanonicalInferredParentNames(t *testing.T) {
	app := goSource(t, "module", "example.com/team/app/v2", "app.go", "package app\ntype Worker struct{}\nfunc (Worker) Work() {}\n")
	contract := goSource(t, "module", "example.com/team/contracts/v3", "contract.go", "package contracts\ntype Worker interface { Work() }\n")
	parsed := append([]model.Type(nil), app.Unit.Types...)
	parsed = append(parsed, contract.Unit.Types...)
	for _, typ := range golang.FinalizeWithEvidence(parsed) {
		if typ.Package == app.Unit.PackagePath {
			app.Unit.Types[0].Implements = typ.Implements
			app.Unit.Types[0].ImplementationMethods = typ.ImplementationMethods
		}
	}
	graph := goBuild(t, []Source{app, contract}, DeclaredTypes)
	if len(graph.Dependencies) != 1 {
		t.Fatalf("dependencies = %#v", graph.Dependencies)
	}
	dependency := goDependency(t, graph, packageID(app), packageID(contract))
	if len(dependency.TypeRelations) != 1 {
		t.Fatalf("relations = %#v", dependency.TypeRelations)
	}
	relation := dependency.TypeRelations[0]
	if relation.Kind != relations.Implementation || relation.To != "example.com/team/contracts/v3.Worker" {
		t.Fatalf("inferred implementation = %#v", relation)
	}
	for _, evidence := range relation.Evidence {
		if !evidence.Inferred {
			t.Fatalf("inferred implementation lost its analysis basis: %#v", evidence)
		}
	}
}

func TestGoPackageGraphUnknownDotImportPreservesUncertainty(t *testing.T) {
	app := goSource(t, "module", "example.com/app", "app.go", `package app
import (
 . "example.com/known"
 . "example.net/unknown"
)
type Holder struct { Value Item }
`)
	known := goSource(t, "module", "example.com/known", "known.go", "package known\ntype Item struct{}\n")
	graph := goBuild(t, []Source{app, known}, DeclaredTypes)
	if len(graph.Dependencies) != 0 {
		t.Fatalf("uncertain dot import produced dependencies: %#v", graph.Dependencies)
	}
	diagnostic := goDiagnostic(t, graph, app.Unit.Path, "Item", Ambiguous)
	want := []Target{{Package: packageID(known), Type: "example.com/known.Item"}}
	if !reflect.DeepEqual(diagnostic.Candidates, want) {
		t.Fatalf("known candidates = %#v, want %#v", diagnostic.Candidates, want)
	}
}

func TestGoPackageGraphKnownBlankImportDoesNotBindSymbols(t *testing.T) {
	app := goSource(t, "module", "app", "app.go", `package app
import _ "plugin"
type Holder struct { Value plugin.Item }
`)
	plugin := goSource(t, "module", "plugin", "plugin.go", "package plugin\ntype Item struct{}\n")
	graph := goBuild(t, []Source{app, plugin}, Both)
	if len(graph.Dependencies) != 1 {
		t.Fatalf("dependencies = %#v", graph.Dependencies)
	}
	dependency := goDependency(t, graph, packageID(app), packageID(plugin))
	if len(dependency.Imports) != 1 || dependency.Imports[0].Import.Alias != "_" || len(dependency.TypeRelations) != 0 {
		t.Fatalf("blank import dependency = %#v", dependency)
	}
	goDiagnostic(t, graph, app.Unit.Path, "plugin.Item", Unresolved)
}

func TestGoPackageGraphDoesNotSelectContextByTypePresence(t *testing.T) {
	app := goSource(t, "consumer", "example.com/app", "app.go", `package app
import shared "example.com/shared"
type Holder struct { Value shared.Item }
`)
	first := goSource(t, "first", "example.com/shared", "first.go", "package shared\ntype Item struct{}\n")
	second := goSource(t, "second", "example.com/shared", "second.go", "package shared\ntype Other struct{}\n")
	graph := goBuild(t, []Source{app, second, first}, Both)
	if len(graph.Dependencies) != 0 {
		t.Fatalf("type presence selected a package context: %#v", graph.Dependencies)
	}
	diagnostic := goDiagnostic(t, graph, app.Unit.Path, "shared.Item", Ambiguous)
	want := []Target{
		{Package: packageID(first), Type: "example.com/shared.Item"},
		{Package: packageID(second)},
	}
	if !reflect.DeepEqual(diagnostic.Candidates, want) {
		t.Fatalf("candidates = %#v, want %#v", diagnostic.Candidates, want)
	}
}

func TestGoInferredCanonicalNameDoesNotCaptureFileAlias(t *testing.T) {
	app := goSource(t, "module", "app", "app.go", "package app\nimport contract \"other\"\ntype Worker struct { Value contract.Worker }\nfunc (Worker) Work() {}\n")
	contract := goSource(t, "module", "contract", "contract.go", "package contracts\ntype Worker interface { Work() }\n")
	other := goSource(t, "module", "other", "other.go", "package other\ntype Worker struct{}\n")
	types := append(append(append([]model.Type(nil), app.Unit.Types...), contract.Unit.Types...), other.Unit.Types...)
	for _, typ := range golang.FinalizeWithEvidence(types) {
		if typ.Package == app.Unit.PackagePath {
			app.Unit.Types[0].Implements = typ.Implements
			app.Unit.Types[0].ImplementationMethods = typ.ImplementationMethods
		}
	}
	graph := goBuild(t, []Source{app, contract, other}, DeclaredTypes)
	if len(graph.Dependencies) != 2 || len(graph.Diagnostics) != 0 {
		t.Fatalf("inferred parent captured the field's file-local alias: %#v", graph)
	}
	implementation := goDependency(t, graph, packageID(app), packageID(contract))
	if len(implementation.TypeRelations) != 1 || implementation.TypeRelations[0].Kind != relations.Implementation ||
		len(implementation.TypeRelations[0].Evidence) != 2 {
		t.Fatalf("implementation lost declaration/matched-method evidence: %#v", implementation)
	}
	field := goDependency(t, graph, packageID(app), packageID(other))
	if len(field.TypeRelations) != 1 || field.TypeRelations[0].Kind != relations.Field {
		t.Fatalf("field ignored its import binding: %#v", field)
	}
}

func TestGoPackageGraphHonorsGenericScopesAndOmitsAliasReceivers(t *testing.T) {
	declarations := goSource(t, "module", "example.com/app", "app/types.go", `package app
import real "example.com/real"
type Pair[A, B any] struct{}
type Box[T, Missing any] struct {
 Generic T
 Unknown Missing
 Nested Pair[T, real.T]
}
type ID = real.T
`)
	methods := goSource(t, "module", "example.com/app", "app/methods.go", `package app
import . "example.com/dep"
import real "example.com/real"
func (b Box[U, V]) Take(value U, concrete T, unknown V) {}
func (ID) Hidden(value real.T) {}
`)
	dep := goSource(t, "module", "example.com/dep", "dep/types.go", "package dep\ntype T struct{}\ntype U struct{}")
	real := goSource(t, "module", "example.com/real", "real/types.go", "package real\ntype T struct{}")
	graph := goBuild(t, []Source{declarations, methods, dep, real}, DeclaredTypes)
	if len(graph.Diagnostics) != 0 || len(graph.Dependencies) != 2 {
		t.Fatalf("generic package graph = %#v", graph)
	}
	for _, test := range []struct {
		target Source
		label  string
	}{
		{dep, "Take(concrete)"},
		{real, "Nested"},
	} {
		dependency := goDependency(t, graph, packageID(declarations), packageID(test.target))
		if len(dependency.TypeRelations) != 1 || dependency.TypeRelations[0].From != "example.com/app.Box" ||
			dependency.TypeRelations[0].Label != test.label || dependency.TypeRelations[0].To != packageID(test.target).Name+".T" {
			t.Fatalf("scoped package relations = %#v", dependency.TypeRelations)
		}
	}
}

func TestGoPackageGraphDistinguishesAliasAndUnknownReceivers(t *testing.T) {
	aliases := goSource(t, "module", "example.com/app", "app/aliases.go", `package app
import dep "example.com/dep"
type Alias = dep.Item
`)
	methods := goSource(t, "module", "example.com/app", "app/methods.go", `package app
import dep "example.com/dep"
func (Alias) Hidden(value dep.Item) {}
func (Unknown) Use(value dep.Item) {}
`)
	dep := goSource(t, "module", "example.com/dep", "dep/types.go", "package dep\ntype Item struct{}")
	// An alias in another analysis context must not hide a partial declaration.
	foreign := goSource(t, "other", "example.com/app", "other/aliases.go", "package app\ntype Unknown = string")
	for _, sources := range [][]Source{
		{aliases, methods, dep, foreign},
		{methods, foreign, dep, aliases},
	} {
		index := newResolverIndex(sources)
		if result := index.resolveGoType(aliases, "Alias", false); result.Status != Unresolved {
			t.Fatalf("alias gained a declaration target: %#v", result)
		}
		for _, mode := range []DependencySource{DeclaredTypes, Both} {
			graph := goBuild(t, sources, mode)
			if len(graph.Diagnostics) != 0 || len(graph.Dependencies) != 1 {
				t.Fatalf("alias and method-only graph = %#v", graph)
			}
			dependency := goDependency(t, graph, packageID(aliases), packageID(dep))
			if len(dependency.TypeRelations) != 1 || dependency.TypeRelations[0].From != "example.com/app.Unknown" ||
				dependency.TypeRelations[0].To != "example.com/dep.Item" || dependency.TypeRelations[0].Label != "Use(value)" {
				t.Fatalf("alias and method-only relations = %#v", dependency.TypeRelations)
			}
			if mode == Both && len(dependency.Imports) != 2 {
				t.Fatalf("alias source imports were lost: %#v", dependency.Imports)
			}
		}
	}
}
