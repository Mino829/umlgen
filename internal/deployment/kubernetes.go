package deployment

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Labels and selectors live only for this parse, never in the returned graph.
type kubernetesWorkload struct {
	id     ID
	labels map[string]string
}

type kubernetesService struct {
	id       ID
	selector map[string]string
	origin   Origin
}

type kubernetesParser struct {
	graph     Graph
	resources map[ID]bool
	nodes     map[ID]bool
	workloads []kubernetesWorkload
	services  []kubernetesService
}

func parseKubernetes(docs []*yaml.Node, source Origin) (Graph, error) {
	p := kubernetesParser{resources: make(map[ID]bool), nodes: make(map[ID]bool)}
	if len(docs) == 0 {
		return Graph{}, invalid(nil, "empty Kubernetes manifest")
	}
	for i, doc := range docs {
		origin := source
		origin.Document = i + 1
		if doc != nil && doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
			doc = doc.Content[0]
		}
		origin = at(origin, doc)
		p.graph.Sources = append(p.graph.Sources, origin)
		if doc == nil || doc.Tag == "!!null" {
			continue
		}
		if err := p.resource(doc, origin); err != nil {
			return Graph{}, err
		}
	}
	for _, service := range p.services {
		matched := false
		if len(service.selector) != 0 {
			for _, workload := range p.workloads {
				if workload.id.Namespace != service.id.Namespace || !kubernetesMatches(service.selector, workload.labels) {
					continue
				}
				matched = true
				p.graph.Edges = append(p.graph.Edges, Edge{From: service.id, To: workload.id, Kind: "selector-candidate", Origin: service.origin})
			}
		}
		if !matched {
			p.unknownEndpoint(service.id, service.origin)
		}
	}
	return p.graph, nil
}

