package deployment

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func composeTestParse(t *testing.T, input string) (Graph, error) {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(input), &doc); err != nil {
		t.Fatal("invalid test fixture")
	}
	if err := validateYAML(&doc); err != nil {
		return Graph{}, err
	}
	return parseCompose(doc.Content[0], Origin{File: "/declarations/compose.yaml", Hash: "manifest-hash", Format: "compose"})
}

func composeTestNode(t *testing.T, graph Graph, id ID) Node {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("missing node %#v", id)
	return Node{}
}

func composeTestEdge(t *testing.T, graph Graph, from, to ID, kind string) Edge {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.From == from && edge.To == to && edge.Kind == kind {
			return edge
		}
	}
	t.Fatalf("missing %s edge %#v -> %#v", kind, from, to)
	return Edge{}
}

func composeTestPrivate(t *testing.T, graph Graph, err error, sentinels ...string) {
	t.Helper()
	encoded, marshalErr := json.Marshal(graph)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	output := string(encoded)
	if err != nil {
		output += err.Error()
	}
	for _, sentinel := range sentinels {
		if strings.Contains(output, sentinel) {
			t.Fatalf("private input appeared in graph or error: %q", sentinel)
		}
	}
}

func TestComposeDeclaredTopologyAndProvenance(t *testing.T) {
	input := `services:
  api:
    image: example/api:1
    depends_on:
      db:
        condition: service_healthy
      job:
        condition: service_completed_successfully
    networks: [back]
    volumes: ['data:/data', './PRIVATE_HOST_PATH:/host:ro']
    ports:
      - '127.0.0.1:8080:80/tcp'
      - target: 53
        published: '5353'
        protocol: udp
  db:
    image: postgres:16
    networks: [back]
  job:
    image: worker:1
    depends_on: [db]
networks:
  back:
    external: true
    name: shared-back
volumes:
  data:
    external: true
`
	graph, err := composeTestParse(t, input)
	if err != nil {
		t.Fatal(err)
	}
	if graph.Incomplete {
		t.Fatalf("supported desired declarations are incomplete: %#v", graph.Diagnostics)
	}
	api := ID{Kind: "compose-service", Name: "api"}
	db := ID{Kind: "compose-service", Name: "db"}
	job := ID{Kind: "compose-service", Name: "job"}
	for _, want := range []struct {
		from, to  ID
		condition string
	}{
		{api, db, "service_healthy"}, {api, job, "service_completed_successfully"}, {job, db, "service_started"},
	} {
		if got := composeTestEdge(t, graph, want.from, want.to, "depends-on").Detail; got != want.condition {
			t.Fatalf("startup condition = %q, want %q", got, want.condition)
		}
	}
	composeTestEdge(t, graph, api, ID{Kind: "image", Name: "example/api:1"}, "uses-image")
	composeTestEdge(t, graph, api, ID{Kind: "network", Name: "back"}, "network-member")
	composeTestEdge(t, graph, api, ID{Kind: "volume", Name: "data"}, "volume-mount")
	composeTestEdge(t, graph, job, ID{Kind: "network", Name: "default"}, "network-member")
	back := composeTestNode(t, graph, ID{Kind: "network", Name: "back"})
	if !strings.Contains(back.Name, "shared-back") || !strings.Contains(back.Name, "existence unknown") {
		t.Fatalf("external desired network lacks explicit uncertainty: %#v", back)
	}
	if !strings.Contains(composeTestNode(t, graph, ID{Kind: "volume", Name: "data"}).Name, "existence unknown") {
		t.Fatal("external volume incorrectly implies observed existence")
	}
	ports := composeTestNode(t, graph, api).Ports
	if len(ports) != 2 || ports[0].Published != "8080" || ports[0].Target != "80" || ports[0].Protocol != "tcp" || ports[1].Published != "5353" || ports[1].Target != "53" || ports[1].Protocol != "udp" {
		t.Fatalf("declared ports = %#v", ports)
	}
	if ports[0].Origin.Line != 12 || ports[1].Origin.Line != 13 {
		t.Fatalf("port provenance = %#v", ports)
	}
	if len(graph.Sources) != 1 || graph.Sources[0].Document != 1 || graph.Sources[0].Hash != "manifest-hash" {
		t.Fatalf("manifest provenance = %#v", graph.Sources)
	}
	seen := make(map[ID]bool)
	for _, node := range graph.Nodes {
		if seen[node.ID] {
			t.Fatalf("duplicate node ID %#v", node.ID)
		}
		seen[node.ID] = true
		if node.Origin.File != graph.Sources[0].File || node.Origin.Hash != "manifest-hash" || node.Origin.Document != 1 || node.Origin.Line <= 0 {
			t.Fatalf("node lacks manifest provenance: %#v", node)
		}
	}
	for _, edge := range graph.Edges {
		if !seen[edge.From] || !seen[edge.To] || edge.Origin.Document != 1 || edge.Origin.Line <= 0 {
			t.Fatalf("edge lacks explicit endpoint/provenance: %#v", edge)
		}
		switch edge.Kind {
		case "uses-image", "depends-on", "network-member", "volume-mount":
		default:
			t.Fatalf("invented connection kind %q", edge.Kind)
		}
	}
	composeTestPrivate(t, graph, nil, "PRIVATE_HOST_PATH", "127.0.0.1")
	for range 12 {
		next, err := composeTestParse(t, input)
		if err != nil || !reflect.DeepEqual(graph, next) {
			t.Fatalf("declarations produce nondeterministic graphs: %v", err)
		}
	}
}

