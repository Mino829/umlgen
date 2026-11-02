package plantuml

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/relations"
)

// PackageOptions contains only layout controls supported by package diagrams.
// Graph visibility, relation selection and cycle scope come from Analyze's View.
type PackageOptions struct {
	Title           string
	LayoutDirection string
	SkinParams      map[string]string
}

func (opts PackageOptions) Validate() error {
	keys := make([]string, 0, len(opts.SkinParams))
	for key := range opts.SkinParams {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if key != "linetype" && key != "shadowing" {
			return fmt.Errorf("unsupported package skinparam %q: expected linetype or shadowing", key)
		}
	}
	return (Options{LayoutDirection: opts.LayoutDirection, SkinParams: opts.SkinParams}).ValidateLayout()
}

// GeneratePackage renders the display projection without recomputing cycles on
// the visible graph. Cycle annotations always refer to the selected analysis
// scope retained in View; they do not claim compiler/build/runtime findings.
func GeneratePackage(view packagegraph.View, opts PackageOptions) (string, error) {
	if err := opts.Validate(); err != nil {
		return "", err
	}
	if view.Scope.Source != packagegraph.DeclaredTypes && view.Scope.Source != packagegraph.Imports && view.Scope.Source != packagegraph.Both {
		return "", fmt.Errorf("package view has no selected dependency source")
	}
	if view.Scope.Cycles != packagegraph.CyclesOff && view.Scope.Cycles != packagegraph.CyclesMark && view.Scope.Cycles != packagegraph.CyclesOnly {
		return "", fmt.Errorf("package view has no selected cycle mode")
	}
	if view.Scope.PackageCount < len(view.Graph.Packages) || view.Scope.DependencyCount < len(view.Graph.Dependencies) {
		return "", fmt.Errorf("visible package graph exceeds its selected analysis scope")
	}
	packages := packageSorted(view.Graph.Packages, func(a, b packagegraph.Package) int { return compareDiagramPackageID(a.ID, b.ID) })
	aliases := make(map[packagegraph.PackageID]int, len(packages))
	names := make(map[string]int, len(packages))
	for n, pkg := range packages {
		if _, exists := aliases[pkg.ID]; exists {
			return "", fmt.Errorf("duplicate package identity in view")
		}
		if pkg.Kind != packagegraph.InternalPackage && pkg.Kind != packagegraph.ExternalPackage {
			return "", fmt.Errorf("unsupported package kind %q", pkg.Kind)
		}
		aliases[pkg.ID] = n
		names[pkg.ID.Name]++
	}
	cycles := view.Cycles
	ownedCycles := false
	for n, cycle := range cycles {
		if cycle.Kind != packagegraph.DeclaredCycle && cycle.Kind != packagegraph.CandidateCycle {
			return "", fmt.Errorf("unsupported cycle kind %q", cycle.Kind)
		}
		if !slices.IsSortedFunc(cycle.Packages, compareDiagramPackageID) {
			if !ownedCycles {
				cycles = slices.Clone(cycles)
				ownedCycles = true
			}
			cycles[n].Packages = packageSorted(cycle.Packages, compareDiagramPackageID)
		}
	}
	if ownedCycles {
		slices.SortFunc(cycles, compareDiagramCycle)
	} else {
		cycles = packageSorted(cycles, compareDiagramCycle)
	}
	membership := make(map[packagegraph.PackageID]packagegraph.CycleKind)
	if view.Scope.Cycles != packagegraph.CyclesOff {
		for _, cycle := range cycles {
			for _, id := range cycle.Packages {
				if membership[id] != packagegraph.DeclaredCycle {
					membership[id] = cycle.Kind
				}
			}
		}
	}
	var b strings.Builder
	writePreamble(&b, packageText(opts.Title), Options{LayoutDirection: opts.LayoutDirection, SkinParams: opts.SkinParams})
	b.WriteByte('\n')
	if len(packages) == 0 {
		b.WriteString("note \"No packages visible under the selected scope\" as NoPackages\n")
	}
	for _, pkg := range packages {
		name := packageDisplayName(pkg.ID)
		label := packageText(name)
		if names[pkg.ID.Name] > 1 {
			label += "\\n" + packageText(pkg.ID.Language+" / "+pkg.ID.Context)
		}
		color := ""
		if pkg.Kind == packagegraph.ExternalPackage {
			label += "\\n[external import path]"
			color = " #E7EDF4"
		}
		switch membership[pkg.ID] {
		case packagegraph.DeclaredCycle:
			label += "\\n[declared cycle in scope]"
			color = " #F8D7DA"
		case packagegraph.CandidateCycle:
			label += "\\n[candidate cycle in scope]"
			color = " #FFF3CD"
		}
		fmt.Fprintf(&b, "package \"%s\" as PKG_%d%s {\n}\n", label, aliases[pkg.ID], color)
	}
	edges := packageSorted(view.Graph.Dependencies, func(a, b packagegraph.Dependency) int {
		if order := compareDiagramPackageID(a.From, b.From); order != 0 {
			return order
		}
		return compareDiagramPackageID(a.To, b.To)
	})
	for _, edge := range edges {
		from, fromOK := aliases[edge.From]
		to, toOK := aliases[edge.To]
		if !fromOK || !toOK {
			return "", fmt.Errorf("package dependency has an endpoint outside the visible graph")
		}
		if edge.From == edge.To {
			return "", fmt.Errorf("package dependency contains a same-package edge")
		}
		inferred := 0
		for _, relation := range edge.TypeRelations {
			if inferredOnly(relation) {
				inferred++
			}
		}
		arrow := "-->"
		if len(edge.Imports) == 0 && inferred == len(edge.TypeRelations) {
			arrow = "..>"
		}
		fmt.Fprintf(&b, "PKG_%d %s PKG_%d : types=%d; imports=%d", from, arrow, to, len(edge.TypeRelations), len(edge.Imports))
		if inferred != 0 {
			fmt.Fprintf(&b, "; inferred-only=%d", inferred)
		}
		b.WriteByte('\n')
	}
	writePackageLegend(&b, view, cycles)
	b.WriteString("\n@enduml\n")
	return b.String(), nil
}

