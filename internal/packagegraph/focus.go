package packagegraph

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/Mino829/umlgen/internal/relations"
)

// Analyze selects declaration evidence before computing cycles and focus hops.
// Its scope and cycle results describe that complete selection, not just the
// packages left visible in Graph. It does not assess compiler/build validity.
// Nested metadata slices may borrow input storage and must be treated as
// read-only. Analyze never mutates the input, including when normalizing order.
func Analyze(graph Graph, opts ViewOptions) (View, error) {
	if opts.Direction == "" {
		opts.Direction = DirectionBoth
	}
	if opts.Cycles == "" {
		opts.Cycles = CyclesMark
	}
	if err := validateViewOptions(graph, opts); err != nil {
		return View{}, err
	}
	packages, err := validateViewGraph(graph)
	if err != nil {
		return View{}, err
	}
	if opts.Focus != nil {
		kind, exists := packages[*opts.Focus]
		if !exists {
			return View{}, fmt.Errorf("focus package not found: %+v", *opts.Focus)
		}
		if kind == ExternalPackage && !opts.ShowExternal {
			return View{}, fmt.Errorf("focus package is external and hidden: %+v", *opts.Focus)
		}
	}

	selectedKinds := slices.Clone(opts.Relations)
	slices.Sort(selectedKinds)
	selectedKinds = slices.Compact(selectedKinds)
	selected := selectViewRelations(graph, selectedKinds)
	view := View{
		Graph: selected,
		Scope: AnalysisScope{
			Source: selected.Source, Relations: selectedKinds,
			PackageCount: len(selected.Packages), DependencyCount: len(selected.Dependencies),
			Depth: opts.Depth, Direction: opts.Direction, ShowExternal: opts.ShowExternal, Cycles: opts.Cycles,
		},
	}
	if opts.Cycles != CyclesOff {
		// Off deliberately skips SCC analysis; an empty result then says
		// nothing about whether the selected graph contains cycles.
		view.Cycles = findCycles(selected)
	}
	if opts.Focus != nil {
		focus := *opts.Focus
		view.Scope.Focus = &focus
	}

	visible := focusPackages(selected, opts.Focus, opts.Depth, opts.Direction)
	if opts.Cycles == CyclesOnly {
		members := make(map[PackageID]struct{})
		for _, cycle := range view.Cycles {
			for _, id := range cycle.Packages {
				members[id] = struct{}{}
			}
		}
		for id := range visible {
			if _, member := members[id]; !member {
				delete(visible, id)
			}
		}
	}
	if !opts.ShowExternal {
		for id, kind := range packages {
			if kind == ExternalPackage {
				delete(visible, id)
			}
		}
	}
	view.Graph.Packages = slices.DeleteFunc(view.Graph.Packages, func(pkg Package) bool {
		_, keep := visible[pkg.ID]
		return !keep
	})
	view.Graph.Dependencies = slices.DeleteFunc(view.Graph.Dependencies, func(dep Dependency) bool {
		_, from := visible[dep.From]
		_, to := visible[dep.To]
		return !from || !to
	})
	return view, nil
}

func validateViewOptions(graph Graph, opts ViewOptions) error {
	if opts.Depth < 0 {
		return fmt.Errorf("package focus depth must not be negative")
	}
	if opts.Direction != DirectionIn && opts.Direction != DirectionOut && opts.Direction != DirectionBoth {
		return fmt.Errorf("unsupported package direction %q", opts.Direction)
	}
	if opts.Cycles != CyclesOff && opts.Cycles != CyclesMark && opts.Cycles != CyclesOnly {
		return fmt.Errorf("unsupported package cycle mode %q", opts.Cycles)
	}
	if graph.Source == Imports && opts.Relations != nil {
		return fmt.Errorf("type relation selection is unavailable for an imports-only graph")
	}
	for _, kind := range opts.Relations {
		if !validViewRelationKind(kind) {
			return fmt.Errorf("unsupported package relation kind %q", kind)
		}
	}
	return nil
}

func validViewRelationKind(kind relations.Kind) bool {
	switch kind {
	case relations.Inheritance, relations.Implementation, relations.Field, relations.Parameter, relations.Return:
		return true
	default:
		return false
	}
}

func validateViewGraph(graph Graph) (map[PackageID]PackageKind, error) {
	if graph.Source != DeclaredTypes && graph.Source != Imports && graph.Source != Both {
		return nil, fmt.Errorf("unsupported dependency source %q", graph.Source)
	}
	packages := make(map[PackageID]PackageKind, len(graph.Packages))
	for _, pkg := range graph.Packages {
		if (pkg.ID.Language != "java" && pkg.ID.Language != "go") || pkg.ID.Context == "" || (pkg.ID.Language == "go" && pkg.ID.Name == "") {
			return nil, fmt.Errorf("invalid package identity: %+v", pkg.ID)
		}
		if pkg.Kind != InternalPackage && pkg.Kind != ExternalPackage {
			return nil, fmt.Errorf("unsupported package kind %q", pkg.Kind)
		}
		if _, exists := packages[pkg.ID]; exists {
			return nil, fmt.Errorf("duplicate package identity: %+v", pkg.ID)
		}
		packages[pkg.ID] = pkg.Kind
	}
	for _, dep := range graph.Dependencies {
		if _, exists := packages[dep.From]; !exists {
			return nil, fmt.Errorf("dependency source package not found: %+v", dep.From)
		}
		if _, exists := packages[dep.To]; !exists {
			return nil, fmt.Errorf("dependency target package not found: %+v", dep.To)
		}
		for _, relation := range dep.TypeRelations {
			if !validViewRelationKind(relation.Kind) {
				return nil, fmt.Errorf("unsupported package relation kind %q", relation.Kind)
			}
		}
	}
	// Parser diagnostics may have no corresponding package: a failed file is
	// retained as uncertainty, never materialized as a phantom graph vertex.
	return packages, nil
}

