package plantuml

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Mino829/umlgen/internal/component"
	"github.com/Mino829/umlgen/internal/packagegraph"
)

type ComponentOptions struct{ Title string }

func GenerateComponent(graph component.Graph, opts ComponentOptions) (string, error) {
	if err := graph.Definition.Validate(); err != nil {
		return "", err
	}
	aliases := make(map[component.ID]string, len(graph.Nodes))
	var out strings.Builder
	out.WriteString("@startuml\n")
	if opts.Title != "" {
		fmt.Fprintf(&out, "title %s\n", packageText(opts.Title))
	}
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		if _, exists := aliases[node.ID]; exists {
			return "", fmt.Errorf("duplicate component diagram endpoint")
		}
		alias := "C" + strconv.Itoa(i)
		aliases[node.ID] = alias
		switch node.ID.Kind {
		case "component":
			if node.Boundary == nil {
				return "", fmt.Errorf("component node has no explicit boundary")
			}
			fmt.Fprintf(&out, "component \"%s\\n%s\" as %s <<configured>>\n", packageText(node.Name), packageText(fmt.Sprintf("%d source packages", len(node.Packages))), alias)
			for j, name := range node.Boundary.Provides {
				contract := alias + "_P" + strconv.Itoa(j)
				fmt.Fprintf(&out, "interface \"%s\" as %s\n%s - %s : configured provides\n", packageText(name), contract, alias, contract)
			}
			for j, name := range node.Boundary.Requires {
				contract := alias + "_R" + strconv.Itoa(j)
				fmt.Fprintf(&out, "interface \"%s\" as %s\n%s ..> %s : configured requires\\nprovider binding not specified\n", packageText(name), contract, alias, contract)
			}
		case "unassigned", "external":
			name := node.Name
			if name == "" {
				name = "(default package)"
			}
			label := packageText(name) + "\\n" + packageText(node.ID.Package.Language+" / "+node.ID.Package.Context)
			fmt.Fprintf(&out, "package \"%s\" as %s <<%s>> {\n}\n", label, alias, node.ID.Kind)
		default:
			return "", fmt.Errorf("unknown component node kind %q", node.ID.Kind)
		}
	}
	for _, edge := range graph.Dependencies {
		from, okFrom := aliases[edge.From]
		to, okTo := aliases[edge.To]
		if !okFrom || !okTo || edge.From == edge.To {
			return "", fmt.Errorf("component dependency has missing or identical endpoints")
		}
		types, imports := componentEvidenceCounts(edge.Originals)
		fmt.Fprintf(&out, "%s --> %s : source package dependencies\\ntype records %d; import declarations %d\n", from, to, types, imports)
	}
	out.WriteString("\nlegend bottom\nExplicit component boundaries; source dependencies, NOT runtime communication\n")
	fmt.Fprintf(&out, "Definition: %s; SHA256 %s\nDependency source: %s; source packages: %d; package dependencies: %d\n", packageText(graph.Definition.Source), packageText(graph.Definition.Hash), packageText(string(graph.Source.Source)), len(graph.Source.Packages), len(graph.Source.Dependencies))
	out.WriteString("Ownership and provides/requires are configured; package names are not inferred responsibilities\nEqual interface names are not provider bindings or observed API calls\nInternal package dependencies retain evidence but draw no component self-arrow\nUnassigned packages stay distinct; external endpoints are not source-owned components\nType records and import declarations are different units, not runtime call counts\nBuild validity, module/classpath resolution, deployment and network traffic are unverified\n")
	if len(graph.Hidden) > 0 {
		fmt.Fprintf(&out, "External-display filter hides %d original package dependencies; evidence retained\n", len(graph.Hidden))
	}
	for _, d := range graph.Diagnostics {
		fmt.Fprintf(&out, "Diagnostic %s: %s [%s:%d]\n", packageText(d.Code), packageText(d.Message), packageText(graph.Definition.Source), d.Line)
	}
	for _, d := range graph.Source.Diagnostics {
		fmt.Fprintf(&out, "Source diagnostic %s (%s): %s [%s:%d]\n", packageText(d.Code), packageText(string(d.Status)), packageText(d.Reference), packageText(d.File.Path), d.Line)
	}
	out.WriteString("endlegend\n@enduml\n")
	return out.String(), nil
}

func componentEvidenceCounts(originals []packagegraph.Dependency) (types, imports int) {
	for i := range originals {
		types += len(originals[i].TypeRelations)
		imports += len(originals[i].Imports)
	}
	return
}