func inferredOnly(relation relations.Relation) bool {
	if len(relation.Evidence) == 0 {
		return false
	}
	for _, evidence := range relation.Evidence {
		if !evidence.Inferred {
			return false
		}
	}
	return true
}

func writePackageLegend(b *strings.Builder, view packagegraph.View, cycles []packagegraph.Cycle) {
	b.WriteString("\nlegend bottom\n")
	fmt.Fprintf(b, "Dependencies: %s; types = declaration relations, imports = source declarations\n", view.Scope.Source)
	fmt.Fprintf(b, "Selected analysis scope: %d packages, %d dependencies\n", view.Scope.PackageCount, view.Scope.DependencyCount)
	fmt.Fprintf(b, "Visible: %d packages, %d dependencies; hidden: %d packages, %d dependencies\n", len(view.Graph.Packages), len(view.Graph.Dependencies), view.Scope.PackageCount-len(view.Graph.Packages), view.Scope.DependencyCount-len(view.Graph.Dependencies))
	relationsText := "all"
	if view.Scope.Relations != nil {
		if len(view.Scope.Relations) == 0 {
			relationsText = "none"
		} else {
			selected := slices.Clone(view.Scope.Relations)
			slices.Sort(selected)
			var names []string
			for _, kind := range selected {
				names = append(names, string(kind))
			}
			relationsText = strings.Join(names, ", ")
		}
	}
	fmt.Fprintf(b, "Type relation selection: %s\n", packageText(relationsText))
	if view.Scope.Focus != nil {
		fmt.Fprintf(b, "Focus: %s\nContext: %s\nHops: depth=%d; direction=%s\n", packageText(packageDisplayName(*view.Scope.Focus)), packageText(view.Scope.Focus.Context), view.Scope.Depth, packageText(string(view.Scope.Direction)))
	}
	external := "hidden"
	if view.Scope.ShowExternal {
		external = "shown"
	}
	fmt.Fprintf(b, "External import paths: %s; existence/types/build compatibility unverified\n", external)
	if view.Scope.Cycles == packagegraph.CyclesOff {
		b.WriteString("Cycle analysis: off (not analyzed)\n")
	} else {
		declared, candidate := 0, 0
		for _, cycle := range cycles {
			if cycle.Kind == packagegraph.DeclaredCycle {
				declared++
			} else {
				candidate++
			}
		}
		fmt.Fprintf(b, "Cycles in selected analysis scope: declared=%d; candidate=%d\n", declared, candidate)
		b.WriteString("Cycle groups are SCC member sets, not ordered call paths\n")
		for n, cycle := range cycles {
			fmt.Fprintf(b, "%s SCC %d:\n", cycle.Kind, n+1)
			for _, id := range cycle.Packages {
				fmt.Fprintf(b, "  %s\n    context: %s\n", packageText(packageDisplayName(id)), packageText(id.Context))
			}
		}
		b.WriteString("Cycle analysis precedes focus/external hiding; omitted edges may hide the cycle\n")
	}
	unresolved, ambiguous, failed, unreadable, unsupported := 0, 0, 0, 0, 0
	for _, diagnostic := range view.Graph.Diagnostics {
		switch diagnostic.Code {
		case "syntax-error":
			failed++
		case "source-read-error":
			unreadable++
		case "unsupported-module-declaration":
			unsupported++
		default:
			if diagnostic.Status == packagegraph.Ambiguous {
				ambiguous++
			} else {
				unresolved++
			}
		}
	}
	fmt.Fprintf(b, "Selected-scope diagnostics: unresolved=%d; ambiguous=%d; syntax failures=%d; read failures=%d; unsupported=%d\n", unresolved, ambiguous, failed, unreadable, unsupported)
	b.WriteString("Incomplete/unresolved analysis is not evidence of absent dependencies\n")
	b.WriteString("Solid arrows have declaration/import proof; dashed arrows are inferred-only\n")
	b.WriteString("Source model only: not compilation, Go import-cycle validation, or runtime behavior\n")
	b.WriteString("Build tags/classpath and unselected sources are not evaluated\nendlegend\n")
}

