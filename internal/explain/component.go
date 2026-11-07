package explain

import (
	"fmt"
	"strings"

	"github.com/Mino829/umlgen/internal/component"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/relations"
)

type ComponentReport struct {
	Graph    component.Graph
	Scope    Scope
	Sources  map[string]Source
	Failures []Failure
}

func FormatComponent(report ComponentReport) string {
	var out strings.Builder
	g := report.Graph
	fmt.Fprintln(&out, "Component boundary explanation (configured ownership, not runtime architecture):")
	fmt.Fprintf(&out, "Definition: %s\nDefinition SHA256: %s\nTargets: %s\nLanguage: %s; selected files: %d\nExclude: %s\nDependency source: %s\n", g.Definition.Source, g.Definition.Hash, listed(report.Scope.Targets, "none"), report.Scope.Language, report.Scope.Files, listed(report.Scope.Excludes, "none"), g.Source.Source)
	fmt.Fprintln(&out, "Boundary ownership and provides/requires are explicit declarations. Equal API names do not bind providers. Source package dependencies do not prove runtime calls, responsibility, deployment or network traffic.")
	fmt.Fprintln(&out, "Type relation records, evidence locations and import declarations are separate counting units. Inferred implementation evidence remains inferred. Failed/unresolved/hidden inputs cannot prove dependency absence.")
	for _, failure := range report.Failures {
		fmt.Fprintf(&out, "Source failure: %s (%s): %s\n", failure.Source, origin(failure.Origin), failure.Message)
	}
	for _, module := range g.Modules {
		fmt.Fprintf(&out, "Captured Go module: %s; context=%s; file=%s; SHA256=%s (lexical go.mod metadata, not a build/workspace graph)\n", module.Name, module.Context, module.File, module.Hash)
	}
	out.WriteString("\nConfigured boundaries:\n")
	for _, b := range g.Definition.Components {
		fmt.Fprintf(&out, "  %s [configured %s:%d]\n", b.Name, g.Definition.Source, b.Line)
		for _, s := range b.Match {
			fmt.Fprintf(&out, "    selector [line %d]: language=%s; package_prefix=%q; source_root=%q; go_module=%q\n", s.Line, s.Language, s.PackagePrefix, s.SourceRoot, s.GoModule)
		}
		fmt.Fprintf(&out, "    configured provides: %s\n    configured requires (provider binding unspecified): %s\n", listed(b.Provides, "none"), listed(b.Requires, "none"))
	}
	files := make(map[string]packagegraph.File)
	for _, pkg := range g.Source.Packages {
		for _, file := range pkg.Files {
			files[file.Path] = file
		}
	}
	renderer := sourceRenderer{sources: report.Sources, lines: make(map[string][]string)}
	out.WriteString("\nDisplayed ownership and package provenance:\n")
	for _, node := range g.Nodes {
		fmt.Fprintf(&out, "  %s [%s]; %d source package identities\n", specified(node.Name, "(default package)"), node.ID.Kind, len(node.Packages))
		for _, pkg := range node.Packages {
			fmt.Fprintf(&out, "    %s [%s]\n", packageIdentity(pkg.ID), pkg.Kind)
			for _, file := range pkg.Files {
				out.WriteString("      ")
				packageSource(&out, file, model.SourceSpan{}, 0, report.Sources)
				out.WriteByte('\n')
			}
		}
		if len(node.Internal) > 0 {
			fmt.Fprintln(&out, "    Internal package dependencies (retained evidence; no component self-arrow):")
			for i := range node.Internal {
				componentOriginal(&out, &node.Internal[i], files, &renderer, report.Sources)
			}
		}
	}
	out.WriteString("\nInter-boundary source dependencies:\n")
	for _, edge := range g.Dependencies {
		fmt.Fprintf(&out, "  %s -> %s\n", componentEndpoint(edge.From), componentEndpoint(edge.To))
		for i := range edge.Originals {
			componentOriginal(&out, &edge.Originals[i], files, &renderer, report.Sources)
		}
	}
	out.WriteString("\nHidden external dependencies (display filter only):\n")
	for i := range g.Hidden {
		componentOriginal(&out, &g.Hidden[i], files, &renderer, report.Sources)
	}
	out.WriteString("\nBoundary diagnostics:\n")
	for _, d := range g.Diagnostics {
		fmt.Fprintf(&out, "  %s: %s; component=%s [configured %s:%d]", d.Code, d.Message, specified(d.Component, "none"), g.Definition.Source, d.Line)
		if d.Package != nil {
			fmt.Fprintf(&out, "; package=%s", packageIdentity(*d.Package))
		}
		out.WriteByte('\n')
	}
	out.WriteString("\nOriginal source diagnostics (no invented unresolved endpoints):\n")
	for _, d := range g.Source.Diagnostics {
		fmt.Fprintf(&out, "  %s [%s, %s]; reference=%s; owner=%s; member=%s\n    ", packageIdentity(d.From), d.Code, d.Status, d.Reference, d.Owner, d.Member)
		packageSource(&out, d.File, d.Span, d.Line, report.Sources)
		out.WriteByte('\n')
		for _, candidate := range d.Candidates {
			fmt.Fprintf(&out, "    unresolved alternative: %s::%s\n", packageIdentity(candidate.Package), candidate.Type)
		}
		line := d.Line
		if line == 0 {
			line = d.Span.Start.Line
		}
		renderer.snippet(&out, relations.Evidence{Source: d.File.Path, Line: line})
	}
	return out.String()
}

func componentEndpoint(id component.ID) string {
	if id.Kind == "component" {
		return id.Name + " [configured component]"
	}
	return packageIdentity(id.Package) + " [" + id.Kind + "]"
}

func componentOriginal(out *strings.Builder, d *packagegraph.Dependency, files map[string]packagegraph.File, r *sourceRenderer, sources map[string]Source) {
	fmt.Fprintf(out, "    Original package edge: %s -> %s\n    Type relation records: %d; import declarations: %d\n", packageIdentity(d.From), packageIdentity(d.To), len(d.TypeRelations), len(d.Imports))
	for _, relation := range d.TypeRelations {
		fmt.Fprintf(out, "      %s -> %s [%s]; label=%q; multiplicity=%q; evidence locations=%d\n", relation.From, relation.To, relation.Kind, relation.Label, relation.Multiplicity, len(relation.Evidence))
		for _, e := range relation.Evidence {
			file := files[e.Source]
			file.Path = e.Source
			out.WriteString("        ")
			packageSource(out, file, model.SourceSpan{}, e.Line, sources)
			fmt.Fprintf(out, ": %s; inferred=%t\n", reason(relation, e), e.Inferred)
			r.snippet(out, e)
		}
	}
	for _, e := range d.Imports {
		fmt.Fprintf(out, "      Import declaration: %q; alias=%q; static=%t; wildcard=%t at ", e.Import.Name, e.Import.Alias, e.Import.Static, e.Import.Wildcard)
		packageSource(out, e.File, e.Import.Span, 0, sources)
		out.WriteByte('\n')
		r.snippet(out, relations.Evidence{Source: e.File.Path, Line: e.Import.Span.Start.Line})
	}
}
