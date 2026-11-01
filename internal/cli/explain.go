package cli

import (
	"fmt"
	"io"

	"github.com/Mino829/umlgen/internal/explain"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/relations"
)

func writeExplanation(w io.Writer, report explain.Report, types, known []model.Type, opts plantuml.Options) {
	if opts.ShowRelations && len(types) > 0 {
		analysis := relations.Analyze(types, known)
		selected := analysis.Relations[:0]
		for _, relation := range analysis.Relations {
			if opts.IncludesRelation(relation.Kind) {
				selected = append(selected, relation)
			}
		}
		analysis.Relations = selected
		diagnostics := analysis.Diagnostics[:0]
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