func selectViewRelations(graph Graph, kinds []relations.Kind) Graph {
	selected := Graph{Source: graph.Source, Packages: slices.Clone(graph.Packages)}
	for i := range selected.Packages {
		selected.Packages[i].Files = sortedViewMetadata(selected.Packages[i].Files, compareViewFile)
	}
	for _, dep := range graph.Dependencies {
		if dep.From == dep.To {
			continue
		}
		dep.TypeRelations = selectViewTypeRelations(dep.TypeRelations, kinds)
		if len(dep.TypeRelations) == 0 && len(dep.Imports) == 0 {
			continue
		}
		dep.Imports = sortedViewMetadata(dep.Imports, compareViewImport)
		selected.Dependencies = append(selected.Dependencies, dep)
	}
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Kind != "" && kinds != nil && !slices.Contains(kinds, diagnostic.Kind) {
			continue
		}
		diagnostic.Candidates = sortedViewMetadata(diagnostic.Candidates, compareTarget)
		selected.Diagnostics = append(selected.Diagnostics, diagnostic)
	}
	slices.SortFunc(selected.Packages, func(a, b Package) int { return comparePackageID(a.ID, b.ID) })
	slices.SortFunc(selected.Dependencies, func(a, b Dependency) int {
		if order := comparePackageID(a.From, b.From); order != 0 {
			return order
		}
		return comparePackageID(a.To, b.To)
	})
	slices.SortFunc(selected.Diagnostics, compareDiagnostic)
	return selected
}

// Already ordered metadata can be borrowed because the view treats it as
// immutable; normalization takes a copy only when it must change that order.
func sortedViewMetadata[T any](records []T, compare func(T, T) int) []T {
	if slices.IsSortedFunc(records, compare) {
		return records
	}
	records = slices.Clone(records)
	slices.SortFunc(records, compare)
	return records
}

func selectViewTypeRelations(records []relations.Relation, kinds []relations.Kind) []relations.Relation {
	owned := false
	if kinds != nil {
		firstExcluded := slices.IndexFunc(records, func(record relations.Relation) bool {
			return !slices.Contains(kinds, record.Kind)
		})
		if firstExcluded >= 0 {
			filtered := make([]relations.Relation, firstExcluded, len(records)-1)
			copy(filtered, records[:firstExcluded])
			for _, record := range records[firstExcluded+1:] {
				if slices.Contains(kinds, record.Kind) {
					filtered = append(filtered, record)
				}
			}
			records, owned = filtered, true
		}
	}
	if !slices.IsSortedFunc(records, compareRelation) {
		if !owned {
			records, owned = slices.Clone(records), true
		}
		slices.SortFunc(records, compareRelation)
	}
	for i := range records {
		if !slices.IsSortedFunc(records[i].Evidence, compareEvidence) {
			if !owned {
				records, owned = slices.Clone(records), true
			}
			evidence := slices.Clone(records[i].Evidence)
			slices.SortFunc(evidence, compareEvidence)
			records[i].Evidence = evidence
		}
	}
	return records
}

func compareViewFile(a, b File) int {
	if order := cmp.Compare(a.Path, b.Path); order != 0 {
		return order
	}
	return cmp.Compare(a.ContentHash, b.ContentHash)
}

func compareViewImport(a, b ImportEvidence) int {
	if order := compareViewFile(a.File, b.File); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Import.Span.Start.Offset, b.Import.Span.Start.Offset); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Import.Name, b.Import.Name); order != 0 {
		return order
	}
	return cmp.Compare(a.Import.Alias, b.Import.Alias)
}

func focusPackages(graph Graph, focus *PackageID, depth int, direction Direction) map[PackageID]struct{} {
	visible := make(map[PackageID]struct{}, len(graph.Packages))
	if focus == nil {
		for _, pkg := range graph.Packages {
			visible[pkg.ID] = struct{}{}
		}
		return visible
	}
	visible[*focus] = struct{}{}
	if depth == 0 {
		return visible
	}
	adjacent := make(map[PackageID][]PackageID, len(graph.Packages))
	for _, dep := range graph.Dependencies {
		if direction != DirectionIn {
			adjacent[dep.From] = append(adjacent[dep.From], dep.To)
		}
		if direction != DirectionOut {
			adjacent[dep.To] = append(adjacent[dep.To], dep.From)
		}
	}
	queue := make([]PackageID, 1, len(graph.Packages))
	queue[0] = *focus
	for start, hop := 0, 0; start < len(queue) && hop < depth; hop++ {
		end := len(queue)
		for ; start < end; start++ {
			for _, next := range adjacent[queue[start]] {
				if _, seen := visible[next]; seen {
					continue
				}
				visible[next] = struct{}{}
				queue = append(queue, next)
			}
		}
	}
	return visible
}