func TestComposeSharedImagesAndOwnerScopedMissingImages(t *testing.T) {
	graph, err := composeTestParse(t, `services:
  api: {image: app:1}
  worker: {image: app:1}
  build_only: {build: ./PRIVATE_BUILD_CONTEXT}
  absent: {}
`)
	if err != nil {
		t.Fatal(err)
	}
	images, unknown := 0, 0
	for _, node := range graph.Nodes {
		switch node.ID.Kind {
		case "image":
			images++
		case "unknown-artifact":
			unknown++
		}
	}
	if images != 1 || unknown != 2 || !graph.Incomplete {
		t.Fatalf("literal images must deduplicate, missing outputs must remain owner-scoped: %#v", graph)
	}
	for _, name := range []string{"api", "worker"} {
		composeTestEdge(t, graph, ID{Kind: "compose-service", Name: name}, ID{Kind: "image", Name: "app:1"}, "uses-image")
	}
	composeTestPrivate(t, graph, nil, "PRIVATE_BUILD_CONTEXT")
}

func TestComposeDeclaredDefaultNetworkIsNotReplaced(t *testing.T) {
	graph, err := composeTestParse(t, `services:
  api: {image: app:1}
  isolated: {image: app:1, network_mode: none}
networks:
  default: {external: true, name: shared-default}
`)
	if err != nil {
		t.Fatal(err)
	}
	defaultID := ID{Kind: "network", Name: "default"}
	composeTestEdge(t, graph, ID{Kind: "compose-service", Name: "api"}, defaultID, "network-member")
	if got := composeTestNode(t, graph, defaultID).Name; !strings.Contains(got, "shared-default") || !strings.Contains(got, "existence unknown") {
		t.Fatalf("implicit membership overwrote explicit default declaration: %q", got)
	}
	for _, edge := range graph.Edges {
		if edge.From.Name == "isolated" && edge.Kind == "network-member" {
			t.Fatalf("network_mode acquired invented membership: %#v", edge)
		}
	}
}

func TestComposeImageBuildAndUnknownReferences(t *testing.T) {
	input := `services:
  api:
    image: '${PRIVATE_IMAGE}'
    depends_on: ['${PRIVATE_SERVICE}', missing]
    networks: ['${PRIVATE_NETWORK}', '${PRIVATE_NETWORK}']
    volumes: ['${PRIVATE_VOLUME:-data}:/data', '${PRIVATE_VOLUME:-data}:/other']
    ports: ['${PRIVATE_PORT}:80', {target: '${PRIVATE_TARGET}', published: '8080'}]
  worker:
    build:
      context: ./PRIVATE_BUILD_CONTEXT
      args: {PASSWORD: PRIVATE_BUILD_SECRET}
    depends_on: ['${PRIVATE_SERVICE}']
    networks: ['${PRIVATE_NETWORK}']
    volumes: [{type: volume, source: '${PRIVATE_VOLUME}', target: /data}]
`
	graph, err := composeTestParse(t, input)
	if err != nil {
		t.Fatal(err)
	}
	if !graph.Incomplete {
		t.Fatal("unresolved declarations must be incomplete")
	}
	counts := make(map[string]int)
	seen := make(map[ID]bool)
	for _, node := range graph.Nodes {
		if seen[node.ID] {
			t.Fatalf("unknown references share an identity: %#v", node.ID)
		}
		seen[node.ID] = true
		counts[node.ID.Kind]++
	}
	if counts["unknown-artifact"] != 2 || counts["unknown-service"] != 3 || counts["unknown-network"] != 3 || counts["unknown-volume"] != 3 || counts["image"] != 0 {
		t.Fatalf("unknown references were resolved or coalesced: %#v", counts)
	}
	if got := composeTestNode(t, graph, ID{Kind: "compose-service", Name: "api"}).Ports; len(got) != 0 {
		t.Fatalf("unresolved ports must not create bindings: %#v", got)
	}
	for _, edge := range graph.Edges {
		if !seen[edge.From] || !seen[edge.To] {
			t.Fatalf("dangling unknown endpoint: %#v", edge)
		}
	}
	composeTestPrivate(t, graph, nil, "PRIVATE_IMAGE", "PRIVATE_SERVICE", "PRIVATE_NETWORK", "PRIVATE_VOLUME", "PRIVATE_PORT", "PRIVATE_TARGET", "PRIVATE_BUILD_CONTEXT", "PRIVATE_BUILD_SECRET")
}

