package explain

import (
	"fmt"
	"strings"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/relations"
)

// PackageReport combines the unfiltered parsed graph with its selected view and
// the exact source snapshots captured during analysis. Formatting performs no IO.
type PackageReport struct {
	Scope    Scope
	Sources  map[string]Source
	Failures []Failure
	Known    packagegraph.Graph
	View     packagegraph.View
}

// FormatPackage explains visible dependencies without loading source or external
// packages. Known identities and selected-scope cycles survive display filtering.
func FormatPackage(report PackageReport) string {
	var out strings.Builder
	fmt.Fprintln(&out, "Package dependency explanations:")
	fmt.Fprintln(&out, "\nAnalysis scope:")
	fmt.Fprintf(&out, "  Targets: %s\n", listed(report.Scope.Targets, "(none supplied)"))
	fmt.Fprintf(&out, "  Selected files: %d\n", report.Scope.Files)
	fmt.Fprintf(&out, "  Language: %s\n", specified(report.Scope.Language, "(not specified)"))
	fmt.Fprintf(&out, "  Include: %s\n", specified(report.Scope.Include, "(no filter)"))
	fmt.Fprintf(&out, "  Exclude: %s\n", listed(report.Scope.Excludes, "(none)"))
	scope, graph := report.View.Scope, report.View.Graph
	fmt.Fprintf(&out, "  Dependency source: %s\n", specified(string(scope.Source), specified(string(graph.Source), "(not supplied)")))
	fmt.Fprint(&out, "  Type relation selection: ")
	if scope.Relations == nil {
		fmt.Fprintln(&out, "all kinds")
	} else if len(scope.Relations) == 0 {
		fmt.Fprintln(&out, "none (import declarations are independent)")
	} else {
		for i, kind := range scope.Relations {
			if i > 0 {
				fmt.Fprint(&out, ", ")
			}
			fmt.Fprint(&out, kind)
		}
		fmt.Fprintln(&out)
	}
	fmt.Fprintf(&out, "  Known parsed graph: %d packages, %d dependencies before package/display filtering.\n", len(report.Known.Packages), len(report.Known.Dependencies))
	fmt.Fprintf(&out, "  Selected analysis scope: %d packages, %d dependencies after package/relation selection, before focus/external/cycle-only filtering.\n", scope.PackageCount, scope.DependencyCount)
	fmt.Fprintf(&out, "  Visible diagram: %d packages, %d dependencies.\n", len(graph.Packages), len(graph.Dependencies))
	if scope.Focus == nil {
		fmt.Fprintln(&out, "  Focus: (none)")
	} else {
		fmt.Fprintf(&out, "  Focus: %s; depth=%d; direction=%s\n", packageIdentity(*scope.Focus), scope.Depth, scope.Direction)
	}
	fmt.Fprintf(&out, "  External packages shown: %t; cycles=%s\n", scope.ShowExternal, specified(string(scope.Cycles), "(not supplied)"))
	fmt.Fprintln(&out, "  Type relation counts count distinct relation records, not evidence locations. Import declaration counts count original file declarations, not type uses or runtime calls.")
	fmt.Fprintln(&out, "  Only selected source files are parsed. Build tags, Java classpath/module resolution, compiler validity and runtime behavior are unverified; function bodies and external dependency source bodies are not analyzed.")
	fmt.Fprintln(&out, "  No displayed dependency does not imply no dependencies: file/package selection, relation, focus, external and cycle filters can hide evidence.")

	if len(report.Failures) > 0 {
		fmt.Fprintln(&out, "\nAnalysis failures:")
		for _, failure := range report.Failures {
			fmt.Fprintf(&out, "  %s (%s): %s\n", specified(failure.Source, "(source unknown)"), origin(failure.Origin), failure.Message)
		}
		fmt.Fprintln(&out, "  Analysis is incomplete. Failed inputs cannot establish the absence of dependencies.")
	}
	if len(graph.Packages) == 0 {
		fmt.Fprintln(&out, "\nNo packages are visible in this result; empty or failed analysis is not proof of dependency absence.")
	} else if len(graph.Dependencies) == 0 {
		fmt.Fprintln(&out, "\nNo inter-package dependencies are visible in this result; this does not establish dependency absence.")
	}

	visible := make(map[packagegraph.PackageID]bool, len(graph.Packages))
	files := make(map[string]packagegraph.File)
	for _, pkg := range report.Known.Packages {
		for _, file := range pkg.Files {
			files[file.Path] = file
		}
	}
	if len(graph.Packages) > 0 {
		fmt.Fprintln(&out, "\nVisible packages:")
	}
	for _, pkg := range graph.Packages {
		visible[pkg.ID] = true
		fmt.Fprintf(&out, "  %s [%s]\n", packageIdentity(pkg.ID), pkg.Kind)
		if pkg.Kind == packagegraph.ExternalPackage {
			fmt.Fprintln(&out, "    External identity only; source body not loaded, package/build availability unverified.")
		}
		for _, file := range pkg.Files {
			files[file.Path] = file
			fmt.Fprint(&out, "    ")
			packageSource(&out, file, model.SourceSpan{}, 0, report.Sources)
			fmt.Fprintln(&out)
		}
	}

	renderer := sourceRenderer{sources: report.Sources, lines: make(map[string][]string)}
	type dependencyID struct{ from, to packagegraph.PackageID }
	visibleDependencies := make(map[dependencyID]bool, len(graph.Dependencies))
	for _, dependency := range graph.Dependencies {
		visibleDependencies[dependencyID{dependency.From, dependency.To}] = true
		fmt.Fprintf(&out, "\n%s -> %s\n", packageIdentity(dependency.From), packageIdentity(dependency.To))
		fmt.Fprintf(&out, "  Type relations: %d; import declarations: %d\n", len(dependency.TypeRelations), len(dependency.Imports))
		for _, relation := range dependency.TypeRelations {
			fmt.Fprintf(&out, "  Declared-type relation: %s -> %s [%s", relation.From, relation.To, relation.Kind)
			if relation.Label != "" {
				fmt.Fprintf(&out, ", label=%q", relation.Label)
			}
			if relation.Multiplicity != "" {
				fmt.Fprintf(&out, ", multiplicity=%s", relation.Multiplicity)
			}
			fmt.Fprintf(&out, "]; evidence locations=%d\n", len(relation.Evidence))
			if len(relation.Evidence) == 0 {
				fmt.Fprintln(&out, "    Declaration evidence was not supplied.")
			}
			for _, evidence := range relation.Evidence {
				file := files[evidence.Source]
				file.Path = evidence.Source
				fmt.Fprint(&out, "    ")
				packageSource(&out, file, model.SourceSpan{}, evidence.Line, report.Sources)
				fmt.Fprintf(&out, ": %s\n", reason(relation, evidence))
				renderer.snippet(&out, evidence)
			}
		}
		for _, evidence := range dependency.Imports {
			imported := evidence.Import
			fmt.Fprintf(&out, "  Import declaration: %q", imported.Name)
			if imported.Static {
				fmt.Fprint(&out, " [static]")
			}
			if imported.Wildcard {
				fmt.Fprint(&out, " [wildcard]")
			}
			if dependency.From.Language == "go" {
				switch imported.Alias {
				case "_":
					fmt.Fprint(&out, " alias=\"_\" (blank import: initialization intent, no type qualifier; runtime effects unverified)")
				case ".":
					fmt.Fprint(&out, " alias=\".\" (dot import: unqualified names; not proof of any particular type use)")
				case "":
					fmt.Fprint(&out, " (no explicit alias; external declared package name is not inferred from the path)")
				default:
					fmt.Fprintf(&out, " alias=%q (explicit local qualifier)", imported.Alias)
				}
			}
			fmt.Fprint(&out, " at ")
			packageSource(&out, evidence.File, imported.Span, 0, report.Sources)
			fmt.Fprintln(&out)
			renderer.snippet(&out, relations.Evidence{Source: evidence.File.Path, Line: imported.Span.Start.Line})
		}
	}

	hiddenHeading := false
	for _, pkg := range report.Known.Packages {
		if visible[pkg.ID] {
			continue
		}
		if !hiddenHeading {
			fmt.Fprintln(&out, "\nKnown packages outside diagram (package or display filters, not unresolved identities):")
			hiddenHeading = true
		}
		fmt.Fprintf(&out, "  %s [%s, hidden]\n", packageIdentity(pkg.ID), pkg.Kind)
	}
	hiddenHeading = false
	for _, dependency := range report.Known.Dependencies {
		if visibleDependencies[dependencyID{dependency.From, dependency.To}] {
			continue
		}
		if !hiddenHeading {
			fmt.Fprintln(&out, "\nKnown dependencies outside diagram (package/relation/display filters; evidence excerpts omitted):")
			hiddenHeading = true
		}
		fmt.Fprintf(&out, "  %s -> %s [hidden]\n", packageIdentity(dependency.From), packageIdentity(dependency.To))
	}

	formatPackageDiagnostics(&out, report, &renderer)
	fmt.Fprintln(&out, "\nCycles in selected analysis scope (before focus/display filtering):")
	if scope.Cycles == packagegraph.CyclesOff {
		fmt.Fprintln(&out, "  Cycle detection is off; acyclicity was not assessed.")
	} else if len(report.View.Cycles) == 0 {
		fmt.Fprintln(&out, "  No cycle groups found in the selected graph; this is not a compiler/build finding or a whole-repository claim.")
	}
	for _, cycle := range report.View.Cycles {
		fmt.Fprintf(&out, "  %s cycle group (strongly connected packages, not a traversal path):\n", cycle.Kind)
		hidden := 0
		for _, id := range cycle.Packages {
			state := "visible"
			if !visible[id] {
				state = "hidden by focus/display filters"
				hidden++
			}
			fmt.Fprintf(&out, "    %s [%s]\n", packageIdentity(id), state)
		}
		fmt.Fprintf(&out, "    Hidden members: %d; classification uses the full selected scope, not only visible edges.\n", hidden)
		if cycle.Kind == packagegraph.CandidateCycle {
			fmt.Fprintln(&out, "    Candidate cycle requires inferred implementation evidence; syntactic method-set matching is not compiler-verified.")
		}
	}
	return out.String()
}

