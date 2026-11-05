package packagediff

import (
	"cmp"
	"slices"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/relations"
)

// All fields here are semantic. Location, content hash and input order are
// deliberately absent; source path and duplicate counts remain significant.
type proofKey struct {
	category                            uint8
	from, to                            string
	kind                                relations.Kind
	label, multiplicity                 string
	source, member, reference, language string
	ownerKind                           model.TypeKind
	name, alias                         string
	flags                               uint8
}

const (
	importProof uint8 = iota
	bareRelationProof
	declarationProof
)

type proof struct {
	key         proofKey
	relation    *relations.Relation
	declaration *relations.Evidence
	imported    *packagegraph.ImportEvidence
}

func dependencyProofs(dep *packagegraph.Dependency) []proof {
	if dep == nil {
		return nil
	}
	count := len(dep.Imports)
	for _, relation := range dep.TypeRelations {
		count += max(1, len(relation.Evidence))
	}
	proofs := make([]proof, 0, count)
	for i := range dep.Imports {
		imported := &dep.Imports[i]
		key := proofKey{category: importProof, source: imported.File.Path, name: imported.Import.Name, alias: imported.Import.Alias}
		if imported.Import.Static {
			key.flags |= 1
		}
		if imported.Import.Wildcard {
			key.flags |= 2
		}
		proofs = append(proofs, proof{key: key, imported: imported})
	}
	for i := range dep.TypeRelations {
		relation := &dep.TypeRelations[i]
		key := proofKey{category: bareRelationProof, from: relation.From, to: relation.To, kind: relation.Kind, label: relation.Label, multiplicity: relation.Multiplicity}
		if len(relation.Evidence) == 0 {
			proofs = append(proofs, proof{key: key, relation: relation})
			continue
		}
		key.category = declarationProof
		for j := range relation.Evidence {
			evidence := &relation.Evidence[j]
			key.source, key.member, key.reference = evidence.Source, evidence.Member, evidence.Type
			key.language, key.ownerKind = evidence.Language, evidence.OwnerKind
			key.flags = 0
			if evidence.Inferred {
				key.flags = 1
			}
			proofs = append(proofs, proof{key: key, relation: relation, declaration: evidence})
		}
	}
	slices.SortFunc(proofs, compareProof)
	return proofs
}

func compareProofKey(a, b proofKey) int {
	return cmp.Or(
		cmp.Compare(a.category, b.category),
		cmp.Compare(a.source, b.source),
		cmp.Compare(a.from, b.from),
		cmp.Compare(a.to, b.to),
		cmp.Compare(a.kind, b.kind),
		cmp.Compare(a.label, b.label),
		cmp.Compare(a.multiplicity, b.multiplicity),
		cmp.Compare(a.member, b.member),
		cmp.Compare(a.reference, b.reference),
		cmp.Compare(a.language, b.language),
		cmp.Compare(a.ownerKind, b.ownerKind),
		cmp.Compare(a.name, b.name),
		cmp.Compare(a.alias, b.alias),
		cmp.Compare(a.flags, b.flags),
	)
}

func compareProof(a, b proof) int {
	if order := compareProofKey(a.key, b.key); order != 0 {
		return order
	}
	// Equivalent duplicate declarations pair by physical position so residual
	// evidence is deterministic without making positions part of identity.
	if a.imported != nil {
		return cmp.Or(compareSpan(a.imported.Import.Span, b.imported.Import.Span), cmp.Compare(a.imported.File.ContentHash, b.imported.File.ContentHash))
	}
	if a.declaration != nil {
		return cmp.Compare(a.declaration.Line, b.declaration.Line)
	}
	return 0
}

func compareSpan(a, b model.SourceSpan) int {
	return cmp.Or(
		cmp.Compare(a.Start.Offset, b.Start.Offset),
		cmp.Compare(a.Start.Line, b.Start.Line),
		cmp.Compare(a.Start.Column, b.Start.Column),
		cmp.Compare(a.End.Offset, b.End.Offset),
		cmp.Compare(a.End.Line, b.End.Line),
		cmp.Compare(a.End.Column, b.End.Column),
	)
}

func (p proof) evidence() Evidence {
	if p.imported != nil {
		imported := *p.imported
		return Evidence{Import: &imported}
	}
	relation := *p.relation
	relation.Evidence = nil
	if p.declaration != nil {
		relation.Evidence = []relations.Evidence{*p.declaration}
	}
	return Evidence{Relation: &relation}
}

func evidenceDelta(before, after *packagegraph.Dependency) (added, removed []Evidence) {
	left, right := dependencyProofs(before), dependencyProofs(after)
	for i, j := 0, 0; i < len(left) || j < len(right); {
		order := 0
		switch {
		case i == len(left):
			order = 1
		case j == len(right):
			order = -1
		default:
			order = compareProofKey(left[i].key, right[j].key)
		}
		switch {
		case order < 0:
			removed = append(removed, left[i].evidence())
			i++
		case order > 0:
			added = append(added, right[j].evidence())
			j++
		default:
			i++
			j++
		}
	}
	return added, removed
}