func TestComposeUnsupportedInfluentialDeclarations(t *testing.T) {
	graph, err := composeTestParse(t, `include: [/PRIVATE_INCLUDE/compose.yaml]
services:
  api:
    image: example/api:1
    extends: {file: /PRIVATE_EXTENDS/compose.yaml, service: PRIVATE_INHERITED}
    profiles: [PRIVATE_PROFILE]
    network_mode: 'service:PRIVATE_NETWORK_MODE'
    environment: {TOKEN: PRIVATE_ENVIRONMENT}
    env_file: [/PRIVATE_ENV_FILE]
    secrets: [PRIVATE_SECRET]
    configs: [PRIVATE_CONFIG]
secrets:
  PRIVATE_SECRET: {file: /PRIVATE_SECRET_PATH}
configs:
  PRIVATE_CONFIG: {content: PRIVATE_CONFIG_CONTENT}
`)
	if err != nil {
		t.Fatal(err)
	}
	if !graph.Incomplete || len(graph.Diagnostics) < 4 {
		t.Fatalf("influential unsupported declarations need diagnostics: %#v", graph)
	}
	for _, edge := range graph.Edges {
		if edge.Kind == "network-member" {
			t.Fatalf("network_mode must not acquire implicit default: %#v", edge)
		}
	}
	composeTestPrivate(t, graph, nil, "PRIVATE_INCLUDE", "PRIVATE_EXTENDS", "PRIVATE_INHERITED", "PRIVATE_PROFILE", "PRIVATE_NETWORK_MODE", "PRIVATE_ENVIRONMENT", "PRIVATE_ENV_FILE", "PRIVATE_SECRET", "PRIVATE_CONFIG")
}

func TestComposeDefaultNetworkAndPorts(t *testing.T) {
	for name, networks := range map[string]string{"absent": "", "empty list": "    networks: []\n", "empty map": "    networks: {}\n", "null": "    networks: null\n"} {
		t.Run(name, func(t *testing.T) {
			graph, err := composeTestParse(t, "services:\n  api:\n    image: app:1\n"+networks+"    ports: [80, '8000-8002:80-82/udp', '[::1]:8443:443']\n")
			if err != nil {
				t.Fatal(err)
			}
			api := ID{Kind: "compose-service", Name: "api"}
			composeTestEdge(t, graph, api, ID{Kind: "network", Name: "default"}, "network-member")
			ports := composeTestNode(t, graph, api).Ports
			if len(ports) != 3 || ports[0].Published != "" || ports[0].Target != "80" || ports[1].Published != "8000-8002" || ports[1].Target != "80-82" || ports[1].Protocol != "udp" || ports[2].Published != "8443" || ports[2].Target != "443" {
				t.Fatalf("short port declarations = %#v", ports)
			}
		})
	}
}

func TestComposeNamedAndBindMounts(t *testing.T) {
	graph, err := composeTestParse(t, `services:
  api:
    image: app:1
    volumes:
      - data:/data:ro
      - {type: volume, source: data, target: /other}
      - ./PRIVATE_RELATIVE:/host
      - /PRIVATE_ABSOLUTE:/host
      - 'C:\PRIVATE_WINDOWS:/host'
      - {type: bind, source: /PRIVATE_LONG_BIND, target: /host}
      - /anonymous
volumes:
  data: {}
`)
	if err != nil {
		t.Fatal(err)
	}
	mounts := 0
	for _, edge := range graph.Edges {
		if edge.Kind == "volume-mount" {
			mounts++
			if edge.To != (ID{Kind: "volume", Name: "data"}) {
				t.Fatalf("bind or anonymous mount acquired named identity: %#v", edge)
			}
		}
	}
	if mounts != 2 {
		t.Fatalf("named volume mount count = %d", mounts)
	}
	composeTestPrivate(t, graph, nil, "PRIVATE_RELATIVE", "PRIVATE_ABSOLUTE", "PRIVATE_WINDOWS", "PRIVATE_LONG_BIND")
}

