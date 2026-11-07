package deployment

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// parseCompose projects declarations only. It never resolves interpolation,
// reads env files or included files, or invokes Compose or a build tool.
func parseCompose(doc *yaml.Node, source Origin) (Graph, error) {
	if source.Document == 0 {
		source.Document = 1
	}
	root, err := mapping(doc)
	if err != nil {
		return Graph{}, err
	}
	if value := root["name"]; value != nil {
		if _, err := text(value); err != nil {
			return Graph{}, err
		}
	}
	if value := root["version"]; value != nil {
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" && value.Tag != "!!int" && value.Tag != "!!float" {
			return Graph{}, invalid(value, "version must be a scalar version declaration")
		}
	}
	services, err := mapping(root["services"])
	if err != nil {
		return Graph{}, err
	}
	if len(services) == 0 {
		return Graph{}, invalid(root["services"], "services must not be empty")
	}
	p := composeParser{graph: Graph{Sources: []Origin{source}}, source: source, seen: make(map[ID]bool), services: services, networks: make(map[string]bool), volumes: make(map[string]bool)}
	for _, field := range []string{"include", "extends"} {
		if n := root[field]; n != nil {
			diagnostic(&p.graph, "unsupported-compose", "Compose includes and inheritance are not evaluated", at(source, n), true)
		}
	}
	serviceIndexes := make(map[string]int, len(services))
	for _, name := range keys(services) {
		if !composeLiteralName(name) {
			return Graph{}, invalid(services[name], "service name must be a literal identifier")
		}
		if _, err := mapping(services[name]); err != nil {
			return Graph{}, err
		}
		id := ID{Kind: "compose-service", Name: name}
		serviceIndexes[name] = len(p.graph.Nodes)
		p.addNode(Node{ID: id, Name: name, Origin: at(source, services[name])})
	}
	if err := p.resources(root["networks"], "network", p.networks); err != nil {
		return Graph{}, err
	}
	if err := p.resources(root["volumes"], "volume", p.volumes); err != nil {
		return Graph{}, err
	}
	for _, name := range keys(services) {
		n := services[name]
		service, _ := mapping(n)
		owner := ID{Kind: "compose-service", Name: name}
		for _, field := range []string{"extends", "profiles", "network_mode", "links", "external_links", "volumes_from"} {
			if value := service[field]; value != nil {
				diagnostic(&p.graph, "unsupported-compose", "A service declaration uses unsupported topology or selection semantics; it was not evaluated", at(source, value), true)
			}
		}
		ports, err := p.ports(service["ports"])
		if err != nil {
			return Graph{}, err
		}
		p.graph.Nodes[serviceIndexes[name]].Ports = ports
		image := ""
		imageOrigin := at(source, n)
		if value := service["image"]; value != nil {
			image, err = text(value)
			if err != nil {
				return Graph{}, err
			}
			imageOrigin = at(source, value)
		} else if value := service["build"]; value != nil {
			if value.Kind != yaml.MappingNode && (value.Kind != yaml.ScalarNode || value.Tag != "!!str") {
				return Graph{}, invalid(value, "build must be a string or mapping")
			}
			imageOrigin = at(source, value)
		}
		addImage(&p.graph, p.seen, image, owner, imageOrigin, "")
		if err := p.dependencies(owner, service["depends_on"]); err != nil {
			return Graph{}, err
		}
		networkUnknown := service["network_mode"] != nil || service["extends"] != nil
		if err := p.memberships(owner, service["networks"], networkUnknown, n); err != nil {
			return Graph{}, err
		}
		if err := p.mounts(owner, service["volumes"]); err != nil {
			return Graph{}, err
		}
	}
	return p.graph, nil
}

type composeParser struct {
	graph             Graph
	source            Origin
	seen              map[ID]bool
	services          map[string]*yaml.Node
	networks, volumes map[string]bool
}

func (p *composeParser) addNode(node Node) {
	if !p.seen[node.ID] {
		p.seen[node.ID] = true
		p.graph.Nodes = append(p.graph.Nodes, node)
	}
}

