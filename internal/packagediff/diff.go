// Package packagediff compares complete selected package graphs before display
// projection. Source locations remain available as evidence, not change identity.
package packagediff

import (
	"cmp"
	"slices"

	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/relations"
)

type Kind string

const (
	Unchanged Kind = "unchanged"
	Added     Kind = "added"
	Removed   Kind = "removed"
	Modified  Kind = "modified"
	Uncertain Kind = "uncertain"
)

type Snapshot struct {
	Revision string
	Complete bool
	View     packagegraph.View
}

type PackageChange struct {
	ID            packagegraph.PackageID
	Before, After *packagegraph.Package
	Kind          Kind
}

// Evidence retains the original source metadata. Exactly one field is set; a
// Relation contains one declaration evidence record, or none for bare relations.
type Evidence struct {
	Relation *relations.Relation
	Import   *packagegraph.ImportEvidence
}

type DependencyChange struct {
	From, To                       packagegraph.PackageID
	Before, After                  *packagegraph.Dependency
	Kind                           Kind
	AddedEvidence, RemovedEvidence []Evidence
}

type CycleChange struct {
	Before, After *packagegraph.Cycle
	Kind          Kind
}

type Result struct {
	Before, After Snapshot
	Packages      []PackageChange
	Dependencies  []DependencyChange
	Cycles        []CycleChange
}

// Compare never mutates either snapshot. Before/After pointers borrow immutable
// snapshot metadata. Identity is exact, including language and directory context;
// neither renames nor equality of display names merge distinct packages.
func Compare(before, after Snapshot) Result {
	result := Result{Before: before, After: after}
	beforeKnowledge, afterKnowledge := knowledgeOf(before), knowledgeOf(after)
	packages := make(map[packagegraph.PackageID]PackageChange)
	for i := range before.View.Graph.Packages {
		pkg := &before.View.Graph.Packages[i]
		packages[pkg.ID] = PackageChange{ID: pkg.ID, Before: pkg}
	}
	for i := range after.View.Graph.Packages {
		pkg := &after.View.Graph.Packages[i]
		change := packages[pkg.ID]
		change.ID, change.After = pkg.ID, pkg
		packages[pkg.ID] = change
	}
	result.Packages = make([]PackageChange, 0, len(packages))
	for _, change := range packages {
		switch {
		case change.Before == nil:
			change.Kind = existenceKind(Added, beforeKnowledge.complete)
		case change.After == nil:
			change.Kind = existenceKind(Removed, afterKnowledge.complete)
		case samePackage(*change.Before, *change.After):
			change.Kind = Unchanged
		default:
			change.Kind = existenceKind(Modified, beforeKnowledge.complete && afterKnowledge.complete)
		}
		result.Packages = append(result.Packages, change)
	}
	slices.SortFunc(result.Packages, func(a, b PackageChange) int { return compareID(a.ID, b.ID) })

	type endpoints struct{ from, to packagegraph.PackageID }
	dependencies := make(map[endpoints]DependencyChange)
	for i := range before.View.Graph.Dependencies {
		dep := &before.View.Graph.Dependencies[i]
		dependencies[endpoints{dep.From, dep.To}] = DependencyChange{From: dep.From, To: dep.To, Before: dep}
	}
	for i := range after.View.Graph.Dependencies {
		dep := &after.View.Graph.Dependencies[i]
		key := endpoints{dep.From, dep.To}
		change := dependencies[key]
		change.From, change.To, change.After = dep.From, dep.To, dep
		dependencies[key] = change
	}
	result.Dependencies = make([]DependencyChange, 0, len(dependencies))
	for _, change := range dependencies {
		change.AddedEvidence, change.RemovedEvidence = evidenceDelta(change.Before, change.After)
		switch {
		case change.Before == nil:
			change.Kind = existenceKind(Added, beforeKnowledge.absenceKnown(change.From))
		case change.After == nil:
			change.Kind = existenceKind(Removed, afterKnowledge.absenceKnown(change.From))
		case len(change.AddedEvidence) == 0 && len(change.RemovedEvidence) == 0:
			change.Kind = Unchanged
		default:
			change.Kind = existenceKind(Modified, beforeKnowledge.absenceKnown(change.From) && afterKnowledge.absenceKnown(change.From))
		}
		result.Dependencies = append(result.Dependencies, change)
	}
	slices.SortFunc(result.Dependencies, func(a, b DependencyChange) int {
		return cmp.Or(compareID(a.From, b.From), compareID(a.To, b.To))
	})
	result.Cycles = compareCycles(before, after, beforeKnowledge, afterKnowledge)
	return result
}

