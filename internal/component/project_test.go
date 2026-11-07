package component

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/relations"
)

func projectionPackage(language, context, name string) packagegraph.Package {
	return packagegraph.Package{
		ID:    packagegraph.PackageID{Language: language, Context: context, Name: name},
		Kind:  packagegraph.InternalPackage,
		Files: []packagegraph.File{{Path: context + "/source", ContentHash: "captured-source-hash"}},
	}
}

func projectionDefinition(boundaries ...Boundary) Definition {
	return Definition{Version: 1, Components: boundaries, Source: "/architecture/components.yaml", Hash: "definition-hash"}
}

func projectionBoundary(name, language, prefix string) Boundary {
	return Boundary{Name: name, Match: []Selector{{Language: language, PackagePrefix: prefix}}}
}

func projectionNode(t *testing.T, graph Graph, id ID) Node {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("node %#v not found in %#v", id, graph.Nodes)
	return Node{}
}

func projectionDiagnosticCount(graph Graph, code string) int {
	count := 0
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Code == code {
			count++
		}
	}
	return count
}

func TestProjectRejectsOverlapWithoutSpecificityOrOrderPrecedence(t *testing.T) {
	pkg := projectionPackage("go", "/src", "example.test/app/api")
	broad := projectionBoundary("Broad", "go", "example.test/app")
	specific := projectionBoundary("Specific", "go", "example.test/app/api")
	for _, boundaries := range [][]Boundary{{broad, specific}, {specific, broad}} {
		graph, err := Project(packagegraph.Graph{Packages: []packagegraph.Package{pkg}}, projectionDefinition(boundaries...), nil, Options{})
		if !errors.Is(err, ErrBoundary) {
			t.Fatalf("overlap error = %v", err)
		}
		if !reflect.DeepEqual(graph, Graph{}) {
			t.Fatalf("overlap returned a partially drawable graph: %#v", graph)
		}
	}
	broad.Match = append(broad.Match, specific.Match...)
	graph, err := Project(packagegraph.Graph{Packages: []packagegraph.Package{pkg}}, projectionDefinition(broad), nil, Options{})
	if err != nil || len(graph.Nodes) != 1 || len(graph.Nodes[0].Packages) != 1 {
		t.Fatalf("repeated matches within one component: %#v, %v", graph, err)
	}
	if projectionDiagnosticCount(graph, "unused-selector") != 0 {
		t.Fatalf("matching selector incorrectly diagnosed: %#v", graph.Diagnostics)
	}
}

func TestProjectUsesLanguageSpecificDelimitedPrefixes(t *testing.T) {
	for _, test := range []struct {
		language    string
		prefix      string
		matching    []string
		notMatching []string
	}{
		{"go", "example.test/app/api", []string{"example.test/app/api", "example.test/app/api/v2"}, []string{"example.test/app/apix", "example.test/app/api.v2"}},
		{"java", "org.example.api", []string{"org.example.api", "org.example.api.v2"}, []string{"org.example.apix", "org.example.api/v2"}},
	} {
		t.Run(test.language, func(t *testing.T) {
			var source packagegraph.Graph
			for _, name := range append(slices.Clone(test.matching), test.notMatching...) {
				source.Packages = append(source.Packages, projectionPackage(test.language, "/src", name))
			}
			graph, err := Project(source, projectionDefinition(projectionBoundary("API", test.language, test.prefix)), nil, Options{})
			if err != nil {
				t.Fatal(err)
			}
			node := projectionNode(t, graph, ID{Kind: "component", Name: "API"})
			if len(node.Packages) != len(test.matching) || projectionDiagnosticCount(graph, "unassigned-package") != len(test.notMatching) {
				t.Fatalf("prefix projection = %#v; diagnostics = %#v", node, graph.Diagnostics)
			}
			for _, pkg := range node.Packages {
				if !slices.Contains(test.matching, pkg.ID.Name) {
					t.Fatalf("false prefix match: %#v", pkg)
				}
			}
		})
	}
}