func composeLiteralName(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-') {
			return false
		}
	}
	return true
}

func (p *composeParser) resources(n *yaml.Node, kind string, declared map[string]bool) error {
	if n == nil {
		return nil
	}
	resources, err := mapping(n)
	if err != nil {
		return err
	}
	for _, name := range keys(resources) {
		value := resources[name]
		if !composeLiteralName(name) {
			return invalid(value, "resource name must be a literal identifier")
		}
		display := name
		if value.Tag != "!!null" {
			fields, err := mapping(value)
			if err != nil {
				return err
			}
			if external := fields["external"]; external != nil {
				if external.Tag != "!!bool" {
					return invalid(external, "external must be a boolean")
				}
				if external.Value == "true" {
					display += " (external declaration; existence unknown)"
				}
			}
			if actual := fields["name"]; actual != nil {
				literal, err := text(actual)
				if err != nil {
					return err
				}
				if !composeLiteralName(literal) {
					diagnostic(&p.graph, "unknown-resource-name", "A resource name is not a supported literal; it was not expanded", at(p.source, actual), true)
				} else {
					display = literal + strings.TrimPrefix(display, name)
				}
			}
		}
		declared[name] = true
		p.addNode(Node{ID: ID{Kind: kind, Name: name}, Name: display, Origin: at(p.source, value)})
	}
	return nil
}

func (p *composeParser) dependencies(owner ID, n *yaml.Node) error {
	if n == nil {
		return nil
	}
	type dependency struct {
		name, condition string
		node            *yaml.Node
	}
	var dependencies []dependency
	switch n.Kind {
	case yaml.SequenceNode:
		for _, value := range n.Content {
			name, err := text(value)
			if err != nil {
				return err
			}
			dependencies = append(dependencies, dependency{name, "service_started", value})
		}
	case yaml.MappingNode:
		entries, _ := mapping(n)
		for _, name := range keys(entries) {
			fields, err := mapping(entries[name])
			if err != nil {
				return err
			}
			condition := "service_started"
			if value := fields["condition"]; value != nil {
				condition, err = text(value)
				if err != nil {
					return err
				}
			}
			switch condition {
			case "service_started", "service_healthy", "service_completed_successfully":
			default:
				return invalid(entries[name], "unsupported dependency condition")
			}
			for _, field := range []string{"required", "restart"} {
				if value := fields[field]; value != nil {
					if value.Tag != "!!bool" {
						return invalid(value, "dependency option must be boolean")
					}
					if field == "required" && value.Value == "false" || field == "restart" && value.Value == "true" {
						condition += "; " + field + "=" + value.Value
					}
				}
			}
			dependencies = append(dependencies, dependency{name, condition, entries[name]})
		}
	default:
		return invalid(n, "depends_on must be a list or mapping")
	}
	for i, dependency := range dependencies {
		target := ID{Kind: "compose-service", Name: dependency.name}
		if !composeLiteralName(dependency.name) || p.services[dependency.name] == nil {
			target = ID{Kind: "unknown-service", Namespace: owner.Name, Name: "dependency-" + strconv.Itoa(i+1)}
			display := "unknown startup dependency (not expanded)"
			if composeLiteralName(dependency.name) {
				display = dependency.name + " (undeclared service)"
			}
			p.addNode(Node{ID: target, Name: display, Origin: at(p.source, dependency.node)})
			diagnostic(&p.graph, "unknown-dependency", "A startup dependency has no known declared service; no runtime communication is inferred", at(p.source, dependency.node), true)
		}
		p.graph.Edges = append(p.graph.Edges, Edge{From: owner, To: target, Kind: "depends-on", Detail: dependency.condition, Origin: at(p.source, dependency.node)})
	}
	return nil
}