func (p *kubernetesParser) resource(doc *yaml.Node, source Origin) error {
	resource, err := mapping(doc)
	if err != nil {
		return err
	}
	version, err := text(resource["apiVersion"])
	if err != nil {
		return err
	}
	kind, err := text(resource["kind"])
	if err != nil {
		return err
	}
	source = at(source, doc)
	if version == "v1" && kind == "List" {
		items := resource["items"]
		if items == nil || items.Kind != yaml.SequenceNode {
			return invalid(items, "Kubernetes List items must be a sequence")
		}
		for _, item := range items.Content {
			if err := p.resource(item, source); err != nil {
				return err
			}
		}
		return nil
	}
	core := version == "v1" && (kind == "Pod" || kind == "Service" || kind == "Secret" || kind == "ConfigMap")
	apps := version == "apps/v1" && (kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet")
	batch := version == "batch/v1" && kind == "Job"
	if !core && !apps && !batch {
		// Unsupported resources are not interpreted, even when their bodies look
		// like a supported kind. Their optional identity is only for duplicates.
		if metadata, ok := resource["metadata"]; ok && metadata.Kind == yaml.MappingNode {
			fields, _ := mapping(metadata)
			name, nameErr := text(fields["name"])
			namespace := ""
			var namespaceErr error
			if fields["namespace"] != nil {
				namespace, namespaceErr = text(fields["namespace"])
			}
			if nameErr == nil && namespaceErr == nil && name != "" {
				if err := p.reserve(ID{Kind: kind, Namespace: namespace, Name: name}, doc); err != nil {
					return err
				}
			}
		}
		diagnostic(&p.graph, "unsupported-resource", "Resource kind or API version is unsupported; no runtime discovery was performed", source, true)
		return nil
	}
	metadata, err := mapping(resource["metadata"])
	if err != nil {
		return err
	}
	name, err := kubernetesName(metadata["name"])
	if err != nil {
		return err
	}
	namespace := ""
	if metadata["namespace"] != nil {
		namespace, err = text(metadata["namespace"])
		if err != nil {
			return err
		}
		if strings.ContainsAny(namespace, " \t\r\n") || kubernetesUnresolved(namespace) {
			return invalid(metadata["namespace"], "namespace must be a literal name")
		}
	}
	id := ID{Kind: kind, Namespace: namespace, Name: name}
	if err := p.reserve(id, doc); err != nil {
		return err
	}
	if kind == "Secret" || kind == "ConfigMap" {
		diagnostic(&p.graph, "omitted-configuration", "Secret or ConfigMap resource was intentionally omitted; payload was not interpreted", source, false)
		return nil
	}
	spec, err := mapping(resource["spec"])
	if err != nil {
		return err
	}
	node := Node{ID: id, Name: name, Origin: source}
	if kind == "Service" {
		return p.service(node, spec)
	}
	if kind == "Deployment" || kind == "StatefulSet" {
		if declared := spec["replicas"]; declared != nil {
			replicas, err := integer(declared)
			if err != nil {
				return err
			}
			node.Replicas = &replicas
		}
	}
	podSpec := spec
	podMetadata := metadata
	if kind != "Pod" {
		template, err := mapping(spec["template"])
		if err != nil {
			return err
		}
		podSpec, err = mapping(template["spec"])
		if err != nil {
			return err
		}
		podMetadata = nil
		if template["metadata"] != nil {
			podMetadata, err = mapping(template["metadata"])
			if err != nil {
				return err
			}
		}
	}
	var labels map[string]string
	if podMetadata["labels"] != nil && podMetadata["labels"].Tag != "!!null" {
		labels, err = stringMap(podMetadata["labels"])
		if err != nil {
			return err
		}
		for key, value := range labels {
			if kubernetesUnresolved(key) || kubernetesUnresolved(value) {
				diagnostic(&p.graph, "unresolved-labels", "Pod labels contain unresolved substitutions; selector candidates are partial and no expansion was performed", at(source, podMetadata["labels"]), true)
				break
			}
		}
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	p.nodes[id] = true
	p.workloads = append(p.workloads, kubernetesWorkload{id: id, labels: labels})
	return p.containers(podSpec, id, source)
}

func (p *kubernetesParser) reserve(id ID, node *yaml.Node) error {
	if p.resources[id] {
		return invalid(node, "duplicate Kubernetes resource identity")
	}
	p.resources[id] = true
	return nil
}

func kubernetesName(node *yaml.Node) (string, error) {
	name, err := text(node)
	if err != nil {
		return "", err
	}
	if name == "" || strings.ContainsAny(name, " \t\r\n") || kubernetesUnresolved(name) {
		return "", invalid(node, "resource or container name must be a nonempty literal")
	}
	return name, nil
}

func (p *kubernetesParser) containers(spec map[string]*yaml.Node, owner ID, source Origin) error {
	seen := make(map[string]bool)
	for _, field := range []string{"containers", "initContainers", "ephemeralContainers"} {
		list := spec[field]
		if list == nil && field != "containers" {
			continue
		}
		if list == nil || list.Kind != yaml.SequenceNode || (field == "containers" && len(list.Content) == 0) {
			return invalid(list, "container declarations must be a sequence with regular containers")
		}
		for _, declaration := range list.Content {
			container, err := mapping(declaration)
			if err != nil {
				return err
			}
			name, err := kubernetesName(container["name"])
			if err != nil {
				return err
			}
			if seen[name] {
				return invalid(declaration, "duplicate container identity")
			}
			seen[name] = true
			image := ""
			imageOrigin := at(source, declaration)
			if container["image"] != nil {
				image, err = text(container["image"])
				if err != nil {
					return err
				}
				imageOrigin = at(source, container["image"])
			}
			if field == "initContainers" {
				name = "init container " + name
			} else if field == "ephemeralContainers" {
				name = "ephemeral container " + name
			}
			addImage(&p.graph, p.nodes, image, owner, imageOrigin, name)
		}
	}
	return nil
}

func (p *kubernetesParser) service(node Node, spec map[string]*yaml.Node) error {
	if ports := spec["ports"]; ports != nil {
		if ports.Kind != yaml.SequenceNode {
			return invalid(ports, "Service ports must be a sequence")
		}
		for _, declaration := range ports.Content {
			port, err := mapping(declaration)
			if err != nil {
				return err
			}
			published, err := kubernetesPort(port["port"], false)
			if err != nil {
				return err
			}
			target := published
			if port["targetPort"] != nil {
				target, err = kubernetesPort(port["targetPort"], true)
				if err != nil {
					return err
				}
			}
			protocol := "TCP"
			if port["protocol"] != nil {
				protocol, err = text(port["protocol"])
				if err != nil {
					return err
				}
				if protocol != "TCP" && protocol != "UDP" && protocol != "SCTP" {
					return invalid(port["protocol"], "unsupported Service port protocol")
				}
			}
			node.Ports = append(node.Ports, Port{Scope: "service", Published: published, Target: target, Protocol: protocol, Origin: at(node.Origin, declaration)})
			if port["nodePort"] != nil {
				nodePort, err := kubernetesPort(port["nodePort"], false)
				if err != nil {
					return err
				}
				node.Ports = append(node.Ports, Port{Scope: "node", Published: nodePort, Target: target, Protocol: protocol, Origin: at(node.Origin, port["nodePort"])})
			}
		}
	}
	p.graph.Nodes = append(p.graph.Nodes, node)
	p.nodes[node.ID] = true
	serviceType := "ClusterIP"
	if spec["type"] != nil {
		var err error
		serviceType, err = text(spec["type"])
		if err != nil {
			return err
		}
	}
	if serviceType == "ExternalName" {
		name, err := text(spec["externalName"])
		if err != nil {
			return err
		}
		if name == "" || strings.ContainsAny(name, " \t\r\n/:@") || kubernetesUnresolved(name) {
			p.unknownEndpoint(node.ID, at(node.Origin, spec["externalName"]))
			return nil
		}
		id := ID{Kind: "external-endpoint", Name: name}
		origin := at(node.Origin, spec["externalName"])
		if !p.nodes[id] {
			p.nodes[id] = true
			p.graph.Nodes = append(p.graph.Nodes, Node{ID: id, Name: name, Origin: origin})
		}
		p.graph.Edges = append(p.graph.Edges, Edge{From: node.ID, To: id, Kind: "external-name", Origin: origin})
		return nil
	}
	if serviceType != "ClusterIP" && serviceType != "NodePort" && serviceType != "LoadBalancer" {
		diagnostic(&p.graph, "unsupported-service-type", "Service type is unsupported; no runtime endpoint discovery was performed", at(node.Origin, spec["type"]), true)
	}
	var selector map[string]string
	origin := node.Origin
	if spec["selector"] != nil {
		var err error
		if spec["selector"].Tag != "!!null" {
			selector, err = stringMap(spec["selector"])
			if err != nil {
				return err
			}
		}
		origin = at(node.Origin, spec["selector"])
	}
	p.services = append(p.services, kubernetesService{id: node.ID, selector: selector, origin: origin})
	return nil
}

func kubernetesPort(node *yaml.Node, named bool) (string, error) {
	if named && node != nil && node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		if node.Value == "" || strings.ContainsAny(node.Value, " \t\r\n/:@") || kubernetesUnresolved(node.Value) {
			return "", invalid(node, "target port must be a literal port name or number")
		}
		return node.Value, nil
	}
	port, err := integer(node)
	if err != nil {
		return "", err
	}
	if port < 1 || port > 65535 {
		return "", invalid(node, "Service port is outside the declared port range")
	}
	return strconv.Itoa(port), nil
}

func kubernetesUnresolved(value string) bool {
	return strings.Contains(value, "$") || strings.Contains(value, "{{")
}

func kubernetesMatches(selector, labels map[string]string) bool {
	for key, expected := range selector {
		actual, exists := labels[key]
		if !exists || actual != expected || kubernetesUnresolved(key) || kubernetesUnresolved(expected) || kubernetesUnresolved(actual) {
			return false
		}
	}
	return true
}

func (p *kubernetesParser) unknownEndpoint(owner ID, source Origin) {
	id := ID{Kind: "unknown-endpoint", Namespace: owner.Namespace, Name: owner.Name}
	p.graph.Nodes = append(p.graph.Nodes, Node{ID: id, Name: "unknown endpoint (not observed)", Origin: source})
	p.nodes[id] = true
	p.graph.Edges = append(p.graph.Edges, Edge{From: owner, To: id, Kind: "endpoint-unknown", Origin: source})
	diagnostic(&p.graph, "unknown-endpoint", "Service has no supported declared endpoint candidate; no endpoints were observed or inferred", source, true)
}
