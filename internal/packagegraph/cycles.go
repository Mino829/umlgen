package packagegraph

import "slices"

// findCycles classifies the selected analysis graph before any visibility
// projection. A declared component remains visible even when inferred evidence
// joins it to a larger candidate component.
func findCycles(graph Graph) []Cycle {
	ids := make([]PackageID, 0, len(graph.Packages))
	vertices := make(map[PackageID]int, len(graph.Packages))
	for _, pkg := range graph.Packages {
		if _, exists := vertices[pkg.ID]; exists {
			continue
		}
		vertices[pkg.ID] = len(ids)
		ids = append(ids, pkg.ID)
	}
	hasInferredOnly := false
	for _, dependency := range graph.Dependencies {
		from, fromExists := vertices[dependency.From]
		to, toExists := vertices[dependency.To]
		if fromExists && toExists && from != to && !dependencyHasDeclaredEvidence(dependency) {
			hasInferredOnly = true
			break
		}
	}
	// Without an inferred-only edge the two graphs are identical: share the
	// adjacency and run only the declared traversal.
	allEdges := make([][]int, len(ids))
	declaredEdges := allEdges
	if hasInferredOnly {
		declaredEdges = make([][]int, len(ids))
	}
	for _, dependency := range graph.Dependencies {
		from, fromExists := vertices[dependency.From]
		to, toExists := vertices[dependency.To]
		if !fromExists || !toExists || from == to {
			continue
		}
		allEdges[from] = append(allEdges[from], to)
		if hasInferredOnly && dependencyHasDeclaredEvidence(dependency) {
			declaredEdges[from] = append(declaredEdges[from], to)
		}
	}

	declared := stronglyConnectedPackages(declaredEdges)
	var cycles []Cycle
	declaredCycles := make([]int, len(declared.sizes))
	for vertex, id := range ids {
		component := declared.labels[vertex]
		if declared.sizes[component] < 2 {
			continue
		}
		if declaredCycles[component] == 0 {
			cycles = append(cycles, Cycle{Kind: DeclaredCycle, Packages: make([]PackageID, 0, declared.sizes[component])})
			declaredCycles[component] = len(cycles)
		}
		cycle := &cycles[declaredCycles[component]-1]
		cycle.Packages = append(cycle.Packages, id)
	}
	if hasInferredOnly {
		all := stronglyConnectedPackages(allEdges)
		candidateCycles := make([]int, len(all.sizes))
		for vertex, id := range ids {
			component := all.labels[vertex]
			// Every declared SCC is contained in an all-evidence SCC. Equal sizes
			// therefore establish identical membership without a set comparison.
			if all.sizes[component] < 2 || all.sizes[component] == declared.sizes[declared.labels[vertex]] {
				continue
			}
			if candidateCycles[component] == 0 {
				cycles = append(cycles, Cycle{Kind: CandidateCycle, Packages: make([]PackageID, 0, all.sizes[component])})
				candidateCycles[component] = len(cycles)
			}
			cycle := &cycles[candidateCycles[component]-1]
			cycle.Packages = append(cycle.Packages, id)
		}
	}
	for i := range cycles {
		slices.SortFunc(cycles[i].Packages, comparePackageID)
	}
	slices.SortFunc(cycles, func(a, b Cycle) int {
		if a.Kind != b.Kind {
			if a.Kind == DeclaredCycle {
				return -1
			}
			return 1
		}
		return slices.CompareFunc(a.Packages, b.Packages, comparePackageID)
	})
	return cycles
}

// An ordinary relation record needs no explicit evidence. Once evidence is
// present, only an exclusively inferred record fails to establish a declared
// dependency; independent import proof always establishes one.
func dependencyHasDeclaredEvidence(dependency Dependency) bool {
	if len(dependency.Imports) != 0 {
		return true
	}
	for _, relation := range dependency.TypeRelations {
		if len(relation.Evidence) == 0 {
			return true
		}
		for _, evidence := range relation.Evidence {
			if !evidence.Inferred {
				return true
			}
		}
	}
	return false
}

type packageComponents struct {
	labels []int
	sizes  []int
}

// stronglyConnectedPackages is iterative Tarjan traversal: the DFS frames and
// unresolved component stack are separate, so chains cannot exhaust the Go
// call stack. Each vertex and adjacency entry is processed in O(V+E) time.
func stronglyConnectedPackages(edges [][]int) packageComponents {
	type frame struct {
		vertex int
		next   int
	}
	indices := make([]int, len(edges))
	low := make([]int, len(edges))
	components := packageComponents{
		labels: make([]int, len(edges)),
		sizes:  make([]int, 0, len(edges)),
	}
	active := make([]int, 0, len(edges))
	frames := make([]frame, 0, len(edges))
	nextIndex := 0
	for root := range edges {
		if indices[root] != 0 {
			continue
		}
		nextIndex++
		indices[root], low[root] = nextIndex, nextIndex
		active = append(active, root)
		frames = append(frames, frame{vertex: root})
		for len(frames) != 0 {
			current := &frames[len(frames)-1]
			vertex := current.vertex
			if current.next < len(edges[vertex]) {
				target := edges[vertex][current.next]
				current.next++
				if indices[target] == 0 {
					nextIndex++
					indices[target], low[target] = nextIndex, nextIndex
					active = append(active, target)
					frames = append(frames, frame{vertex: target})
				} else if components.labels[target] == 0 {
					low[vertex] = min(low[vertex], indices[target])
				}
				continue
			}
			frames = frames[:len(frames)-1]
			if low[vertex] == indices[vertex] {
				// During traversal, zero means the vertex is still active.
				label := len(components.sizes) + 1
				size := 0
				for {
					member := active[len(active)-1]
					active = active[:len(active)-1]
					components.labels[member] = label
					size++
					if member == vertex {
						break
					}
				}
				components.sizes = append(components.sizes, size)
			}
			if len(frames) != 0 {
				parent := frames[len(frames)-1].vertex
				low[parent] = min(low[parent], low[vertex])
			}
		}
	}
	for vertex := range components.labels {
		components.labels[vertex]--
	}
	return components
}