func TestComposeMalformedSupportedDeclarationsAreSafe(t *testing.T) {
	cases := map[string]string{
		"root":                      "[]",
		"missing services":          "networks: {}",
		"empty services":            "services: {}",
		"project name type":         "name: [PRIVATE_VALUE]\nservices: {api: {image: app:1}}",
		"version type":              "version: {PRIVATE_VALUE: true}\nservices: {api: {image: app:1}}",
		"service scalar":            "services: {api: PRIVATE_VALUE}",
		"service name substitution": "services: {'${PRIVATE_VALUE}': {image: app:1}}",
		"image type":                "services: {api: {image: [PRIVATE_VALUE]}}",
		"build type":                "services: {api: {build: [PRIVATE_VALUE]}}",
		"dependencies scalar":       "services: {api: {image: app:1, depends_on: PRIVATE_VALUE}}",
		"dependency long scalar":    "services: {api: {image: app:1, depends_on: {db: PRIVATE_VALUE}}}",
		"dependency condition":      "services: {api: {image: app:1, depends_on: {db: {condition: PRIVATE_VALUE}}}}",
		"networks scalar":           "services: {api: {image: app:1, networks: PRIVATE_VALUE}}",
		"missing network":           "services: {api: {image: app:1, networks: [PRIVATE_VALUE]}}",
		"network definition":        "services: {api: {image: app:1}}\nnetworks: {back: PRIVATE_VALUE}",
		"external type":             "services: {api: {image: app:1}}\nnetworks: {back: {external: PRIVATE_VALUE}}",
		"volumes scalar":            "services: {api: {image: app:1, volumes: PRIVATE_VALUE}}",
		"missing volume":            "services: {api: {image: app:1, volumes: ['PRIVATE_VALUE:/data']}}",
		"mount target":              "services: {api: {image: app:1, volumes: [{type: volume, source: data, target: [PRIVATE_VALUE]}]}}",
		"ports scalar":              "services: {api: {image: app:1, ports: PRIVATE_VALUE}}",
		"port syntax":               "services: {api: {image: app:1, ports: ['PRIVATE_VALUE:80:80:80']}}",
		"port number":               "services: {api: {image: app:1, ports: [PRIVATE_VALUE]}}",
		"port overflow":             "services: {api: {image: app:1, ports: ['99999:80']}}",
		"port descending range":     "services: {api: {image: app:1, ports: ['9000-8000:80']}}",
		"port protocol":             "services: {api: {image: app:1, ports: ['80/PRIVATE_VALUE']}}",
		"long port target":          "services: {api: {image: app:1, ports: [{target: [PRIVATE_VALUE]}]}}",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			graph, err := composeTestParse(t, input)
			if !errors.Is(err, ErrInput) {
				t.Fatalf("expected input error, got %v", err)
			}
			if !reflect.DeepEqual(graph, Graph{}) {
				t.Fatalf("invalid declaration returned partial graph: %#v", graph)
			}
			composeTestPrivate(t, graph, err, "PRIVATE_VALUE")
		})
	}
}

func TestComposeExplicitManifestNeverReadsEnvOrIncludes(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("IMAGE=PRIVATE_DOTENV_VALUE\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "included.yaml"), []byte("PRIVATE_INVALID_INCLUDED_YAML: ["), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("include: [included.yaml]\nservices:\n  api:\n    image: '${IMAGE}'\n    env_file: [missing.env]\n    environment: {PASSWORD: PRIVATE_INLINE_VALUE}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	graph, err := LoadManifest(manifest, "compose")
	if err != nil {
		t.Fatal(err)
	}
	if !graph.Incomplete || len(graph.Sources) != 1 || graph.Sources[0].File != manifest {
		t.Fatalf("only explicit manifest should be used: %#v", graph)
	}
	unknown := 0
	for _, node := range graph.Nodes {
		if node.ID.Kind == "unknown-artifact" {
			unknown++
		}
	}
	if unknown != 1 {
		t.Fatal("image interpolation was unexpectedly resolved")
	}
	composeTestPrivate(t, graph, nil, "PRIVATE_DOTENV_VALUE", "PRIVATE_INVALID_INCLUDED_YAML", "PRIVATE_INLINE_VALUE", "${IMAGE}", "missing.env")
}
