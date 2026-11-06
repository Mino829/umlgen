package explain

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/packagediff"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/relations"
)

// PackageDiffReport retains each immutable side's captured sources and analysis
// failures. Formatting never opens source paths or resolves Git revisions.
type PackageDiffReport struct {
	Diff          packagediff.Result
	Before, After PackageReport
}

// FormatPackageDiff explains semantic evidence deltas and each side's located
// evidence. Display hiding never reclassifies the full comparison's changes.
func FormatPackageDiff(report PackageDiffReport) string {
	var out strings.Builder
	diff := report.Diff
	fmt.Fprintln(&out, "Package dependency changes:")
	fmt.Fprintf(&out, "Before Git commit: %q (complete=%t)\nAfter Git commit: %q (complete=%t)\n", diff.Before.Revision, diff.Before.Complete, diff.After.Revision, diff.After.Complete)
	fmt.Fprintln(&out, "Immutable Git trees only: dirty, staged and untracked content is excluded. Current invocation configuration is applied identically to both snapshots.")
	fmt.Fprintln(&out, "Comparison precedes external/cycle-only display hiding; hidden evidence does not become a removed dependency. Unchanged dependencies are context.")
	fmt.Fprintln(&out, "Semantic evidence changes ignore content hashes, byte spans and line shifts; source-file identity and declaration meaning are retained. A modified edge remains an edge, not a removal/addition.")
	if !diff.Before.Complete || !diff.After.Complete {
		fmt.Fprintln(&out, "WARNING: incomplete analysis; uncertain changes cannot establish dependency addition/removal or cycle resolution.")
	}
	// Compare/Project already supply deterministic change and proof ordering.
	packages := diff.Packages
	fmt.Fprintf(&out, "\nVisible before/after union: %d packages, %d dependencies.\n", len(packages), len(diff.Dependencies))
	for _, change := range packages {
		fmt.Fprintf(&out, "  %s [%s]; before=%t; after=%t\n", packageIdentity(change.ID), diffExplanationStatus(change.Kind, false), change.Before != nil, change.After != nil)
		if change.Before != nil && change.Before.Kind == packagegraph.ExternalPackage || change.After != nil && change.After.Kind == packagegraph.ExternalPackage {
			fmt.Fprintln(&out, "    External import-path identity only; availability and build compatibility are unverified.")
		}
	}
	before, after := packageDiffSide(report.Before, diff, false), packageDiffSide(report.After, diff, true)
	beforeRenderer := sourceRenderer{sources: before.Sources, lines: make(map[string][]string)}
	afterRenderer := sourceRenderer{sources: after.Sources, lines: make(map[string][]string)}
	beforeFiles, afterFiles := packageDiffFiles(before), packageDiffFiles(after)
	for _, change := range diff.Dependencies {
		fmt.Fprintf(&out, "\n%s -> %s [%s]\n", packageIdentity(change.From), packageIdentity(change.To), diffExplanationStatus(change.Kind, true))
		beforeTypes, beforeImports, afterTypes, afterImports := 0, 0, 0, 0
		if change.Before != nil {
			beforeTypes, beforeImports = len(change.Before.TypeRelations), len(change.Before.Imports)
		}
		if change.After != nil {
			afterTypes, afterImports = len(change.After.TypeRelations), len(change.After.Imports)
		}
		fmt.Fprintf(&out, "  Before: type relations=%d; import declarations=%d. After: type relations=%d; import declarations=%d.\n", beforeTypes, beforeImports, afterTypes, afterImports)
		formatPackageDiffEvidence(&out, "Removed evidence (before Git snapshot)", change.RemovedEvidence, before, beforeFiles, &beforeRenderer)
		formatPackageDiffEvidence(&out, "Added evidence (after Git snapshot)", change.AddedEvidence, after, afterFiles, &afterRenderer)
	}
	fmt.Fprintln(&out, "\nCycle changes in both full selected graphs (before display hiding):")
	if diff.Before.View.Scope.Cycles == packagegraph.CyclesOff || diff.After.View.Scope.Cycles == packagegraph.CyclesOff {
		fmt.Fprintln(&out, "  Cycle detection is off; no cycle comparison or acyclicity claim.")
	} else {
		fmt.Fprintf(&out, "  Before groups=%d; after groups=%d. SCC member sets are not traversal paths or compiler/build findings.\n", len(diff.Before.View.Cycles), len(diff.After.View.Cycles))
		for _, change := range diff.Cycles {
			cycle := change.After
			if cycle == nil {
				cycle = change.Before
			}
			if cycle == nil {
				fmt.Fprintln(&out, "  Cycle evidence was not supplied.")
				continue
			}
			kind := change.Kind
			if kind != packagediff.Unchanged && (!diff.Before.Complete || !diff.After.Complete) {
				kind = packagediff.Uncertain
			}
			fmt.Fprintf(&out, "  %s cycle group [%s]; observed before=%t; observed after=%t\n", cycle.Kind, diffExplanationStatus(kind, false), change.Before != nil, change.After != nil)
			members := cycle.Packages
			if !slices.IsSortedFunc(members, compareDiffPackageID) {
				members = slices.Clone(members)
				slices.SortFunc(members, compareDiffPackageID)
			}
			for _, id := range members {
				fmt.Fprintf(&out, "    %s\n", packageIdentity(id))
			}
		}
	}
	fmt.Fprintln(&out, "\nBEFORE snapshot: located evidence, scope, hidden identities and diagnostics")
	out.WriteString(FormatPackage(before))
	fmt.Fprintln(&out, "\nAFTER snapshot: located evidence, scope, hidden identities and diagnostics")
	out.WriteString(FormatPackage(after))
	return out.String()
}