func existenceKind(kind Kind, complete bool) Kind {
	if !complete {
		return Uncertain
	}
	return kind
}

func compareID(a, b packagegraph.PackageID) int {
	return cmp.Or(cmp.Compare(a.Language, b.Language), cmp.Compare(a.Context, b.Context), cmp.Compare(a.Name, b.Name))
}

func samePackage(a, b packagegraph.Package) bool {
	if a.Kind != b.Kind || len(a.Files) != len(b.Files) {
		return false
	}
	ordered := true
	for i := range a.Files {
		if a.Files[i].Path != b.Files[i].Path {
			ordered = false
			break
		}
	}
	if ordered {
		return true
	}
	// File membership is a set of original paths, independent of content hashes
	// and ordering. Graph construction supplies each source file exactly once.
	files := make(map[string]struct{}, len(a.Files))
	for _, file := range a.Files {
		files[file.Path] = struct{}{}
	}
	for _, file := range b.Files {
		if _, exists := files[file.Path]; !exists {
			return false
		}
	}
	return true
}

type cycleRecord struct {
	cycle   *packagegraph.Cycle
	members []packagegraph.PackageID
}

func orderedCycles(cycles []packagegraph.Cycle) []cycleRecord {
	result := make([]cycleRecord, 0, len(cycles))
	for i := range cycles {
		members := slices.Clone(cycles[i].Packages)
		slices.SortFunc(members, compareID)
		members = slices.Compact(members)
		result = append(result, cycleRecord{cycle: &cycles[i], members: members})
	}
	slices.SortFunc(result, compareCycleRecord)
	return result
}

func compareCycleRecord(a, b cycleRecord) int {
	return cmp.Or(cmp.Compare(a.cycle.Kind, b.cycle.Kind), slices.CompareFunc(a.members, b.members, compareID))
}

func compareCycles(before, after Snapshot, beforeKnowledge, afterKnowledge snapshotKnowledge) []CycleChange {
	left, right := orderedCycles(before.View.Cycles), orderedCycles(after.View.Cycles)
	var changes []CycleChange
	for i, j := 0, 0; i < len(left) || j < len(right); {
		order := 0
		switch {
		case i == len(left):
			order = 1
		case j == len(right):
			order = -1
		default:
			order = compareCycleRecord(left[i], right[j])
		}
		switch {
		case order < 0:
			cycle := left[i].cycle
			changes = append(changes, CycleChange{Before: cycle, Kind: existenceKind(Removed, beforeKnowledge.cycleKnown(cycle) && afterKnowledge.cycleKnown(cycle))})
			i++
		case order > 0:
			cycle := right[j].cycle
			changes = append(changes, CycleChange{After: cycle, Kind: existenceKind(Added, beforeKnowledge.cycleKnown(cycle) && afterKnowledge.cycleKnown(cycle))})
			j++
		default:
			changes = append(changes, CycleChange{Before: left[i].cycle, After: right[j].cycle, Kind: Unchanged})
			i++
			j++
		}
	}
	return changes
}

// Project selects display changes using visible membership from either side.
// Classification, snapshots, full-scope cycle changes and analysis counts are
// retained verbatim; hidden evidence can never become a false edge removal.
func Project(result Result, showExternal, cyclesOnly bool) Result {
	if showExternal && !cyclesOnly {
		return result
	}
	visible := make(map[packagegraph.PackageID]struct{})
	for _, snapshot := range []Snapshot{result.Before, result.After} {
		var members map[packagegraph.PackageID]struct{}
		if cyclesOnly {
			members = make(map[packagegraph.PackageID]struct{})
			for _, cycle := range snapshot.View.Cycles {
				for _, id := range cycle.Packages {
					members[id] = struct{}{}
				}
			}
		}
		for _, pkg := range snapshot.View.Graph.Packages {
			if !showExternal && pkg.Kind == packagegraph.ExternalPackage {
				continue
			}
			if cyclesOnly {
				if _, member := members[pkg.ID]; !member {
					continue
				}
			}
			visible[pkg.ID] = struct{}{}
		}
	}
	projected := result
	projected.Packages = make([]PackageChange, 0, len(result.Packages))
	for _, change := range result.Packages {
		if _, keep := visible[change.ID]; keep {
			projected.Packages = append(projected.Packages, change)
		}
	}
	projected.Dependencies = make([]DependencyChange, 0, len(result.Dependencies))
	for _, change := range result.Dependencies {
		_, from := visible[change.From]
		_, to := visible[change.To]
		if from && to {
			projected.Dependencies = append(projected.Dependencies, change)
		}
	}
	return projected
}
