package deployment

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/Mino829/umlgen/internal/component"
	"gopkg.in/yaml.v3"
)

// BindComponents binds exact configured identities, not source names, labels or
// similar image names. No source-to-image build proof is inferred.
func BindComponents(graph Graph, boundaryPath, mappingPath string) (Graph, error) {
	graph.Diagnostics = slices.Clone(graph.Diagnostics)
	if boundaryPath == "" {
		if mappingPath != "" {
			return Graph{}, fmt.Errorf("%w: mapping requires explicit component boundaries", ErrInput)
		}
		diagnostic(&graph, "component-mapping-unspecified", "Component/source to artifact mapping is unspecified", Origin{}, false)
		return graph, nil
	}
	graph.Nodes = slices.Clone(graph.Nodes)
	graph.Edges = slices.Clone(graph.Edges)
	graph.Sources = slices.Clone(graph.Sources)
	definition, err := component.LoadDefinition(boundaryPath)
	if err != nil {
		var readError *os.PathError
		if errors.As(err, &readError) {
			return Graph{}, fmt.Errorf("read deployment boundary: %w", readError)
		}
		return Graph{}, fmt.Errorf("%w: component boundary definition is invalid", ErrInput)
	}
	components := make(map[string]ID, len(definition.Components))
	bound := make(map[string]bool)
	boundary := Origin{File: definition.Source, Hash: definition.Hash, Format: "component-boundary"}
	graph.Sources = append(graph.Sources, boundary)
	for _, b := range definition.Components {
		id := ID{Kind: "component", Name: b.Name}
		components[b.Name] = id
		origin := boundary
		origin.Line = b.Line
		graph.Nodes = append(graph.Nodes, Node{ID: id, Name: b.Name, Origin: origin})
	}
	if mappingPath != "" {
		docs, source, err := decode(mappingPath, "component-mapping")
		if err != nil {
			return Graph{}, err
		}
		var doc *yaml.Node
		for i, n := range docs {
			if n == nil {
				continue
			}
			if doc != nil {
				return Graph{}, invalid(n, "mapping requires one document")
			}
			doc = n
			source.Document = i + 1
		}
		if doc == nil {
			return Graph{}, fmt.Errorf("%w: empty component mapping", ErrInput)
		}
		root, err := strictMapping(doc, "version", "bindings")
		if err != nil {
			return Graph{}, err
		}
		version, err := integer(root["version"])
		if err != nil || version != 1 {
			return Graph{}, invalid(doc, "mapping requires version 1")
		}
		bindings := root["bindings"]
		if bindings == nil || bindings.Kind != yaml.SequenceNode {
			return Graph{}, invalid(bindings, "bindings must be a sequence")
		}
		graph.Sources = append(graph.Sources, source)
		available := make(map[ID]bool, len(graph.Nodes))
		for _, n := range graph.Nodes {
			available[n.ID] = true
		}
		type pair struct {
			component ID
			target    ID
		}
		seen := make(map[pair]bool)
		for _, entry := range bindings.Content {
			row, err := strictMapping(entry, "component", "target")
			if err != nil {
				return Graph{}, err
			}
			name, err := requiredText(row["component"])
			if err != nil {
				return Graph{}, err
			}
			owner, ok := components[name]
			if !ok {
				return Graph{}, invalid(entry, "mapping refers to an undeclared component")
			}
			fields, err := strictMapping(row["target"], "kind", "namespace", "name")
			if err != nil {
				return Graph{}, err
			}
			kind, err := requiredText(fields["kind"])
			if err != nil {
				return Graph{}, err
			}
			switch kind {
			case "compose-service", "Deployment", "StatefulSet", "DaemonSet", "Job", "Pod", "Service":
			default:
				return Graph{}, invalid(entry, "mapping target kind is unsupported")
			}
			targetName, err := requiredText(fields["name"])
			if err != nil {
				return Graph{}, err
			}
			namespace := ""
			if fields["namespace"] != nil {
				namespace, err = text(fields["namespace"])
				if err != nil {
					return Graph{}, err
				}
			}
			target := ID{Kind: kind, Namespace: namespace, Name: targetName}
			if !available[target] {
				return Graph{}, invalid(entry, "mapping target is absent from captured manifest")
			}
			key := pair{owner, target}
			if seen[key] {
				return Graph{}, invalid(entry, "duplicate explicit component binding")
			}
			seen[key] = true
			bound[name] = true
			graph.Edges = append(graph.Edges, Edge{From: owner, To: target, Kind: "component-mapping", Detail: "configured; source/image correspondence unverified", Origin: at(source, entry)})
		}
	}
	for _, b := range definition.Components {
		if !bound[b.Name] {
			origin := boundary
			origin.Line = b.Line
			diagnostic(&graph, "unbound-component", "Configured component has no explicit manifest binding", origin, false)
		}
	}
	if err := validateGraph(graph); err != nil {
		return Graph{}, err
	}
	normalize(&graph)
	return graph, nil
}

func strictMapping(n *yaml.Node, allowed ...string) (map[string]*yaml.Node, error) {
	m, err := mapping(n)
	if err != nil {
		return nil, err
	}
	for key, node := range m {
		found := false
		for _, name := range allowed {
			if key == name {
				found = true
				break
			}
		}
		if !found {
			return nil, invalid(node, "unknown configuration key")
		}
	}
	return m, nil
}
func requiredText(n *yaml.Node) (string, error) {
	s, err := text(n)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(s) == "" {
		return "", invalid(n, "nonempty string required")
	}
	return s, nil
}