func TestProjectKeepsLanguageAndExactRootIdentity(t *testing.T) {
	packages := []packagegraph.Package{
		projectionPackage("go", "/root/one", "shared"),
		projectionPackage("go", "/root/two", "shared"),
		projectionPackage("java", "/root/one", "shared"),
		projectionPackage("go", "/root/one/nested", "shared"),
	}
	// A file beneath the selected root is not the package's analysis identity.
	packages[1].Files[0].Path = "/root/one/copied/source.go"
	selected := Boundary{Name: "Selected", Match: []Selector{{Language: "go", SourceRoot: "/root/one", PackagePrefix: "shared"}}}
	graph, err := Project(packagegraph.Graph{Packages: packages}, projectionDefinition(selected), nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	node := projectionNode(t, graph, ID{Kind: "component", Name: "Selected"})
	if len(node.Packages) != 1 || node.Packages[0].ID != packages[0].ID {
		t.Fatalf("root/language assignment = %#v", node.Packages)
	}
	if len(graph.Nodes) != 4 || projectionDiagnosticCount(graph, "unassigned-package") != 3 {
		t.Fatalf("unassigned packages collapsed: %#v", graph)
	}
	for _, pkg := range packages[1:] {
		unassigned := projectionNode(t, graph, ID{Kind: "unassigned", Package: pkg.ID})
		if len(unassigned.Packages) != 1 || unassigned.Packages[0].ID != pkg.ID || unassigned.Boundary != nil {
			t.Fatalf("unassigned identity = %#v", unassigned)
		}
	}
}

func TestProjectGoModuleUsesCapturedExactIdentity(t *testing.T) {
	parent := projectionPackage("go", "/root", "example.test/app/api")
	nested := projectionPackage("go", "/root/nested", "example.test/app/nested/api")
	unknown := projectionPackage("go", "/unknown", "example.test/app/unknown")
	external := projectionPackage("go", "/root", "example.test/app/external")
	external.Kind = packagegraph.ExternalPackage
	java := projectionPackage("java", "/root", "example.test.app")
	definition := projectionDefinition(
		Boundary{Name: "Parent", Match: []Selector{{Language: "go", GoModule: "example.test/app"}}},
		Boundary{Name: "Nested", Match: []Selector{{Language: "go", GoModule: "example.test/app/nested", PackagePrefix: "example.test/app/nested/api"}}},
	)
	modules := []Module{
		{Context: "/root/nested", Name: "example.test/app/nested", File: "/root/nested/go.mod", Hash: "nested-hash"},
		{Context: "/root", Name: "example.test/app", File: "/root/go.mod", Hash: "parent-hash"},
	}
	graph, err := Project(packagegraph.Graph{Packages: []packagegraph.Package{external, unknown, nested, parent, java}}, definition, modules, Options{ShowExternal: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		name string
		pkg  packagegraph.Package
	}{{"Parent", parent}, {"Nested", nested}} {
		node := projectionNode(t, graph, ID{Kind: "component", Name: expected.name})
		if len(node.Packages) != 1 || node.Packages[0].ID != expected.pkg.ID {
			t.Fatalf("module assignment for %s = %#v", expected.name, node.Packages)
		}
	}
	projectionNode(t, graph, ID{Kind: "external", Package: external.ID})
	projectionNode(t, graph, ID{Kind: "unassigned", Package: unknown.ID})
	projectionNode(t, graph, ID{Kind: "unassigned", Package: java.ID})
	if !reflect.DeepEqual(graph.Modules, []Module{modules[1], modules[0]}) {
		t.Fatalf("captured module provenance = %#v", graph.Modules)
	}
}

func TestProjectRequiresEverySelectorCondition(t *testing.T) {
	selected := projectionPackage("go", "/selected", "example.test/app/api")
	wrongPrefix := projectionPackage("go", "/selected", "example.test/app/store")
	wrongRoot := projectionPackage("go", "/other", "example.test/app/api")
	boundary := Boundary{Name: "API", Match: []Selector{{
		Language: "go", SourceRoot: "/selected", PackagePrefix: "example.test/app/api", GoModule: "example.test/app",
	}}}
	source := packagegraph.Graph{Packages: []packagegraph.Package{wrongRoot, wrongPrefix, selected}}
	modules := []Module{{Context: "/selected", Name: "example.test/app"}, {Context: "/other", Name: "example.test/app"}}
	graph, err := Project(source, projectionDefinition(boundary), modules, Options{})
	if err != nil {
		t.Fatal(err)
	}
	node := projectionNode(t, graph, ID{Kind: "component", Name: "API"})
	if !reflect.DeepEqual(node.Packages, []packagegraph.Package{selected}) || projectionDiagnosticCount(graph, "unassigned-package") != 2 {
		t.Fatalf("selector conditions were not AND: %#v", graph)
	}
	// A package-path prefix cannot substitute for the exact captured module name.
	boundary.Match[0].GoModule = "example.test"
	graph, err = Project(source, projectionDefinition(boundary), modules, Options{})
	if err != nil {
		t.Fatal(err)
	}
	node = projectionNode(t, graph, ID{Kind: "component", Name: "API"})
	if len(node.Packages) != 0 || projectionDiagnosticCount(graph, "unassigned-package") != 3 {
		t.Fatalf("module selector used prefix rather than identity: %#v", graph)
	}
}

func TestProjectPreservesOriginalEvidenceAndInternalDependencies(t *testing.T) {
	api := projectionPackage("go", "/src", "example.test/app/api")
	worker := projectionPackage("go", "/src", "example.test/app/worker")
	storage := projectionPackage("go", "/src", "example.test/app/storage")
	file := packagegraph.File{Path: "/src/api/type.go", ContentHash: "original-hash"}
	first := packagegraph.Dependency{
		From: api.ID, To: storage.ID,
		TypeRelations: []relations.Relation{{
			From: "example.test/app/api.Service", To: "example.test/app/storage.Store", Kind: relations.Implementation,
			Label: "Store", Multiplicity: "*",
			Evidence: []relations.Evidence{
				{Member: "Save", Type: "Store", Source: file.Path, Line: 7, Inferred: true, Language: "go", OwnerKind: model.Struct},
				{Member: "Load", Type: "Store", Source: file.Path, Line: 9, Inferred: true, Language: "go", OwnerKind: model.Struct},
			},
		}},
		Imports: []packagegraph.ImportEvidence{
			{File: file, Import: model.Import{Name: storage.ID.Name, Alias: "db", Span: model.SourceSpan{Start: model.SourcePosition{Line: 2}}}},
			{File: file, Import: model.Import{Name: storage.ID.Name, Alias: "_", Span: model.SourceSpan{Start: model.SourcePosition{Line: 3}}}},
		},
	}
	second := packagegraph.Dependency{From: worker.ID, To: storage.ID, TypeRelations: []relations.Relation{{
		From: "example.test/app/worker.Worker", To: "example.test/app/storage.Store", Kind: relations.Field,
		Evidence: []relations.Evidence{{Member: "DB", Source: "/src/worker/type.go", Line: 11}},
	}}}
	internal := packagegraph.Dependency{From: api.ID, To: worker.ID, Imports: []packagegraph.ImportEvidence{{File: file, Import: model.Import{Name: worker.ID.Name}}}}
	source := packagegraph.Graph{Source: packagegraph.Both, Packages: []packagegraph.Package{storage, worker, api}, Dependencies: []packagegraph.Dependency{second, internal, first}}
	definition := projectionDefinition(
		Boundary{Name: "Application", Match: []Selector{{Language: "go", PackagePrefix: api.ID.Name}, {Language: "go", PackagePrefix: worker.ID.Name}}},
		projectionBoundary("Storage", "go", storage.ID.Name),
	)
	beforeSource, _ := json.Marshal(source)
	beforeDefinition, _ := json.Marshal(definition)
	graph, err := Project(source, definition, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Dependencies) != 1 {
		t.Fatalf("aggregated dependencies = %#v", graph.Dependencies)
	}
	dependency := graph.Dependencies[0]
	if dependency.From != (ID{Kind: "component", Name: "Application"}) || dependency.To != (ID{Kind: "component", Name: "Storage"}) || !reflect.DeepEqual(dependency.Originals, []packagegraph.Dependency{first, second}) {
		t.Fatalf("lost original endpoint/member/import evidence: %#v", dependency)
	}
	// A type record, its evidence locations, and import declarations remain
	// distinct counting units rather than interchangeable occurrences.
	if len(dependency.Originals[0].TypeRelations) != 1 || len(dependency.Originals[0].TypeRelations[0].Evidence) != 2 || len(dependency.Originals[0].Imports) != 2 || !dependency.Originals[0].TypeRelations[0].Evidence[0].Inferred {
		t.Fatalf("evidence units/inference flag changed: %#v", dependency.Originals[0])
	}
	application := projectionNode(t, graph, ID{Kind: "component", Name: "Application"})
	if !reflect.DeepEqual(application.Internal, []packagegraph.Dependency{internal}) || !reflect.DeepEqual(application.Packages, []packagegraph.Package{api, worker}) {
		t.Fatalf("internal/package provenance = %#v", application)
	}
	if !reflect.DeepEqual(graph.Source, source) || !reflect.DeepEqual(graph.Definition, definition) {
		t.Fatal("complete source scope or definition not retained")
	}
	afterSource, _ := json.Marshal(source)
	afterDefinition, _ := json.Marshal(definition)
	if string(beforeSource) != string(afterSource) || string(beforeDefinition) != string(afterDefinition) {
		t.Fatal("projection mutated its input")
	}
}

func TestProjectPreservesUncertaintyAndHiddenExternalOriginals(t *testing.T) {
	assigned := projectionPackage("java", "/src", "org.example.api")
	unassigned := projectionPackage("java", "/other", "org.example.other")
	external := projectionPackage("java", "/src", "org.vendor.lib")
	external.Kind = packagegraph.ExternalPackage
	missing := packagegraph.PackageID{Language: "java", Context: "/src", Name: "org.missing"}
	diagnostic := packagegraph.Diagnostic{
		From: assigned.ID, Status: packagegraph.Unresolved, Code: "unresolved-type", Reference: "Missing",
		File: assigned.Files[0], Line: 8, Kind: relations.Field, Member: "value", Owner: "org.example.api.Handler",
		Candidates: []packagegraph.Target{{Package: missing, Type: "org.missing.Missing"}},
	}
	visible := packagegraph.Dependency{From: assigned.ID, To: unassigned.ID}
	hidden := packagegraph.Dependency{From: assigned.ID, To: external.ID, Imports: []packagegraph.ImportEvidence{{File: assigned.Files[0], Import: model.Import{Name: "org.vendor.lib.Item"}}}}
	source := packagegraph.Graph{Packages: []packagegraph.Package{assigned, unassigned, external}, Dependencies: []packagegraph.Dependency{hidden, visible}, Diagnostics: []packagegraph.Diagnostic{diagnostic}}
	// Even an explicit external-name selector cannot assign external endpoints.
	definition := projectionDefinition(Boundary{Name: "API", Match: []Selector{{Language: "java", PackagePrefix: "org.example.api"}, {Language: "java", PackagePrefix: "org.vendor"}}})
	for _, show := range []bool{false, true} {
		graph, err := Project(source, definition, nil, Options{ShowExternal: show})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(graph.Source.Diagnostics, []packagegraph.Diagnostic{diagnostic}) {
			t.Fatalf("source uncertainty lost: %#v", graph.Source.Diagnostics)
		}
		for _, node := range graph.Nodes {
			if node.ID.Package == missing {
				t.Fatal("unresolved diagnostic invented a package node")
			}
		}
		if projectionDiagnosticCount(graph, "unassigned-package") != 1 || projectionDiagnosticCount(graph, "unused-selector") != 1 {
			t.Fatalf("assignment diagnostics = %#v", graph.Diagnostics)
		}
		if show {
			if len(graph.Hidden) != 0 || len(graph.Dependencies) != 2 || len(graph.Nodes) != 3 {
				t.Fatalf("external visibility = %#v", graph)
			}
			node := projectionNode(t, graph, ID{Kind: "external", Package: external.ID})
			if node.Boundary != nil || !reflect.DeepEqual(node.Packages, []packagegraph.Package{external}) {
				t.Fatalf("external endpoint inherited ownership: %#v", node)
			}
		} else if !reflect.DeepEqual(graph.Hidden, []packagegraph.Dependency{hidden}) || len(graph.Dependencies) != 1 || len(graph.Nodes) != 2 {
			t.Fatalf("hidden external provenance = %#v", graph)
		}
	}
}

func TestProjectRetainsEmptyComponentsAndDisplayOnlyInterfaces(t *testing.T) {
	definition := projectionDefinition(
		Boundary{Name: "API", Line: 3, Match: []Selector{{Language: "go", PackagePrefix: "app/api", Line: 5}}, Provides: []string{"Orders"}, Requires: []string{"Storage"}},
		Boundary{Name: "Storage", Line: 9, Match: []Selector{{Language: "go", PackagePrefix: "app/storage", Line: 11}}, Provides: []string{"Storage"}, Requires: []string{"Orders"}},
	)
	graph, err := Project(packagegraph.Graph{}, definition, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 || len(graph.Dependencies) != 0 || projectionDiagnosticCount(graph, "unused-boundary") != 2 || projectionDiagnosticCount(graph, "unused-selector") != 2 {
		t.Fatalf("empty components or contractual non-edges = %#v", graph)
	}
	for _, boundary := range definition.Components {
		node := projectionNode(t, graph, ID{Kind: "component", Name: boundary.Name})
		if node.Boundary == nil || !reflect.DeepEqual(*node.Boundary, boundary) || len(node.Packages) != 0 {
			t.Fatalf("interface declaration lost: %#v", node)
		}
		for _, diagnostic := range graph.Diagnostics {
			if diagnostic.Component == boundary.Name && ((diagnostic.Code == "unused-boundary" && diagnostic.Line != boundary.Line) || (diagnostic.Code == "unused-selector" && diagnostic.Line != boundary.Match[0].Line)) {
				t.Fatalf("boundary diagnostic lost YAML location: %#v", diagnostic)
			}
		}
	}
}

func TestProjectRejectsInconsistentSourceAndModuleMetadata(t *testing.T) {
	valid := projectionPackage("go", "/src", "example.test/app/api")
	missing := valid.ID
	missing.Name = "example.test/app/missing"
	definition := projectionDefinition(projectionBoundary("App", "go", "example.test/app"))
	for _, test := range []struct {
		name    string
		source  packagegraph.Graph
		modules []Module
	}{
		{name: "duplicate package", source: packagegraph.Graph{Packages: []packagegraph.Package{valid, valid}}},
		{name: "unknown endpoint source", source: packagegraph.Graph{Packages: []packagegraph.Package{valid}, Dependencies: []packagegraph.Dependency{{From: missing, To: valid.ID}}}},
		{name: "unknown endpoint target", source: packagegraph.Graph{Packages: []packagegraph.Package{valid}, Dependencies: []packagegraph.Dependency{{From: valid.ID, To: missing}}}},
		{name: "invalid package language", source: packagegraph.Graph{Packages: []packagegraph.Package{projectionPackage("ruby", "/src", "app")}}},
		{name: "empty package context", source: packagegraph.Graph{Packages: []packagegraph.Package{projectionPackage("go", "", "app")}}},
		{name: "empty Go package name", source: packagegraph.Graph{Packages: []packagegraph.Package{projectionPackage("go", "/src", "")}}},
		{name: "invalid package kind", source: packagegraph.Graph{Packages: []packagegraph.Package{{ID: valid.ID, Kind: "unresolved"}}}},
		{name: "missing module name", source: packagegraph.Graph{Packages: []packagegraph.Package{valid}}, modules: []Module{{Context: "/src"}}},
		{name: "missing module context", source: packagegraph.Graph{Packages: []packagegraph.Package{valid}}, modules: []Module{{Name: "example.test/app"}}},
		{name: "conflicting module names", source: packagegraph.Graph{Packages: []packagegraph.Package{valid}}, modules: []Module{{Context: "/src", Name: "example.test/app"}, {Context: "/src", Name: "example.test/app/nested"}}},
		{name: "conflicting module snapshots", source: packagegraph.Graph{Packages: []packagegraph.Package{valid}}, modules: []Module{{Context: "/src", Name: "example.test/app", Hash: "one"}, {Context: "/src", Name: "example.test/app", Hash: "two"}}},
		{name: "package outside captured module", source: packagegraph.Graph{Packages: []packagegraph.Package{valid}}, modules: []Module{{Context: "/src", Name: "example.test/other"}}},
		{name: "module prefix is not module identity", source: packagegraph.Graph{Packages: []packagegraph.Package{valid}}, modules: []Module{{Context: "/src", Name: "example.test/ap"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph, err := Project(test.source, definition, test.modules, Options{})
			if !errors.Is(err, ErrBoundary) || !reflect.DeepEqual(graph, Graph{}) {
				t.Fatalf("invalid source returned %#v, %v", graph, err)
			}
		})
	}
}

func TestProjectDeterministicWithoutReorderingInputs(t *testing.T) {
	one := projectionPackage("go", "/one", "app/one")
	two := projectionPackage("go", "/two", "app/two")
	other := projectionPackage("go", "/one", "app/other")
	definition := projectionDefinition(projectionBoundary("Two", "go", "app/two"), projectionBoundary("One", "go", "app/one"))
	modules := []Module{{Context: "/two", Name: "app"}, {Context: "/one", Name: "app"}}
	source := packagegraph.Graph{
		Packages:     []packagegraph.Package{two, other, one},
		Dependencies: []packagegraph.Dependency{{From: two.ID, To: one.ID}, {From: one.ID, To: other.ID}},
	}
	first, err := Project(source, definition, modules, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if source.Packages[0].ID != two.ID || source.Dependencies[0].From != two.ID || modules[0].Context != "/two" || definition.Components[0].Name != "Two" {
		t.Fatal("input order changed")
	}
	reordered := source
	reordered.Packages = slices.Clone(source.Packages)
	reordered.Dependencies = slices.Clone(source.Dependencies)
	slices.Reverse(reordered.Packages)
	slices.Reverse(reordered.Dependencies)
	second, err := Project(reordered, definition, []Module{modules[1], modules[0]}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Source deliberately retains original capture order; the drawable projection
	// has canonical order independent of package/dependency/module input order.
	first.Source = packagegraph.Graph{}
	second.Source = packagegraph.Graph{}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("projection depends on input order:\nfirst: %#v\nsecond: %#v", first, second)
	}
}
