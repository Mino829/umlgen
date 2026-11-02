package packagegraph

import "github.com/Mino829/umlgen/internal/relations"

type CycleKind string

const (
	DeclaredCycle  CycleKind = "declared"
	CandidateCycle CycleKind = "candidate"
)

// Cycle is a strongly connected group in the selected analysis graph, not a
// build/compiler finding. Candidate groups require inferred implementation
// evidence; declared subgroups may also appear within a larger candidate group.
type Cycle struct {
	Kind     CycleKind
	Packages []PackageID
}

type CycleMode string

const (
	CyclesOff  CycleMode = "off"
	CyclesMark CycleMode = "mark"
	CyclesOnly CycleMode = "only"
)

type Direction string

const (
	DirectionIn   Direction = "in"
	DirectionOut  Direction = "out"
	DirectionBoth Direction = "both"
)

type ViewOptions struct {
	// Nil selects all type relation kinds; a nonnil empty slice selects none.
	// Import declarations are independent and survive type relation filtering.
	Relations    []relations.Kind
	Focus        *PackageID
	Depth        int
	Direction    Direction
	ShowExternal bool
	Cycles       CycleMode
}

// AnalysisScope describes the graph AFTER dependency/relation selection but
// BEFORE focus, cycle-only selection and external-node hiding. Counts must not
// be presented as the size of the entire repository or of a particular build.
type AnalysisScope struct {
	Source          DependencySource
	Relations       []relations.Kind
	PackageCount    int
	DependencyCount int
	Focus           *PackageID
	Depth           int
	Direction       Direction
	ShowExternal    bool
	Cycles          CycleMode
}

// View keeps complete selected-scope cycles and diagnostics even if their nodes
// or edges are hidden in Graph. CyclesOff skips detection, rather than asserting
// acyclicity. Only Graph.Packages/Dependencies are projected for display.
// Nested file/evidence/candidate slices may borrow immutable analysis metadata;
// callers must treat them as read-only. Analyze never mutates its input graph.
// The visible graph must never be used to reclassify the original cycle result.
type View struct {
	Graph  Graph
	Cycles []Cycle
	Scope  AnalysisScope
}
