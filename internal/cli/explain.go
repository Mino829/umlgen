package cli

import (
	"fmt"
	"io"

	"github.com/Mino829/umlgen/internal/explain"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/relations"
)

func writeExplanation(w io.Writer, report explain.Report, analysis relations.Analysis, opts plantuml.Options) {
	if opts.ShowRelations {
		var selected []relations.Relation
		for _, relation := range analysis.Relations {
			if opts.IncludesRelation(relation.Kind) {
				selected = append(selected, relation)
			}
		}
		analysis.Relations = selected
		var diagnostics []relations.ReferenceDiagnostic
		for _, diagnostic := range analysis.Diagnostics {
			if opts.IncludesRelation(diagnostic.Kind) {
				diagnostics = append(diagnostics, diagnostic)
			}
		}
		analysis.Diagnostics = diagnostics
		report.Analysis = analysis
	}
	fmt.Fprint(w, explain.Format(report))
}
