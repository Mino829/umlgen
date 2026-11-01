// Package explain renders relationship evidence from captured analysis snapshots.
package explain

import (
	"fmt"
	"strings"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

// Source is the exact input captured for this analysis, not a path to reread.
type Source struct {
	Content []byte
	Origin  string
}

// Failure records a source that could not be analyzed.
type Failure struct {
	Source  string
	Message string
	Origin  string
}

// Scope describes the selected analysis inputs and filters.
type Scope struct {
	Targets  []string
	Excludes []string
	Include  string
	Files    int
	Language string
}

// Report combines already-filtered relationships with analysis context.
type Report struct {
	Analysis relations.Analysis
	Sources  map[string]Source
	Failures []Failure
	Scope    Scope
}

// Format explains only the supplied analysis, without reading source files.
func Format(report Report) string {
	var out strings.Builder
	fmt.Fprintln(&out, "Relationship explanations:")
	fmt.Fprintln(&out, "\nAnalysis scope:")
	fmt.Fprintf(&out, "  Targets: %s\n", listed(report.Scope.Targets, "(none supplied)"))
	fmt.Fprintf(&out, "  Selected files: %d\n", report.Scope.Files)
	fmt.Fprintf(&out, "  Language: %s\n", specified(report.Scope.Language, "(not specified)"))
	fmt.Fprintf(&out, "  Include: %s\n", specified(report.Scope.Include, "(no filter)"))
	fmt.Fprintf(&out, "  Exclude: %s\n", listed(report.Scope.Excludes, "(none)"))
	fmt.Fprintln(&out, "  Only selected files are parsed; sources outside file selection and external dependency sources are not loaded.")
	fmt.Fprintln(&out, "  Package/include and display filters can hide declarations that were parsed.")
	fmt.Fprintln(&out, "  No displayed edge does not imply no dependencies; this report is limited to the selected analysis and display filters.")

	if len(report.Failures) > 0 {
		fmt.Fprintln(&out, "\nAnalysis failures:")
		for _, failure := range report.Failures {
			fmt.Fprintf(&out, "  %s (%s): %s\n", specified(failure.Source, "(source unknown)"), origin(failure.Origin), failure.Message)
		}
		fmt.Fprintln(&out, "  Analysis is incomplete. Failed inputs cannot establish the absence of dependencies.")
	}

	renderer := sourceRenderer{sources: report.Sources, lines: make(map[string][]string)}
	for _, relation := range report.Analysis.Relations {
		fmt.Fprintf(&out, "\n%s -> %s [%s", relation.From, relation.To, relation.Kind)
		if relation.Label != "" {
			fmt.Fprintf(&out, ", label=%q", relation.Label)
		}
		if relation.Multiplicity != "" {
			fmt.Fprintf(&out, ", multiplicity=%s", relation.Multiplicity)
		}
		fmt.Fprintln(&out, "]")
		if len(relation.Evidence) == 0 {
			fmt.Fprintln(&out, "  Declaration evidence was not supplied.")
		}
		for _, evidence := range relation.Evidence {
			fmt.Fprintf(&out, "  %s:%d: %s\n", specified(evidence.Source, "(source unknown)"), evidence.Line, reason(relation, evidence))
			renderer.snippet(&out, evidence)
		}
	}

	for _, category := range []struct {
		status  relations.ReferenceStatus
		heading string
		note    string
	}{
		{relations.StatusUnresolved, "Unresolved references", "These references could not be resolved from analyzed declarations. Their cause is not established; this does not prove a type or dependency is absent."},
		{relations.StatusOutsideDiagram, "Known references outside diagram", "These references resolved to known types that are not displayed in this diagram."},
	} {
		shown := false
		for _, diagnostic := range report.Analysis.Diagnostics {
			if diagnostic.Status != category.status {
				continue
			}
			if !shown {
				fmt.Fprintf(&out, "\n%s:\n  %s\n", category.heading, category.note)
				shown = true
			}
			fmt.Fprintf(&out, "  %s [%s, %s]: reference=%q", diagnostic.From, diagnostic.Kind, diagnostic.Status, diagnostic.Reference)
			if diagnostic.Target != "" {
				fmt.Fprintf(&out, ", target=%s", diagnostic.Target)
			}
			if diagnostic.Evidence.Member != "" {
				fmt.Fprintf(&out, ", member=%q", diagnostic.Evidence.Member)
			}
			fmt.Fprintf(&out, " at %s:%d\n", specified(diagnostic.Evidence.Source, "(source unknown)"), diagnostic.Evidence.Line)
			renderer.snippet(&out, diagnostic.Evidence)
		}
	}
	return out.String()
}

func reason(relation relations.Relation, evidence relations.Evidence) string {
	member := specified(evidence.Member, "(member not supplied)")
	typeName := strings.Join(strings.Fields(specified(evidence.Type, relation.To)), " ")
	switch relation.Kind {
	case relations.Inheritance:
		if evidence.Language == "go" {
			if evidence.OwnerKind == model.Interface {
				return fmt.Sprintf("Go interface embedding: %s embeds %s, including its interface requirements; this is not Java class inheritance.", relation.From, typeName)
			}
			return fmt.Sprintf("Go embedding: %s embeds %s; embedding can promote members, but is not Java class inheritance.", relation.From, typeName)
		}
		if evidence.Language == "java" {
			if evidence.OwnerKind == model.Interface {
				return fmt.Sprintf("Java interface extension: %s extends interface %s.", relation.From, typeName)
			}
			return fmt.Sprintf("Java class inheritance: %s extends superclass %s.", relation.From, typeName)
		}
		return fmt.Sprintf("%s declares %s as a parent type.", relation.From, typeName)
	case relations.Implementation:
		if evidence.Inferred {
			if evidence.Member != "" {
				return fmt.Sprintf("Inferred Go implementation of %s: matched method signature %s. This is syntactic method-set matching, not compiler-verified.", relation.To, evidence.Member)
			}
			return fmt.Sprintf("Inferred Go implementation of %s: candidate type %s is anchored here. This is syntactic method-set matching, not compiler-verified; this anchor is not a matched method.", relation.To, relation.From)
		}
		return fmt.Sprintf("Explicit implementation: %s declares that it implements %s.", relation.From, typeName)
	case relations.Field:
		return fmt.Sprintf("Field %s holds a value of declared type %s, referencing %s.", member, typeName, relation.To)
	case relations.Parameter:
		return fmt.Sprintf("Parameter %s receives a value of declared type %s, referencing %s.", member, typeName, relation.To)
	case relations.Return:
		return fmt.Sprintf("Method %s returns a value of declared type %s, referencing %s.", member, typeName, relation.To)
	default:
		return fmt.Sprintf("Declaration references %s (declared type %s).", relation.To, typeName)
	}
}

type sourceRenderer struct {
	sources map[string]Source
	lines   map[string][]string
}

func (r *sourceRenderer) snippet(out *strings.Builder, evidence relations.Evidence) {
	source, ok := r.sources[evidence.Source]
	if !ok {
		fmt.Fprintln(out, "    Source snippet unavailable: no captured source snapshot.")
		return
	}
	lines, cached := r.lines[evidence.Source]
	if !cached {
		text := string(source.Content)
		if text != "" {
			// A terminal newline ends the last physical line; it does not add one.
			lines = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
		}
		r.lines[evidence.Source] = lines
	}
	if evidence.Line < 1 || evidence.Line > len(lines) {
		fmt.Fprintf(out, "    Source snippet unavailable (%s): line %d is outside the captured snapshot (%d lines).\n", origin(source.Origin), evidence.Line, len(lines))
		return
	}
	fmt.Fprintf(out, "    Source snippet (%s; up to 3 lines from the declaration, not necessarily the whole declaration):\n", origin(source.Origin))
	for index := evidence.Line - 1; index < len(lines) && index-evidence.Line+1 < 3; index++ {
		fmt.Fprintf(out, "      %d | %s\n", index+1, strings.TrimSuffix(lines[index], "\r"))
	}
}

func origin(value string) string {
	return specified(value, "origin unknown")
}

func specified(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func listed(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return strings.Join(values, ", ")
}