func (p *composeParser) memberships(owner ID, n *yaml.Node, suppressDefault bool, service *yaml.Node) error {
	if n == nil || n.Tag == "!!null" {
		if suppressDefault {
			return nil
		}
		p.addNode(Node{ID: ID{Kind: "network", Name: "default"}, Name: "default", Origin: at(p.source, service)})
		p.graph.Edges = append(p.graph.Edges, Edge{From: owner, To: ID{Kind: "network", Name: "default"}, Kind: "network-member", Origin: at(p.source, service)})
		return nil
	}
	var names []string
	var origins []*yaml.Node
	switch n.Kind {
	case yaml.SequenceNode:
		for _, value := range n.Content {
			name, err := text(value)
			if err != nil {
				return err
			}
			names = append(names, name)
			origins = append(origins, value)
		}
	case yaml.MappingNode:
		entries, _ := mapping(n)
		for _, name := range keys(entries) {
			if entries[name].Tag != "!!null" {
				if _, err := mapping(entries[name]); err != nil {
					return err
				}
			}
			names = append(names, name)
			origins = append(origins, entries[name])
		}
	default:
		return invalid(n, "networks must be a list or mapping")
	}
	if len(names) == 0 && !suppressDefault {
		return p.memberships(owner, nil, false, service)
	}
	for i, name := range names {
		target := ID{Kind: "network", Name: name}
		if !composeLiteralName(name) {
			target = ID{Kind: "unknown-network", Namespace: owner.Name, Name: "network-" + strconv.Itoa(i+1)}
			p.addNode(Node{ID: target, Name: "unknown network (not expanded)", Origin: at(p.source, origins[i])})
			diagnostic(&p.graph, "unknown-network", "A network reference was not expanded or resolved", at(p.source, origins[i]), true)
		} else if name == "default" {
			p.addNode(Node{ID: target, Name: "default", Origin: at(p.source, origins[i])})
		} else if !p.networks[name] {
			return invalid(origins[i], "network reference has no declaration")
		}
		p.graph.Edges = append(p.graph.Edges, Edge{From: owner, To: target, Kind: "network-member", Origin: at(p.source, origins[i])})
	}
	return nil
}

func (p *composeParser) mounts(owner ID, n *yaml.Node) error {
	if n == nil {
		return nil
	}
	if n.Kind != yaml.SequenceNode {
		return invalid(n, "volumes must be a list")
	}
	for i, value := range n.Content {
		kind, source := "volume", ""
		if value.Kind == yaml.MappingNode {
			fields, _ := mapping(value)
			var err error
			kind, err = text(fields["type"])
			if err != nil {
				return err
			}
			if target := fields["target"]; target == nil || target.Kind != yaml.ScalarNode || target.Tag != "!!str" || target.Value == "" {
				return invalid(target, "mount target must be a nonempty string")
			}
			if field := fields["source"]; field != nil {
				source, err = text(field)
				if err != nil {
					return err
				}
			}
		} else {
			entry, err := text(value)
			if err != nil {
				return err
			}
			// Interpolation may contain colons and slashes; it must not be
			// parsed as a literal bind path or named-volume reference.
			if strings.Contains(entry, "$") {
				target := ID{Kind: "unknown-volume", Namespace: owner.Name, Name: "volume-" + strconv.Itoa(i+1)}
				p.addNode(Node{ID: target, Name: "unknown mount (not expanded)", Origin: at(p.source, value)})
				diagnostic(&p.graph, "unknown-volume", "A mount declaration was not expanded or resolved", at(p.source, value), true)
				p.graph.Edges = append(p.graph.Edges, Edge{From: owner, To: target, Kind: "volume-mount", Origin: at(p.source, value)})
				continue
			}
			parts := strings.Split(entry, ":")
			if entry == "" || len(parts) > 3 && !(len(entry) > 1 && entry[1] == ':') {
				return invalid(value, "invalid short volume declaration")
			}
			if len(parts) > 1 {
				source = parts[0]
				if source == "" || parts[1] == "" {
					return invalid(value, "invalid short volume declaration")
				}
				if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "~") || strings.ContainsAny(source, "/\\") || len(entry) > 1 && entry[1] == ':' {
					kind = "bind"
				}
			}
		}
		if kind == "bind" {
			diagnostic(&p.graph, "bind-mount-omitted", "Bind mounts are omitted; host paths are not retained", at(p.source, value), false)
			continue
		}
		if kind != "volume" {
			diagnostic(&p.graph, "unsupported-mount", "A mount type is unsupported and was not evaluated", at(p.source, value), true)
			continue
		}
		if source == "" {
			diagnostic(&p.graph, "anonymous-volume-omitted", "Anonymous volumes are omitted from the named-volume projection", at(p.source, value), false)
			continue
		}
		target := ID{Kind: "volume", Name: source}
		if !composeLiteralName(source) {
			target = ID{Kind: "unknown-volume", Namespace: owner.Name, Name: "volume-" + strconv.Itoa(i+1)}
			p.addNode(Node{ID: target, Name: "unknown volume (not expanded)", Origin: at(p.source, value)})
			diagnostic(&p.graph, "unknown-volume", "A volume reference was not expanded or resolved", at(p.source, value), true)
		} else if !p.volumes[source] {
			return invalid(value, "volume reference has no declaration")
		}
		p.graph.Edges = append(p.graph.Edges, Edge{From: owner, To: target, Kind: "volume-mount", Origin: at(p.source, value)})
	}
	return nil
}

