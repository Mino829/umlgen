package component

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mino829/umlgen/internal/packagegraph"
)

func TestBoundaryRootsAreLiteralFilesystemPaths(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "source [api] (v1)")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	text := "version: 1\ncomponents:\n  - name: API\n    match:\n      - language: java\n        source_root: './source [api] (v1)'\n"
	d, err := LoadDefinition(writeBoundaryDefinition(t, dir, text))
	if err != nil {
		t.Fatal(err)
	}
	if err = d.Validate(); err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if d.Components[0].Match[0].SourceRoot != expected {
		t.Fatalf("literal filesystem path was interpreted as a pattern: %+v", d.Components[0].Match[0])
	}
}

func TestSymlinkParentBoundaryOwnsPhysicalContext(t *testing.T) {
	dir := t.TempDir()
	actual := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(actual, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(actual, "src"), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	source := packagegraph.Graph{Packages: []packagegraph.Package{{ID: packagegraph.PackageID{Language: "java", Context: actual}, Kind: packagegraph.InternalPackage}}}
	for _, root := range []string{"link/..", link + string(os.PathSeparator) + ".."} {
		text := fmt.Sprintf("version: 1\ncomponents:\n  - name: Physical\n    match: [{language: java, source_root: %q}]\n", root)
		definition, err := LoadDefinition(writeBoundaryDefinition(t, dir, text))
		if err != nil {
			t.Fatal(err)
		}
		graph, err := Project(source, definition, nil, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if len(graph.Nodes) != 1 || len(graph.Nodes[0].Packages) != 1 {
			t.Fatalf("symlink parent selected the lexical rather than physical context: %+v", graph)
		}
	}
}

func TestPhysicalRootCaseAliasesCannotHideBoundaryConflict(t *testing.T) {
	dir := t.TempDir()
	actual := filepath.Join(dir, "PhysicalApp")
	alias := filepath.Join(dir, "physicalapp")
	if err := os.Mkdir(actual, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(alias); os.IsNotExist(err) {
		t.Skip("filesystem is case-sensitive")
	} else if err != nil {
		t.Fatal(err)
	}
	source := packagegraph.Graph{Packages: []packagegraph.Package{{ID: packagegraph.PackageID{Language: "java", Context: actual}, Kind: packagegraph.InternalPackage}}}
	definition := Definition{Version: 1, Components: []Boundary{{Name: "App", Match: []Selector{{Language: "java", SourceRoot: alias}}}}}
	graph, err := Project(source, definition, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 1 || len(graph.Nodes[0].Packages) != 1 {
		t.Fatalf("physical case alias lost ownership: %+v", graph)
	}
	definition.Components = append(definition.Components, Boundary{Name: "Conflict", Match: []Selector{{Language: "java", SourceRoot: actual}}})
	if _, err = Project(source, definition, nil, Options{}); !errors.Is(err, ErrBoundary) {
		t.Fatalf("physical alias concealed conflicting boundaries: %v", err)
	}
}

func TestAbsentRootWithForwardSeparatorsRemainsUnused(t *testing.T) {
	dir := t.TempDir()
	text := "version: 1\ncomponents:\n  - name: Future\n    match: [{language: java, source_root: 'missing/child/'}]\n"
	definition, err := LoadDefinition(writeBoundaryDefinition(t, dir, text))
	if err != nil {
		t.Fatal(err)
	}
	source := packagegraph.Graph{Packages: []packagegraph.Package{{ID: packagegraph.PackageID{Language: "java", Context: dir}, Kind: packagegraph.InternalPackage}}}
	graph, err := Project(source, definition, nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 {
		t.Fatalf("absent nested root acquired existing packages: %+v", graph)
	}
	found := false
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Code == "unused-boundary" {
			found = true
		}
	}
	if !found {
		t.Fatalf("future root silently disappeared: %+v", graph.Diagnostics)
	}
}