func packageDisplayName(id packagegraph.PackageID) string {
	if id.Name == "" {
		return "(unnamed package)"
	}
	return id.Name
}

// Unicode text escapes keep user punctuation outside preprocessing, quoted-name
// syntax, Creole links/images and HTML tags. Controls/line breaks become spaces.
func packageText(text string) string {
	var b strings.Builder
	const hex = "0123456789ABCDEF"
	for _, r := range text {
		switch {
		case unicode.IsControl(r) || unicode.IsSpace(r):
			b.WriteByte(' ')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteString("<U+")
			width := 4
			if r > 0xffff {
				width = 6
			}
			for shift := (width - 1) * 4; shift >= 0; shift -= 4 {
				b.WriteByte(hex[(uint32(r)>>uint(shift))&15])
			}
			b.WriteByte('>')
		}
	}
	return b.String()
}

func compareDiagramPackageID(a, b packagegraph.PackageID) int {
	if order := cmp.Compare(a.Language, b.Language); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Context, b.Context); order != 0 {
		return order
	}
	return cmp.Compare(a.Name, b.Name)
}

func compareDiagramCycle(a, b packagegraph.Cycle) int {
	if a.Kind != b.Kind {
		if a.Kind == packagegraph.DeclaredCycle {
			return -1
		}
		return 1
	}
	return slices.CompareFunc(a.Packages, b.Packages, compareDiagramPackageID)
}

// Already-normalized analysis output is borrowed read-only; only unsorted input
// needs storage for normalization. Callers never observe their slices reordered.
func packageSorted[S ~[]E, E any](items S, compare func(E, E) int) S {
	if slices.IsSortedFunc(items, compare) {
		return items
	}
	items = slices.Clone(items)
	slices.SortFunc(items, compare)
	return items
}