func composePortNumber(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) > 2 {
		return false
	}
	previous := 0
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 || n > 65535 || n < previous {
			return false
		}
		previous = n
	}
	return true
}

func composePortScalar(n *yaml.Node) (string, error) {
	if n == nil || n.Kind != yaml.ScalarNode || n.Tag != "!!str" && n.Tag != "!!int" {
		return "", invalid(n, "port must be a string or integer")
	}
	return n.Value, nil
}

func (p *composeParser) ports(n *yaml.Node) ([]Port, error) {
	if n == nil {
		return nil, nil
	}
	if n.Kind != yaml.SequenceNode {
		return nil, invalid(n, "ports must be a list")
	}
	var ports []Port
	for _, value := range n.Content {
		port := Port{Scope: "host", Protocol: "tcp", Origin: at(p.source, value)}
		if value.Kind == yaml.MappingNode {
			fields, _ := mapping(value)
			var err error
			port.Target, err = composePortScalar(fields["target"])
			if err != nil {
				return nil, err
			}
			if field := fields["published"]; field != nil {
				port.Published, err = composePortScalar(field)
				if err != nil {
					return nil, err
				}
			}
			if field := fields["protocol"]; field != nil {
				port.Protocol, err = text(field)
				if err != nil {
					return nil, err
				}
			}
		} else {
			entry, err := composePortScalar(value)
			if err != nil {
				return nil, err
			}
			if strings.Contains(entry, "$") {
				diagnostic(&p.graph, "unknown-port", "A port declaration is unresolved; no binding was inferred", at(p.source, value), true)
				continue
			}
			parts := strings.Split(entry, "/")
			if len(parts) > 2 {
				return nil, invalid(value, "invalid short port declaration")
			}
			if len(parts) == 2 {
				port.Protocol = parts[1]
			}
			binding := parts[0]
			if strings.HasPrefix(binding, "[") {
				end := strings.Index(binding, "]:")
				if end < 0 {
					return nil, invalid(value, "invalid short port declaration")
				}
				binding = binding[end+2:]
			} else if strings.Count(binding, ":") == 2 {
				binding = binding[strings.IndexByte(binding, ':')+1:]
			}
			fields := strings.Split(binding, ":")
			switch len(fields) {
			case 1:
				port.Target = fields[0]
			case 2:
				port.Published, port.Target = fields[0], fields[1]
			default:
				return nil, invalid(value, "invalid short port declaration")
			}
		}
		if strings.ContainsAny(port.Target+port.Published+port.Protocol, "$") {
			diagnostic(&p.graph, "unknown-port", "A port declaration is unresolved; no binding was inferred", at(p.source, value), true)
			continue
		}
		if !composePortNumber(port.Target) || port.Published != "" && !composePortNumber(port.Published) {
			return nil, invalid(value, "invalid port number or range")
		}
		if port.Protocol != "tcp" && port.Protocol != "udp" && port.Protocol != "sctp" {
			return nil, invalid(value, "unsupported port protocol")
		}
		ports = append(ports, port)
	}
	return ports, nil
}
