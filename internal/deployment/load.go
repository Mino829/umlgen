package deployment

import (
	"fmt"
	"slices"
	"strings"
)

// LoadManifest reads only the named input. It neither merges other files nor
// expands environment variables, builds images or queries a cluster.
func LoadManifest(path, format string) (Graph, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "auto"
	}
	if format != "auto" && format != "compose" && format != "kubernetes" {
		return Graph{}, fmt.Errorf("%w: manifest format must be auto, compose or kubernetes", ErrInput)
	}
	docs, source, err := decode(path, format)
	if err != nil {
		return Graph{}, err
	}
	firstIndex := -1
	for i, doc := range docs {
		if doc != nil {
			firstIndex = i
			break
		}
	}
	if firstIndex < 0 {
		return Graph{}, fmt.Errorf("%w: empty manifest", ErrInput)
	}
	first, err := mapping(docs[firstIndex])
	if err != nil {
		return Graph{}, err
	}
	if format == "auto" {
		switch {
		case first["services"] != nil:
			format = "compose"
		case first["apiVersion"] != nil && first["kind"] != nil:
			format = "kubernetes"
		default:
			return Graph{}, invalid(docs[firstIndex], "unrecognized manifest format")
		}
	}
	source.Format = format
	var graph Graph
	if format == "compose" {
		for i, doc := range docs {
			if i != firstIndex && doc != nil {
				return Graph{}, invalid(doc, "Compose input must contain one document; merging is unsupported")
			}
		}
		source.Document = firstIndex + 1
		graph, err = parseCompose(docs[firstIndex], source)
	} else {
		graph, err = parseKubernetes(docs, source)
	}
	if err != nil {
		return Graph{}, err
	}
	if len(graph.Nodes) == 0 {
		return Graph{}, fmt.Errorf("%w: no supported topology resources; previous diagram preserved", ErrInput)
	}
	if err = validateGraph(graph); err != nil {
		return Graph{}, err
	}
	normalize(&graph)
	return graph, nil
}

func validateGraph(g Graph) error {
	ids := make(map[ID]bool, len(g.Nodes))
	for _, node := range g.Nodes {
		if node.ID.Kind == "" || node.ID.Name == "" || ids[node.ID] {
			return fmt.Errorf("%w: missing or duplicate topology identity", ErrInput)
		}
		ids[node.ID] = true
	}
	for _, edge := range g.Edges {
		if !ids[edge.From] || !ids[edge.To] {
			return fmt.Errorf("%w: undeclared topology endpoint", ErrInput)
		}
	}
	return nil
}
func compareID(a, b ID) int {
	if n := strings.Compare(a.Kind, b.Kind); n != 0 {
		return n
	}
	if n := strings.Compare(a.Namespace, b.Namespace); n != 0 {
		return n
	}
	return strings.Compare(a.Name, b.Name)
}
func normalize(g *Graph) {
	slices.SortFunc(g.Nodes, func(a, b Node) int { return compareID(a.ID, b.ID) })
	slices.SortStableFunc(g.Edges, func(a, b Edge) int {
		if n := compareID(a.From, b.From); n != 0 {
			return n
		}
		if n := compareID(a.To, b.To); n != 0 {
			return n
		}
		if n := strings.Compare(a.Kind, b.Kind); n != 0 {
			return n
		}
		if n := strings.Compare(a.Detail, b.Detail); n != 0 {
			return n
		}
		if a.Origin.Document != b.Origin.Document {
			return a.Origin.Document - b.Origin.Document
		}
		return a.Origin.Line - b.Origin.Line
	})
	slices.SortStableFunc(g.Diagnostics, func(a, b Diagnostic) int {
		if n := strings.Compare(a.Origin.File, b.Origin.File); n != 0 {
			return n
		}
		if a.Origin.Document != b.Origin.Document {
			return a.Origin.Document - b.Origin.Document
		}
		if a.Origin.Line != b.Origin.Line {
			return a.Origin.Line - b.Origin.Line
		}
		return strings.Compare(a.Code, b.Code)
	})
}