// Preserve full-scope diagnostics/cycles, replacing only display membership with
// the already classified union projection's observed packages and dependencies.
func packageDiffSide(report PackageReport, diff packagediff.Result, after bool) PackageReport {
	snapshot := diff.Before
	if after {
		snapshot = diff.After
	}
	displayScope := report.View.Scope
	report.View = snapshot.View
	if displayScope.Source != "" {
		report.View.Scope.ShowExternal = displayScope.ShowExternal
		if snapshot.View.Scope.Cycles != packagegraph.CyclesOff {
			report.View.Scope.Cycles = displayScope.Cycles
		}
	}
	if report.Known.Packages == nil && report.Known.Dependencies == nil {
		report.Known = snapshot.View.Graph
	}
	report.View.Graph.Packages = nil
	report.View.Graph.Dependencies = nil
	for _, change := range diff.Packages {
		pkg := change.Before
		if after {
			pkg = change.After
		}
		if pkg != nil {
			report.View.Graph.Packages = append(report.View.Graph.Packages, *pkg)
		}
	}
	for _, change := range diff.Dependencies {
		edge := change.Before
		if after {
			edge = change.After
		}
		if edge != nil {
			report.View.Graph.Dependencies = append(report.View.Graph.Dependencies, *edge)
		}
	}
	if report.View.Scope.Cycles == packagegraph.CyclesOff {
		report.View.Cycles = nil
	}
	return report
}

func formatPackageDiffEvidence(out *strings.Builder, heading string, evidence []packagediff.Evidence, report PackageReport, files map[string]packagegraph.File, renderer *sourceRenderer) {
	if len(evidence) == 0 {
		return
	}
	fmt.Fprintf(out, "  %s: %d\n", heading, len(evidence))
	for _, item := range evidence {
		if item.Import != nil && item.Relation == nil {
			declaration := item.Import
			fmt.Fprintf(out, "    Import %q; alias=%q; static=%t; wildcard=%t at ", declaration.Import.Name, declaration.Import.Alias, declaration.Import.Static, declaration.Import.Wildcard)
			packageSource(out, declaration.File, declaration.Import.Span, 0, report.Sources)
			fmt.Fprintln(out)
			renderer.snippet(out, relations.Evidence{Source: declaration.File.Path, Line: declaration.Import.Span.Start.Line})
		} else if item.Relation != nil && item.Import == nil {
			relation := *item.Relation
			fmt.Fprintf(out, "    Type relation %s -> %s [%s]; label=%q; multiplicity=%q\n", relation.From, relation.To, relation.Kind, relation.Label, relation.Multiplicity)
			if len(relation.Evidence) == 0 {
				fmt.Fprintln(out, "      Declaration evidence location was not supplied.")
			}
			for _, location := range relation.Evidence {
				fmt.Fprint(out, "      ")
				file := files[location.Source]
				file.Path = location.Source
				packageSource(out, file, model.SourceSpan{}, location.Line, report.Sources)
				fmt.Fprintf(out, ": %s\n", reason(relation, location))
				renderer.snippet(out, location)
			}
		} else {
			fmt.Fprintln(out, "    Invalid evidence record: expected exactly one relation or import.")
		}
	}
}

func packageDiffFiles(report PackageReport) map[string]packagegraph.File {
	files := make(map[string]packagegraph.File)
	for _, graph := range []packagegraph.Graph{report.Known, report.View.Graph} {
		for _, pkg := range graph.Packages {
			for _, file := range pkg.Files {
				if _, known := files[file.Path]; !known {
					files[file.Path] = file
				}
			}
		}
	}
	return files
}

func diffExplanationStatus(kind packagediff.Kind, edge bool) string {
	if kind == packagediff.Unchanged {
		return "unchanged context"
	}
	if kind == packagediff.Modified && edge {
		return "modified evidence; existing edge retained"
	}
	if kind == packagediff.Uncertain {
		return "uncertain; incomplete analysis, not a confirmed addition/removal/resolution"
	}
	return string(kind)
}

func compareDiffPackageID(a, b packagegraph.PackageID) int {
	if order := cmp.Compare(a.Language, b.Language); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Context, b.Context); order != 0 {
		return order
	}
	return cmp.Compare(a.Name, b.Name)
}
