package plantuml

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Mino829/umlgen/internal/deployment"
)

type DeploymentOptions struct{ Title string }

func GenerateDeployment(graph deployment.Graph, opts DeploymentOptions) (string, error) {
	aliases := make(map[deployment.ID]string, len(graph.Nodes))
	var out strings.Builder
	out.WriteString("@startuml\n")
	if opts.Title != "" {
		fmt.Fprintf(&out, "title %s\n", packageText(opts.Title))
	}
	for i, node := range graph.Nodes {
		if _, ok := aliases[node.ID]; ok {
			return "", fmt.Errorf("duplicate deployment endpoint")
		}
		alias := "D" + strconv.Itoa(i)
		aliases[node.ID] = alias
		shape, stereotype := "rectangle", "declared workload"
		switch node.ID.Kind {
		case "component":
			shape = "component"
			stereotype = "configured component"
		case "compose-service":
			stereotype = "declared Compose service"
		case "Deployment", "StatefulSet", "DaemonSet", "Job", "Pod":
			stereotype = "declared " + node.ID.Kind
		case "Service":
			shape = "component"
			stereotype = "declared Kubernetes Service"
		case "image":
			shape = "artifact"
			stereotype = "declared image"
		case "unknown-artifact":
			shape = "artifact"
			stereotype = "unknown image"
		case "network":
			shape = "cloud"
			stereotype = "declared network"
		case "volume":
			shape = "folder"
			stereotype = "declared volume"
		case "external-endpoint":
			shape = "cloud"
			stereotype = "declared external alias"
		case "unknown-endpoint", "unknown-service", "unknown-network", "unknown-volume":
			shape = "cloud"
			stereotype = "unknown"
		default:
			return "", fmt.Errorf("unsupported deployment node kind")
		}
		label := packageText(node.Name)
		switch node.ID.Kind {
		case "Deployment", "StatefulSet", "DaemonSet", "Job", "Pod", "Service":
			namespace := node.ID.Namespace
			if namespace == "" {
				namespace = "(namespace unspecified)"
			}
			label += "\\n" + packageText(namespace)
		}
		if node.Replicas != nil {
			label += "\\ndesired replicas " + strconv.Itoa(*node.Replicas) + " (not observed)"
		}
		for _, port := range node.Ports {
			scope := port.Scope
			if scope == "" {
				scope = "configured"
			}
			published := port.Published
			if published == "" {
				published = "unknown (dynamic)"
			}
			label += "\\n" + packageText(scope+" port "+published+" -> "+port.Target+"/"+port.Protocol+" (declared)")
		}
		fmt.Fprintf(&out, "%s \"%s\" as %s <<%s>>\n", shape, label, alias, stereotype)
	}
	for _, edge := range graph.Edges {
		from, okFrom := aliases[edge.From]
		to, okTo := aliases[edge.To]
		if !okFrom || !okTo {
			return "", fmt.Errorf("deployment edge has undeclared endpoint")
		}
		arrow, label := "..>", ""
		switch edge.Kind {
		case "component-mapping":
			label = "explicit component mapping; source/image proof unspecified"
		case "uses-image":
			arrow = "-->"
			label = "declared image"
		case "depends-on":
			label = "configured startup dependency; NOT traffic"
		case "network-member":
			arrow = "--"
			label = "declared network membership"
		case "volume-mount":
			arrow = "--"
			label = "declared named volume mount"
		case "selector-candidate":
			label = "selector candidate; NOT observed endpoint"
		case "endpoint-unknown":
			label = "endpoint unknown"
		case "external-name":
			label = "declared DNS alias; resolution unobserved"
		default:
			return "", fmt.Errorf("unsupported deployment relation")
		}
		if edge.Detail != "" {
			label += "\\n" + packageText(edge.Detail)
		}
		fmt.Fprintf(&out, "%s %s %s : %s\n", from, arrow, to, label)
	}
	out.WriteString("\nlegend bottom\nDeclared topology only; NOT observed placement, health, endpoints or network traffic\nRuntime nodes/instances, image availability and build validity are unknown\nCompose startup dependencies are not HTTP/RPC calls; selector matches are candidates only\nComponents bind only by explicit mapping; source-to-image/build correspondence is unverified\nNo environment expansion, manifest merge, include, build, Docker or cluster query\nEnvironment/Secret/ConfigMap values and raw YAML snippets are omitted\n")
	if graph.Incomplete {
		out.WriteString("INCOMPLETE: unsupported/unresolved declarations remain; absence is not proof\n")
	}
	type inputIdentity struct{ file, hash, format string }
	inputs := make(map[inputIdentity]bool, len(graph.Sources))
	for _, source := range graph.Sources {
		key := inputIdentity{source.File, source.Hash, source.Format}
		if inputs[key] {
			continue
		}
		inputs[key] = true
		fmt.Fprintf(&out, "Input %s: %s; SHA256 %s\n", packageText(source.Format), packageText(source.File), packageText(source.Hash))
	}
	for _, d := range graph.Diagnostics {
		fmt.Fprintf(&out, "Diagnostic %s: %s [%s doc %d:%d]\n", packageText(d.Code), packageText(d.Message), packageText(d.Origin.File), d.Origin.Document, d.Origin.Line)
	}
	out.WriteString("endlegend\n@enduml\n")
	return out.String(), nil
}