func packageIdentity(id packagegraph.PackageID) string {
	return fmt.Sprintf("%s %s::%s", specified(id.Language, "(language unknown)"), specified(id.Context, "(context unknown)"), specified(id.Name, "(unnamed package)"))
}

func packageSource(out *strings.Builder, file packagegraph.File, span model.SourceSpan, line int, sources map[string]Source) {
	fmt.Fprint(out, specified(file.Path, "(source unknown)"))
	if span.Start.Line > 0 {
		fmt.Fprintf(out, ":%d:%d-%d:%d [bytes %d:%d, half-open]", span.Start.Line, span.Start.Column, span.End.Line, span.End.Column, span.Start.Offset, span.End.Offset)
	} else if line > 0 {
		fmt.Fprintf(out, ":%d", line)
	}
	fmt.Fprintf(out, " (origin=%s; hash=%s)", origin(sources[file.Path].Origin), specified(file.ContentHash, "unknown"))
}

func formatPackageDiagnostics(out *strings.Builder, report PackageReport, renderer *sourceRenderer) {
	for _, category := range []struct {
		heading string
		failure bool
	}{
		{"Source analysis diagnostics (not unresolved dependencies)", true},
		{"Unresolved or ambiguous references in selected scope", false},
	} {
		shown := false
		for _, diagnostic := range report.View.Graph.Diagnostics {
			isFailure := diagnostic.Code == "syntax-error" || diagnostic.Code == "source-read-error" || diagnostic.Code == "unsupported-module-declaration"
			if isFailure != category.failure {
				continue
			}
			if !shown {
				fmt.Fprintf(out, "\n%s:\n", category.heading)
				shown = true
			}
			fmt.Fprintf(out, "  %s [%s", packageIdentity(diagnostic.From), diagnostic.Code)
			if !isFailure {
				fmt.Fprintf(out, ", %s", diagnostic.Status)
			}
			fmt.Fprint(out, "]")
			if diagnostic.Reference != "" {
				fmt.Fprintf(out, " reference=%q", diagnostic.Reference)
			}
			if diagnostic.Owner != "" {
				fmt.Fprintf(out, " owner=%s", diagnostic.Owner)
			}
			if diagnostic.Kind != "" {
				fmt.Fprintf(out, " kind=%s", diagnostic.Kind)
			}
			if diagnostic.Member != "" {
				fmt.Fprintf(out, " member=%q", diagnostic.Member)
			}
			fmt.Fprint(out, " at ")
			packageSource(out, diagnostic.File, diagnostic.Span, diagnostic.Line, report.Sources)
			fmt.Fprintln(out)
			line := diagnostic.Line
			if diagnostic.Span.Start.Line > 0 {
				line = diagnostic.Span.Start.Line
			}
			if line > 0 {
				renderer.snippet(out, relations.Evidence{Source: diagnostic.File.Path, Line: line})
			}
			switch diagnostic.Code {
			case "syntax-error":
				fmt.Fprintln(out, "    Parsing failed; see the actual failure message above when supplied. This is not an ordinary unresolved reference.")
			case "source-read-error":
				fmt.Fprintln(out, "    Source could not be read; no captured content or content hash is established. This is not a syntax failure.")
			case "unsupported-module-declaration":
				fmt.Fprintln(out, "    Java module declarations are unsupported; module dependencies were not analyzed.")
			}
			for _, candidate := range diagnostic.Candidates {
				fmt.Fprintf(out, "    Candidate: %s; type=%s\n", packageIdentity(candidate.Package), candidate.Type)
			}
		}
	}
}
