package plantuml

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Mino829/umlgen/internal/packagediff"
	"github.com/Mino829/umlgen/internal/packagegraph"
)

// GeneratePackageDiff renders the union projection supplied by packagediff.Project.
// Status and full-scope cycle findings are never recomputed from visible edges.
func GeneratePackageDiff(result packagediff.Result, opts PackageOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", err
	}
	for _, snapshot := range []packagediff.Snapshot{result.Before, result.After} {
		scope := snapshot.View.Scope
		if scope.Source != packagegraph.DeclaredTypes && scope.Source != packagegraph.Imports && scope.Source != packagegraph.Both {
			return "", fmt.Errorf("package diff has no selected dependency source")
		}
		if scope.Cycles != packagegraph.CyclesOff && scope.Cycles != packagegraph.CyclesMark && scope.Cycles != packagegraph.CyclesOnly {
			return "", fmt.Errorf("package diff has no selected cycle mode")
		}
		if scope.PackageCount < len(snapshot.View.Graph.Packages) || scope.DependencyCount < len(snapshot.View.Graph.Dependencies) {
			return "", fmt.Errorf("package diff graph exceeds its selected analysis scope")
		}
	}
	packages := packageSorted(result.Packages, func(a, b packagediff.PackageChange) int { return compareDiagramPackageID(a.ID, b.ID) })
	aliases := make(map[packagegraph.PackageID]int, len(packages))
	names := make(map[string]int, len(packages))
	for n, change := range packages {
		if _, exists := aliases[change.ID]; exists {
			return "", fmt.Errorf("duplicate package identity in diff")
		}
		if !validPackageDiffKind(change.Kind) || change.Before == nil && change.After == nil {
			return "", fmt.Errorf("invalid package change")
		}
		for _, pkg := range []*packagegraph.Package{change.Before, change.After} {
			if pkg != nil && (pkg.ID != change.ID || pkg.Kind != packagegraph.InternalPackage && pkg.Kind != packagegraph.ExternalPackage) {
				return "", fmt.Errorf("package change has inconsistent identity or kind")
			}
		}
		aliases[change.ID] = n
		names[change.ID.Name]++
	}
	edges := packageSorted(result.Dependencies, func(a, b packagediff.DependencyChange) int {
		if order := compareDiagramPackageID(a.From, b.From); order != 0 {
			return order
		}
		return compareDiagramPackageID(a.To, b.To)
	})
	for n, change := range edges {
		from, fromOK := aliases[change.From]
		to, toOK := aliases[change.To]
		if !fromOK || !toOK || from == to {
			return "", fmt.Errorf("package diff dependency has invalid visible endpoints")
		}
		if n > 0 && edges[n-1].From == change.From && edges[n-1].To == change.To {
			return "", fmt.Errorf("duplicate dependency in package diff")
		}
		if !validPackageDiffKind(change.Kind) || change.Before == nil && change.After == nil {
			return "", fmt.Errorf("invalid dependency change")
		}
		for side, edge := range []*packagegraph.Dependency{change.Before, change.After} {
			if edge == nil {
				continue
			}
			if edge.From != change.From || edge.To != change.To || side == 0 && (packages[from].Before == nil || packages[to].Before == nil) || side == 1 && (packages[from].After == nil || packages[to].After == nil) {
				return "", fmt.Errorf("package diff dependency disagrees with its side's endpoints")
			}
		}
	}
	cycles := make([]packagediff.CycleChange, len(result.Cycles))
	for n, change := range result.Cycles {
		if !validPackageDiffKind(change.Kind) || change.Before == nil && change.After == nil {
			return "", fmt.Errorf("invalid cycle change")
		}
		cycles[n] = change
		for side, cycle := range []*packagegraph.Cycle{change.Before, change.After} {
			if cycle == nil {
				continue
			}
			if cycle.Kind != packagegraph.DeclaredCycle && cycle.Kind != packagegraph.CandidateCycle {
				return "", fmt.Errorf("unsupported package diff cycle kind")
			}
			copy := *cycle
			copy.Packages = packageSorted(cycle.Packages, compareDiagramPackageID)
			if side == 0 {
				cycles[n].Before = &copy
			} else {
				cycles[n].After = &copy
			}
		}
	}
	slices.SortFunc(cycles, func(a, b packagediff.CycleChange) int {
		return compareDiagramCycle(*packageDiffCycle(a), *packageDiffCycle(b))
	})
	var out strings.Builder
	writePreamble(&out, packageText(opts.Title), Options{LayoutDirection: opts.LayoutDirection, SkinParams: opts.SkinParams})
	out.WriteByte('\n')
	if len(packages) == 0 {
		out.WriteString("note \"No packages visible in the before/after union\" as NoPackages\n")
	}
	for _, change := range packages {
		label := packageText(packageDisplayName(change.ID))
		if names[change.ID.Name] > 1 {
			label += "\\n" + packageText(change.ID.Language+" / "+change.ID.Context)
		}
		label += "\\n[" + packageDiffLabel(change.Kind, false) + "]"
		if change.Before != nil && change.Before.Kind == packagegraph.ExternalPackage || change.After != nil && change.After.Kind == packagegraph.ExternalPackage {
			label += "\\n[external import path; availability unverified]"
		}
		fmt.Fprintf(&out, "package \"%s\" as PKG_%d #%s {\n}\n", label, aliases[change.ID], packageDiffColor(change.Kind))
	}
	for _, change := range edges {
		style := ""
		if packageDiffInferred(change.Before) && packageDiffInferred(change.After) {
			style = ",dashed"
		}
		fmt.Fprintf(&out, "PKG_%d -[#%s%s]-> PKG_%d : [%s]; before: types=%d; imports=%d; after: types=%d; imports=%d", aliases[change.From], packageDiffEdgeColor(change.Kind), style, aliases[change.To], packageDiffLabel(change.Kind, true), packageDiffTypes(change.Before), packageDiffImports(change.Before), packageDiffTypes(change.After), packageDiffImports(change.After))
		if change.Kind == packagediff.Modified || change.Kind == packagediff.Uncertain {
			fmt.Fprintf(&out, "; evidence +%d / -%d", len(change.AddedEvidence), len(change.RemovedEvidence))
		}
		out.WriteByte('\n')
	}
	writePackageDiffLegend(&out, result, cycles)
	out.WriteString("\n@enduml\n")
	return out.String(), nil
}

