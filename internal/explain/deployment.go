package explain

import (
	"fmt"
	"strings"

	"github.com/Mino829/umlgen/internal/deployment"
)

// FormatDeployment uses only the whitelisted captured topology. It intentionally
// never reads files again or prints raw YAML/snippets, labels or env/secret data.
func FormatDeployment(graph deployment.Graph) string {
	var out strings.Builder
	out.WriteString("Declared deployment explanation (configuration, NOT observed runtime):\n")
	out.WriteString("No manifest merge, environment expansion, build, script, Docker or cluster query was performed.\nRuntime placement, health, replicas/endpoints and actual traffic are unobserved.\nComponent bindings are explicit declarations, not source-to-image/build proof.\nRaw YAML snippets and environment/Secret/ConfigMap values are deliberately omitted.\n")
	for _, source := range graph.Sources {
		fmt.Fprintf(&out, "\nInput: %s [%s document %d]\nSHA256: %s\n", source.File, source.Format, source.Document, source.Hash)
	}
	out.WriteString("\nDeclared entities:\n")
	for _, node := range graph.Nodes {
		fmt.Fprintf(&out, "  %s: %s\n", deploymentIdentity(node.ID), node.Name)
		deploymentOrigin(&out, node.Origin)
		if node.Replicas != nil {
			fmt.Fprintf(&out, "    Desired replicas: %d; observed instances unknown\n", *node.Replicas)
		}
		for _, port := range node.Ports {
			fmt.Fprintf(&out, "    Declared %s port: %s -> %s/%s; exposure/traffic unverified\n", port.Scope, port.Published, port.Target, port.Protocol)
			deploymentOrigin(&out, port.Origin)
		}
	}
	out.WriteString("\nDeclared relationships (not runtime calls):\n")
	for _, edge := range graph.Edges {
		fmt.Fprintf(&out, "  %s -> %s\n    Kind: %s; %s\n", deploymentIdentity(edge.From), deploymentIdentity(edge.To), edge.Kind, edge.Detail)
		deploymentOrigin(&out, edge.Origin)
	}
	out.WriteString("\nUncertainty and omissions:\n")
	if graph.Incomplete {
		out.WriteString("  Incomplete: unsupported/unresolved declarations remain; missing edges do not prove absence.\n")
	}
	for _, d := range graph.Diagnostics {
		fmt.Fprintf(&out, "  %s: %s\n", d.Code, d.Message)
		deploymentOrigin(&out, d.Origin)
	}
	return out.String()
}
func deploymentIdentity(id deployment.ID) string {
	namespace := id.Namespace
	if namespace == "" {
		namespace = "(unspecified)"
	}
	return id.Kind + " " + namespace + "::" + id.Name
}
func deploymentOrigin(out *strings.Builder, origin deployment.Origin) {
	if origin.File == "" {
		return
	}
	fmt.Fprintf(out, "    Origin: %s document %d line %d (SHA256 %s; %s)\n", origin.File, origin.Document, origin.Line, origin.Hash, origin.Format)
}
