package component

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/Mino829/umlgen/internal/packagegraph"
)

// Project groups only captured source dependencies under explicit boundaries.
// Nested source metadata is borrowed read-only, as in packagegraph.View; neither
// the source graph nor the definition is modified and no new evidence is inferred.
func Project(source packagegraph.Graph, definition Definition, modules []Module, opts Options) (Graph, error) {
	if err := definition.Validate(); err != nil {
		return Graph{}, err
	}
	packages, moduleByContext, err := validateProjectionSource(source, modules)
	if err != nil {
		return Graph{}, err
	}

	// Resolve every assignment before constructing a drawable graph. In
	// particular, a more specific selector never overrides a broader boundary.
	owners := make(map[packagegraph.PackageID]int, len(packages))
	matched := make([][]bool, len(definition.Components))
	var roots rootIdentities
	for i, boundary := range definition.Components {
		matched[i] = make([]bool, len(boundary.Match))
	}
	ordered := make([]int, len(source.Packages))
	for i := range ordered {
		ordered[i] = i
	}
	slices.SortFunc(ordered, func(a, b int) int {
		return compareProjectionPackage(source.Packages[a].ID, source.Packages[b].ID)
	})
	for _, index := range ordered {
		pkg := source.Packages[index]
		if pkg.Kind != packagegraph.InternalPackage {
			continue
		}
		owner := -1
		for i, boundary := range definition.Components {
			for j, selector := range boundary.Match {
				if !matchesPackage(pkg.ID, selector, moduleByContext, &roots) {
					continue
				}
				matched[i][j] = true
				if owner != -1 && owner != i {
					return Graph{}, fmt.Errorf("%w: package %s:%s::%s matches both %q and %q", ErrBoundary,
						pkg.ID.Language, pkg.ID.Context, pkg.ID.Name, definition.Components[owner].Name, boundary.Name)
				}
				owner = i
			}
		}
		owners[pkg.ID] = owner
	}

	graph := Graph{Definition: definition, Source: source, Modules: slices.Clone(modules)}
	slices.SortFunc(graph.Modules, func(a, b Module) int {
		if order := cmp.Compare(a.Context, b.Context); order != 0 {
			return order
		}
		return cmp.Compare(a.Name, b.Name)
	})
	nodes := make(map[ID]int, len(definition.Components)+len(packages))
	for i := range definition.Components {
		boundary := &definition.Components[i]
		id := ID{Kind: "component", Name: boundary.Name}
		nodes[id] = len(graph.Nodes)
		graph.Nodes = append(graph.Nodes, Node{ID: id, Name: boundary.Name, Boundary: boundary})
		used := false
		for j, didMatch := range matched[i] {
			if didMatch {
				used = true
				continue
			}
			graph.Diagnostics = append(graph.Diagnostics, Diagnostic{
				Code: "unused-selector", Component: boundary.Name, Line: boundary.Match[j].Line,
				Message: "selector matches no internal source package",
			})
		}
		if !used {
			graph.Diagnostics = append(graph.Diagnostics, Diagnostic{
				Code: "unused-boundary", Component: boundary.Name, Line: boundary.Line,
				Message: "component boundary contains no internal source packages",
			})
		}
	}
	projected := make(map[packagegraph.PackageID]ID, len(packages))
	for _, index := range ordered {
		pkg := source.Packages[index]
		var id ID
		if pkg.Kind == packagegraph.ExternalPackage {
			id = ID{Kind: "external", Package: pkg.ID}
			projected[pkg.ID] = id
			if !opts.ShowExternal {
				continue
			}
		} else if owner := owners[pkg.ID]; owner >= 0 {
			id = ID{Kind: "component", Name: definition.Components[owner].Name}
		} else {
			id = ID{Kind: "unassigned", Package: pkg.ID}
			packageID := pkg.ID
			graph.Diagnostics = append(graph.Diagnostics, Diagnostic{
				Code: "unassigned-package", Package: &packageID,
				Message: "internal source package matches no component boundary",
			})
		}
		projected[pkg.ID] = id
		position, exists := nodes[id]
		if !exists {
			position = len(graph.Nodes)
			nodes[id] = position
			graph.Nodes = append(graph.Nodes, Node{ID: id, Name: pkg.ID.Name})
		}
		graph.Nodes[position].Packages = append(graph.Nodes[position].Packages, pkg)
	}

	type edge struct{ from, to ID }
	edges := make(map[edge]int)
	dependencyOrder := make([]int, len(source.Dependencies))
	for i := range dependencyOrder {
		dependencyOrder[i] = i
	}
	slices.SortStableFunc(dependencyOrder, func(a, b int) int {
		left, right := source.Dependencies[a], source.Dependencies[b]
		if order := compareProjectionPackage(left.From, right.From); order != 0 {
			return order
		}
		return compareProjectionPackage(left.To, right.To)
	})
	for _, index := range dependencyOrder {
		original := source.Dependencies[index]
		from, to := projected[original.From], projected[original.To]
		if !opts.ShowExternal && (from.Kind == "external" || to.Kind == "external") {
			graph.Hidden = append(graph.Hidden, original)
			continue
		}
		if from == to {
			position := nodes[from]
			graph.Nodes[position].Internal = append(graph.Nodes[position].Internal, original)
			continue
		}
		key := edge{from, to}
		position, exists := edges[key]
		if !exists {
			position = len(graph.Dependencies)
			edges[key] = position
			graph.Dependencies = append(graph.Dependencies, Dependency{From: from, To: to})
		}
		graph.Dependencies[position].Originals = append(graph.Dependencies[position].Originals, original)
	}
	slices.SortFunc(graph.Nodes, func(a, b Node) int { return compareProjectionID(a.ID, b.ID) })
	slices.SortFunc(graph.Dependencies, func(a, b Dependency) int {
		if order := compareProjectionID(a.From, b.From); order != 0 {
			return order
		}
		return compareProjectionID(a.To, b.To)
	})
	slices.SortFunc(graph.Diagnostics, func(a, b Diagnostic) int {
		if order := cmp.Compare(a.Component, b.Component); order != 0 {
			return order
		}
		if order := cmp.Compare(a.Code, b.Code); order != 0 {
			return order
		}
		if order := cmp.Compare(a.Line, b.Line); order != 0 {
			return order
		}
		if a.Package != nil && b.Package != nil {
			return compareProjectionPackage(*a.Package, *b.Package)
		}
		return 0
	})
	return graph, nil
}