func validPackageDiffKind(kind packagediff.Kind) bool {
	return kind == packagediff.Unchanged || kind == packagediff.Added || kind == packagediff.Removed || kind == packagediff.Modified || kind == packagediff.Uncertain
}

func packageDiffColor(kind packagediff.Kind) string {
	switch kind {
	case packagediff.Added:
		return "C6EFCE"
	case packagediff.Removed:
		return "F8D7DA"
	case packagediff.Modified:
		return "D6E9FA"
	case packagediff.Uncertain:
		return "FFF3CD"
	default:
		return "E2E3E5"
	}
}

// Pastel package fills are not legible as thin dependency strokes.
func packageDiffEdgeColor(kind packagediff.Kind) string {
	switch kind {
	case packagediff.Added:
		return "22863A"
	case packagediff.Removed:
		return "CB2431"
	case packagediff.Modified:
		return "005CC5"
	case packagediff.Uncertain:
		return "9A6700"
	default:
		return "57606A"
	}
}

func packageDiffLabel(kind packagediff.Kind, edge bool) string {
	switch kind {
	case packagediff.Unchanged:
		return "unchanged context"
	case packagediff.Added:
		return "added"
	case packagediff.Removed:
		return "removed"
	case packagediff.Modified:
		if edge {
			return "modified evidence; edge retained"
		}
		return "modified source membership"
	default:
		return "uncertain; incomplete analysis"
	}
}

func packageDiffTypes(edge *packagegraph.Dependency) int {
	if edge == nil {
		return 0
	}
	return len(edge.TypeRelations)
}

func packageDiffImports(edge *packagegraph.Dependency) int {
	if edge == nil {
		return 0
	}
	return len(edge.Imports)
}

func packageDiffInferred(edge *packagegraph.Dependency) bool {
	if edge == nil {
		return true
	}
	if len(edge.Imports) != 0 || len(edge.TypeRelations) == 0 {
		return false
	}
	for _, relation := range edge.TypeRelations {
		if !inferredOnly(relation) {
			return false
		}
	}
	return true
}

