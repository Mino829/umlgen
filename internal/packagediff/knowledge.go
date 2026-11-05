package packagediff

import "github.com/Mino829/umlgen/internal/packagegraph"

// A successfully parsed file can still contain references that did not resolve.
// Their source package cannot prove absence of a formerly known dependency.
// Keep this uncertainty local instead of invalidating unrelated packages.
type snapshotKnowledge struct {
	complete   bool
	unresolved map[packagegraph.PackageID]bool
}

func knowledgeOf(snapshot Snapshot) snapshotKnowledge {
	knowledge := snapshotKnowledge{complete: snapshot.Complete}
	for _, diagnostic := range snapshot.View.Graph.Diagnostics {
		if diagnostic.Code == "unsupported-module-declaration" {
			knowledge.complete = false
		}
		if diagnostic.Status != packagegraph.Unresolved && diagnostic.Status != packagegraph.Ambiguous {
			continue
		}
		if knowledge.unresolved == nil {
			knowledge.unresolved = make(map[packagegraph.PackageID]bool)
		}
		knowledge.unresolved[diagnostic.From] = true
	}
	return knowledge
}

func (k snapshotKnowledge) absenceKnown(from packagegraph.PackageID) bool {
	return k.complete && !k.unresolved[from]
}

func (k snapshotKnowledge) cycleKnown(cycle *packagegraph.Cycle) bool {
	if !k.complete {
		return false
	}
	for _, id := range cycle.Packages {
		if k.unresolved[id] {
			return false
		}
	}
	return true
}