func validateProjectionSource(source packagegraph.Graph, modules []Module) (map[packagegraph.PackageID]packagegraph.PackageKind, map[string]Module, error) {
	packages := make(map[packagegraph.PackageID]packagegraph.PackageKind, len(source.Packages))
	for _, pkg := range source.Packages {
		id := pkg.ID
		if (id.Language != "go" && id.Language != "java") || id.Context == "" || (id.Language == "go" && id.Name == "") {
			return nil, nil, fmt.Errorf("%w: invalid source package identity %#v", ErrBoundary, id)
		}
		if pkg.Kind != packagegraph.InternalPackage && pkg.Kind != packagegraph.ExternalPackage {
			return nil, nil, fmt.Errorf("%w: invalid source package kind %q", ErrBoundary, pkg.Kind)
		}
		if _, exists := packages[id]; exists {
			return nil, nil, fmt.Errorf("%w: duplicate source package identity %#v", ErrBoundary, id)
		}
		packages[id] = pkg.Kind
	}
	for _, dependency := range source.Dependencies {
		if _, exists := packages[dependency.From]; !exists {
			return nil, nil, fmt.Errorf("%w: dependency source is not a captured package: %#v", ErrBoundary, dependency.From)
		}
		if _, exists := packages[dependency.To]; !exists {
			return nil, nil, fmt.Errorf("%w: dependency target is not a captured package: %#v", ErrBoundary, dependency.To)
		}
	}
	moduleByContext := make(map[string]Module, len(modules))
	for _, module := range modules {
		if module.Context == "" || module.Name == "" {
			return nil, nil, fmt.Errorf("%w: Go module metadata requires context and module name", ErrBoundary)
		}
		if previous, exists := moduleByContext[module.Context]; exists && previous != module {
			return nil, nil, fmt.Errorf("%w: conflicting Go module metadata for context %q", ErrBoundary, module.Context)
		}
		moduleByContext[module.Context] = module
	}
	for _, pkg := range source.Packages {
		if pkg.Kind != packagegraph.InternalPackage || pkg.ID.Language != "go" {
			continue
		}
		if module, exists := moduleByContext[pkg.ID.Context]; exists && !delimitedPrefix(pkg.ID.Name, module.Name, "/") {
			return nil, nil, fmt.Errorf("%w: Go package %q is outside captured module %q for context %q", ErrBoundary,
				pkg.ID.Name, module.Name, module.Context)
		}
	}
	return packages, moduleByContext, nil
}

func matchesPackage(id packagegraph.PackageID, selector Selector, modules map[string]Module, roots *rootIdentities) bool {
	if id.Language != selector.Language {
		return false
	}
	if selector.PackagePrefix != "" {
		delimiter := "."
		if id.Language == "go" {
			delimiter = "/"
		}
		if !delimitedPrefix(id.Name, selector.PackagePrefix, delimiter) {
			return false
		}
	}
	if selector.GoModule != "" {
		module, exists := modules[id.Context]
		if id.Language != "go" || !exists || module.Name != selector.GoModule || !delimitedPrefix(id.Name, module.Name, "/") {
			return false
		}
	}
	return selector.SourceRoot == "" || roots.same(id.Context, selector.SourceRoot)
}

// EvalSymlinks does not normalize path casing on case-insensitive Unix
// filesystems. SameFile compares physical identity without case-folding paths
// on case-sensitive filesystems. Stat each distinct root at most once.
type rootIdentities struct {
	info map[string]os.FileInfo
}

func (r *rootIdentities) same(a, b string) bool {
	if a == b {
		return true
	}
	left, right := r.stat(a), r.stat(b)
	return left != nil && right != nil && os.SameFile(left, right)
}

func (r *rootIdentities) stat(path string) os.FileInfo {
	if r.info == nil {
		r.info = make(map[string]os.FileInfo)
	}
	if info, ok := r.info[path]; ok {
		return info
	}
	info, _ := os.Stat(path)
	r.info[path] = info
	return info
}

func delimitedPrefix(name, prefix, delimiter string) bool {
	return name == prefix || strings.HasPrefix(name, prefix+delimiter)
}

func compareProjectionPackage(a, b packagegraph.PackageID) int {
	if order := cmp.Compare(a.Language, b.Language); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Context, b.Context); order != 0 {
		return order
	}
	return cmp.Compare(a.Name, b.Name)
}

func compareProjectionID(a, b ID) int {
	if order := cmp.Compare(a.Kind, b.Kind); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Name, b.Name); order != 0 {
		return order
	}
	return compareProjectionPackage(a.Package, b.Package)
}