func packageDiffCycle(change packagediff.CycleChange) *packagegraph.Cycle {
	if change.After != nil {
		return change.After
	}
	return change.Before
}

func writePackageDiffLegend(out *strings.Builder, result packagediff.Result, cycles []packagediff.CycleChange) {
	out.WriteString("\nlegend bottom\n")
	fmt.Fprintf(out, "Before Git commit: %s\nAfter Git commit: %s\n", packageText(result.Before.Revision), packageText(result.After.Revision))
	out.WriteString("Immutable Git trees only; dirty, staged and untracked content excluded\nCurrent invocation configuration applies identically to both snapshots\n")
	for n, snapshot := range []packagediff.Snapshot{result.Before, result.After} {
		side := "Before"
		if n == 1 {
			side = "After"
		}
		packages, dependencies := 0, 0
		for _, change := range result.Packages {
			if n == 0 && change.Before != nil || n == 1 && change.After != nil {
				packages++
			}
		}
		for _, change := range result.Dependencies {
			if n == 0 && change.Before != nil || n == 1 && change.After != nil {
				dependencies++
			}
		}
		scope := snapshot.View.Scope
		fmt.Fprintf(out, "%s selected scope: %d packages, %d dependencies; visible: %d packages, %d dependencies; complete=%t\n", side, scope.PackageCount, scope.DependencyCount, packages, dependencies, snapshot.Complete)
		fmt.Fprintf(out, "%s dependency source: %s; diagnostics=%d\n", side, packageText(string(scope.Source)), len(snapshot.View.Graph.Diagnostics))
		relationNames := "all"
		if scope.Relations != nil {
			names := make([]string, 0, len(scope.Relations))
			for _, kind := range scope.Relations {
				names = append(names, string(kind))
			}
			slices.Sort(names)
			relationNames = strings.Join(names, ", ")
			if len(names) == 0 {
				relationNames = "none"
			}
		}
		fmt.Fprintf(out, "%s type relation selection: %s\n", side, packageText(relationNames))
	}
	fmt.Fprintf(out, "Visible union: %d packages, %d dependencies\n", len(result.Packages), len(result.Dependencies))
	out.WriteString("Green=added; red=removed; blue=modified; yellow=uncertain; gray=unchanged context\nTypes count relation records; imports count original declarations, separately per side\nModified evidence retains an existing edge; it is not edge removal/addition\nDashed arrows have inferred-only evidence on all observed sides\nUncertain changes do not establish additions, removals or cycle resolution\n")
	if result.Before.View.Scope.Cycles == packagegraph.CyclesOff || result.After.View.Scope.Cycles == packagegraph.CyclesOff {
		out.WriteString("Cycle analysis: off; no cycle comparison\n")
	} else {
		fmt.Fprintf(out, "Full selected-scope cycles: before=%d; after=%d\n", len(result.Before.View.Cycles), len(result.After.View.Cycles))
		for _, change := range cycles {
			kind := change.Kind
			if kind != packagediff.Unchanged && (!result.Before.Complete || !result.After.Complete) {
				kind = packagediff.Uncertain
			}
			cycle := packageDiffCycle(change)
			fmt.Fprintf(out, "Cycle group: %s; %s; before=%t; after=%t\n", cycle.Kind, packageDiffLabel(kind, false), change.Before != nil, change.After != nil)
			for _, id := range cycle.Packages {
				fmt.Fprintf(out, "  %s / %s / %s\n", packageText(id.Language), packageText(id.Context), packageText(packageDisplayName(id)))
			}
		}
		out.WriteString("Cycle groups are SCC member sets, not ordered paths or compiler findings\nCycle comparison uses both full selected graphs before display hiding\n")
	}
	out.WriteString("External identities are import paths only; availability/build compatibility unverified\nHidden packages/edges are display filters, not change reclassification\nIncomplete/unresolved analysis cannot prove dependency absence\nSource model only: not compilation, Go import-cycle validation, or runtime behavior\nBuild tags, Java classpath, unselected sources and function bodies are not evaluated\nendlegend\n")
}
