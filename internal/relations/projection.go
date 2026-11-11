package relations

import (
	"sort"

	"github.com/Mino829/umlgen/internal/model"
)

// Select narrows an analysis to a subset of its declarations without resolving
// names again. Hidden known targets remain outside-diagram, never unresolved or
// rebound to a different import. The original analysis and its slices are read-only.
func (a Analysis) Select(types []model.Type) Analysis {
	visible := make(map[string]bool, len(types))
	for _, t := range types {
		visible[t.QualifiedName()] = true
	}
	result := Analysis{Project: model.Project{Types: types}}
	for _, relation := range a.Relations {
		if visible[relation.From] && visible[relation.To] {
			result.Relations = append(result.Relations, relation)
		}
	}
	var seen map[ReferenceDiagnostic]bool
	add := func(diagnostic ReferenceDiagnostic) {
		if seen == nil {
			seen = make(map[ReferenceDiagnostic]bool)
		}
		if !seen[diagnostic] {
			result.Diagnostics = append(result.Diagnostics, diagnostic)
			seen[diagnostic] = true
		}
	}
	for _, diagnostic := range a.Diagnostics {
		if visible[diagnostic.From] {
			add(diagnostic)
		}
	}
	for _, reference := range a.references {
		if !visible[reference.From] {
			continue
		}
		if visible[reference.Target] {
			result.references = append(result.references, reference)
		} else {
			add(reference)
		}
	}
	sortDiagnostics(result.Diagnostics)
	return result
}

func sortDiagnostics(diagnostics []ReferenceDiagnostic) {
	if len(diagnostics) < 2 {
		return
	}
	sort.Slice(diagnostics, func(a, b int) bool {
		left, right := diagnostics[a], diagnostics[b]
		if left.From != right.From {
			return left.From < right.From
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Evidence.Source != right.Evidence.Source {
			return left.Evidence.Source < right.Evidence.Source
		}
		if left.Evidence.Line != right.Evidence.Line {
			return left.Evidence.Line < right.Evidence.Line
		}
		if left.Evidence.Member != right.Evidence.Member {
			return left.Evidence.Member < right.Evidence.Member
		}
		if left.Reference != right.Reference {
			return left.Reference < right.Reference
		}
		return left.Status < right.Status
	})
}
