package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func kubernetesFixture(input string) (Graph, error) {
	decoder := yaml.NewDecoder(strings.NewReader(input))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		if err := decoder.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			return Graph{}, errors.New("invalid fixture YAML")
		}
		if len(doc.Content) == 0 || doc.Content[0].Tag == "!!null" {
			docs = append(docs, nil)
		} else {
			docs = append(docs, doc.Content[0])
		}
	}
	return parseKubernetes(docs, Origin{File: "declared.yaml", Hash: "manifest-hash", Format: "kubernetes"})
}

func requireKubernetes(t *testing.T, input string) Graph {
	t.Helper()
	graph, err := kubernetesFixture(input)
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

func kubernetesNode(t *testing.T, graph Graph, id ID) Node {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("missing declared node %+v", id)
	return Node{}
}

func TestKubernetesSupportedWorkloadsAndLiteralImages(t *testing.T) {
	for _, test := range []struct {
		kind    string
		version string
	}{
		{"Pod", "v1"}, {"Deployment", "apps/v1"}, {"StatefulSet", "apps/v1"}, {"DaemonSet", "apps/v1"}, {"Job", "batch/v1"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			podSpec := "containers: [{name: app, image: registry.example/app@sha256:" + strings.Repeat("a", 64) + "}]\ninitContainers: [{name: prepare, image: registry.example/setup:1}]\n"
			spec := podSpec
			if test.kind != "Pod" {
				spec = "replicas: 0\ntemplate:\n  metadata: {labels: {role: api}}\n  spec:\n" + indentKubernetes(podSpec, "    ")
			}
			input := fmt.Sprintf("apiVersion: %s\nkind: %s\nmetadata: {name: api}\nspec:\n%s", test.version, test.kind, indentKubernetes(spec, "  "))
			graph := requireKubernetes(t, input)
			if graph.Incomplete || len(graph.Diagnostics) != 0 {
				t.Fatalf("supported declarations unexpectedly incomplete: %+v", graph.Diagnostics)
			}
			node := kubernetesNode(t, graph, ID{Kind: test.kind, Name: "api"})
			if test.kind == "Deployment" || test.kind == "StatefulSet" {
				if node.Replicas == nil || *node.Replicas != 0 {
					t.Fatalf("explicit zero desired replicas lost: %+v", node.Replicas)
				}
			} else if node.Replicas != nil {
				t.Fatal("replicas invented for a workload without supported desired replicas")
			}
			if len(graph.Nodes) != 3 || len(graph.Edges) != 2 {
				t.Fatalf("expected workload and two declared images, got %d nodes and %d edges", len(graph.Nodes), len(graph.Edges))
			}
			for _, edge := range graph.Edges {
				if edge.From != node.ID || edge.Kind != "uses-image" || edge.To.Kind != "image" {
					t.Fatalf("unexpected declared image relation: %+v", edge)
				}
			}
			if graph.Edges[1].Detail != "init container prepare" {
				t.Fatal("init image declaration is indistinguishable from an ordinary container")
			}
		})
	}
}

func indentKubernetes(input, prefix string) string {
	return prefix + strings.ReplaceAll(strings.TrimSuffix(input, "\n"), "\n", "\n"+prefix) + "\n"
}

func TestKubernetesListSelectorsNamespacesAndProvenance(t *testing.T) {
	input := `---
# Empty physical document is not document one for a resource.
---
apiVersion: v1
kind: List
items:
  - apiVersion: v1
    kind: Service
    metadata: {name: api, namespace: blue}
    spec: {selector: {role: api}}
  - apiVersion: apps/v1
    kind: Deployment
    metadata: {name: web, namespace: blue}
    spec:
      replicas: 3
      template:
        metadata: {labels: {role: api}}
        spec: {containers: [{name: web, image: app:1}]}
  - apiVersion: v1
    kind: Pod
    metadata: {name: worker, namespace: blue, labels: {role: api}}
    spec: {containers: [{name: app, image: app:1}]}
  - apiVersion: v1
    kind: Pod
    metadata: {name: outside, namespace: red, labels: {role: api}}
    spec: {containers: [{name: app, image: app:1}]}
  - apiVersion: apps/v1
    kind: Deployment
    metadata: {name: wrong-template, namespace: blue, labels: {role: api}}
    spec:
      template:
        metadata: {labels: {role: not-api}}
        spec: {containers: [{name: app, image: app:1}]}
---
apiVersion: v1
kind: Service
metadata: {name: unspecified}
spec: {selector: {role: api}}
---
apiVersion: v1
kind: Pod
metadata: {name: unspecified, labels: {role: api}}
spec: {containers: [{name: app, image: app:1}]}
---
apiVersion: v1
kind: Pod
metadata: {name: explicit-default, namespace: default, labels: {role: api}}
spec: {containers: [{name: app, image: app:1}]}
`
	graph := requireKubernetes(t, input)
	if graph.Incomplete {
		t.Fatalf("selector candidates should be fully declared: %+v", graph.Diagnostics)
	}
	var candidates []Edge
	for _, edge := range graph.Edges {
		if edge.Kind == "selector-candidate" {
			candidates = append(candidates, edge)
		}
	}
	want := []struct{ from, to ID }{
		{ID{Kind: "Service", Namespace: "blue", Name: "api"}, ID{Kind: "Deployment", Namespace: "blue", Name: "web"}},
		{ID{Kind: "Service", Namespace: "blue", Name: "api"}, ID{Kind: "Pod", Namespace: "blue", Name: "worker"}},
		{ID{Kind: "Service", Name: "unspecified"}, ID{Kind: "Pod", Name: "unspecified"}},
	}
	if len(candidates) != len(want) {
		t.Fatalf("unexpected namespace or selector candidates: %+v", candidates)
	}
	for i, candidate := range candidates {
		if candidate.From != want[i].from || candidate.To != want[i].to {
			t.Fatalf("candidate mismatch: %+v", candidate)
		}
	}
	deployment := kubernetesNode(t, graph, want[0].to)
	if deployment.Origin.Document != 2 || deployment.Origin.Line == 0 || deployment.Origin.Line == graph.Sources[1].Line {
		t.Fatalf("List item lost physical document or item line: %+v", deployment.Origin)
	}
	pod := kubernetesNode(t, graph, want[2].to)
	if pod.Origin.Document != 4 || pod.Origin.File != "declared.yaml" || pod.Origin.Hash != "manifest-hash" {
		t.Fatalf("multi-document provenance lost: %+v", pod.Origin)
	}
	if len(graph.Sources) != 5 {
		t.Fatalf("physical source document positions lost: %+v", graph.Sources)
	}
	for range 5 {
		again := requireKubernetes(t, input)
		if !reflect.DeepEqual(graph, again) {
			t.Fatal("same declarations produced a nondeterministic graph")
		}
	}
}

func TestKubernetesSelectorsNeverGuessEndpoints(t *testing.T) {
	for name, selector := range map[string]string{
		"absent": "", "empty": "selector: {}", "null": "selector: null", "unmatched": "selector: {role: elsewhere}", "substitution": "selector: {role: '${ROLE}'}",
	} {
		t.Run(name, func(t *testing.T) {
			graph := requireKubernetes(t, "apiVersion: v1\nkind: Service\nmetadata: {name: api}\nspec:\n  "+selector+"\n  ports: [{port: 80}]\n---\napiVersion: v1\nkind: Pod\nmetadata: {name: api, labels: {role: api}}\nspec: {containers: [{name: app, image: app:1}]}\n")
			var unknown int
			for _, edge := range graph.Edges {
				if edge.Kind == "selector-candidate" {
					t.Fatal("missing or unresolved selector invented an endpoint candidate")
				}
				if edge.Kind == "endpoint-unknown" {
					unknown++
					if edge.To.Kind != "unknown-endpoint" {
						t.Fatalf("unknown endpoint not explicit: %+v", edge)
					}
					kubernetesNode(t, graph, edge.To)
				}
			}
			if !graph.Incomplete || unknown != 1 {
				t.Fatalf("unresolved endpoint must remain explicit and incomplete: %+v", graph)
			}
		})
	}
}

func TestKubernetesExternalNameAndDeclaredPorts(t *testing.T) {
	graph := requireKubernetes(t, `apiVersion: v1
kind: Service
metadata: {name: database}
spec:
  type: ExternalName
  externalName: database.example.invalid
  ports:
    - port: 5432
      targetPort: database
      protocol: TCP
      nodePort: 30432
    - port: 53
      protocol: UDP
    - port: 9999
      targetPort: 10000
      protocol: SCTP
`)
	service := kubernetesNode(t, graph, ID{Kind: "Service", Name: "database"})
	want := []Port{
		{Scope: "service", Published: "5432", Target: "database", Protocol: "TCP"},
		{Scope: "node", Published: "30432", Target: "database", Protocol: "TCP"},
		{Scope: "service", Published: "53", Target: "53", Protocol: "UDP"},
		{Scope: "service", Published: "9999", Target: "10000", Protocol: "SCTP"},
	}
	if len(service.Ports) != len(want) {
		t.Fatalf("declared ports lost: %+v", service.Ports)
	}
	for i, port := range service.Ports {
		if port.Origin.Document != 1 || port.Origin.Line == 0 || port.Origin.Hash != "manifest-hash" {
			t.Fatalf("port provenance lost: %+v", port.Origin)
		}
		port.Origin = Origin{}
		if port != want[i] {
			t.Fatalf("declared port mismatch: got %+v, want %+v", port, want[i])
		}
	}
	if graph.Incomplete || len(graph.Edges) != 1 || graph.Edges[0].Kind != "external-name" || graph.Edges[0].To != (ID{Kind: "external-endpoint", Name: "database.example.invalid"}) {
		t.Fatalf("ExternalName declaration misrepresented: %+v", graph)
	}
	kubernetesNode(t, graph, graph.Edges[0].To)
}

func TestKubernetesRejectsDuplicateResourceIdentityAcrossListsAndDocuments(t *testing.T) {
	pod := "apiVersion: v1\nkind: Pod\nmetadata: {name: private-name}\nspec: {containers: [{name: app, image: app:1}]}\n"
	for _, input := range []string{
		pod + "---\n" + pod,
		"apiVersion: v1\nkind: List\nitems:\n" + indentKubernetes("- "+strings.ReplaceAll(strings.TrimSuffix(pod, "\n"), "\n", "\n  ")+"\n", "  ") + "---\n" + pod,
	} {
		_, err := kubernetesFixture(input)
		if !errors.Is(err, ErrInput) || strings.Contains(err.Error(), "private-name") {
			t.Fatalf("duplicate must fail with a value-free input error: %v", err)
		}
	}
	graph := requireKubernetes(t, pod+"---\n"+strings.Replace(pod, "{name: private-name}", "{name: private-name, namespace: explicit}", 1))
	if len(graph.Nodes) != 3 {
		t.Fatalf("namespace-distinct declarations or shared image were lost: %+v", graph.Nodes)
	}
}

func TestKubernetesUnsupportedVersionsAndKindsAreNotInterpreted(t *testing.T) {
	for _, test := range []struct{ version, kind string }{
		{"extensions/v1beta1", "Deployment"}, {"apps/v1beta1", "StatefulSet"}, {"v1", "Deployment"}, {"apps/v1", "Pod"}, {"batch/v1beta1", "Job"}, {"v1", "Endpoints"}, {"discovery.k8s.io/v1", "EndpointSlice"}, {"example.invalid/v1", "PRIVATE_KIND_SENTINEL"}, {"example.invalid/v1", "List"},
	} {
		t.Run(test.version+"/"+test.kind, func(t *testing.T) {
			graph := requireKubernetes(t, fmt.Sprintf("apiVersion: %s\nkind: %s\nmetadata: {name: PRIVATE_RESOURCE_SENTINEL}\nspec: {payload: PRIVATE_PAYLOAD_SENTINEL}\n", test.version, test.kind))
			if !graph.Incomplete || len(graph.Nodes) != 0 || len(graph.Edges) != 0 || len(graph.Diagnostics) != 1 {
				t.Fatalf("unsupported API was interpreted or silently accepted: %+v", graph)
			}
			encoded, err := json.Marshal(graph)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "PRIVATE_") {
				t.Fatal("unsupported resource data escaped through graph or diagnostics")
			}
		})
	}
}

func TestKubernetesProjectionDoesNotRetainConfigurationOrSelectorValues(t *testing.T) {
	graph := requireKubernetes(t, `apiVersion: v1
kind: List
items:
  - apiVersion: v1
    kind: Secret
    metadata: {name: PRIVATE_SECRET_NAME, labels: {PRIVATE_LABEL_KEY: PRIVATE_LABEL_VALUE}}
    stringData: {password: PRIVATE_SECRET_VALUE}
    data: PRIVATE_ARBITRARY_SECRET_SHAPE
  - apiVersion: v1
    kind: ConfigMap
    metadata: {name: PRIVATE_CONFIG_NAME}
    data: {config: PRIVATE_CONFIG_VALUE}
  - apiVersion: apps/v1
    kind: Deployment
    metadata: {name: app}
    spec:
      template:
        metadata: {labels: {PRIVATE_SELECTOR_KEY: PRIVATE_SELECTOR_VALUE}}
        spec:
          imagePullSecrets: [{name: PRIVATE_IMAGE_PULL_SECRET}]
          volumes: [{name: private, secret: {secretName: PRIVATE_VOLUME_SECRET}}]
          containers:
            - name: app
              image: app:1
              command: [PRIVATE_COMMAND]
              env: [{name: PRIVATE_ENV_NAME, value: PRIVATE_ENV_VALUE}]
              envFrom: [{configMapRef: {name: PRIVATE_ENV_CONFIG}}]
  - apiVersion: v1
    kind: Service
    metadata: {name: app}
    spec: {selector: {PRIVATE_SELECTOR_KEY: PRIVATE_SELECTOR_VALUE}}
`)
	if graph.Incomplete {
		t.Fatalf("omitted configuration must not make an otherwise declared projection incomplete: %+v", graph.Diagnostics)
	}
	encoded, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "PRIVATE_") {
		t.Fatal("configuration payload, references, or transient selector data persisted in graph")
	}
	if len(graph.Nodes) != 3 || len(graph.Edges) != 2 || len(graph.Diagnostics) != 2 {
		t.Fatalf("unexpected privacy projection shape: nodes=%d edges=%d diagnostics=%d", len(graph.Nodes), len(graph.Edges), len(graph.Diagnostics))
	}
}

func TestKubernetesUnknownImageAndExternalReferencesNeverLeakOrBind(t *testing.T) {
	graph := requireKubernetes(t, `apiVersion: v1
kind: Pod
metadata: {name: app}
spec:
  containers:
    - {name: first, image: '${PRIVATE_IMAGE_SENTINEL}'}
    - {name: second, image: 'https://user:PRIVATE_PASSWORD_SENTINEL@registry.invalid/image'}
    - {name: missing}
---
apiVersion: v1
kind: Service
metadata: {name: outside}
spec: {type: ExternalName, externalName: '${PRIVATE_EXTERNAL_SENTINEL}'}
`)
	if !graph.Incomplete {
		t.Fatal("unresolved declarations did not mark graph incomplete")
	}
	var unknownImages int
	for _, node := range graph.Nodes {
		if node.ID.Kind == "image" || node.ID.Kind == "external-endpoint" {
			t.Fatalf("nonliteral declaration bound a literal artifact or endpoint: %+v", node.ID)
		}
		if node.ID.Kind == "unknown-artifact" {
			unknownImages++
		}
	}
	if unknownImages != 3 {
		t.Fatalf("unknown images must retain independent declaration identity: %d", unknownImages)
	}
	encoded, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "PRIVATE_") {
		t.Fatal("unknown reference leaked through graph or diagnostics")
	}
	for _, edge := range graph.Edges {
		kubernetesNode(t, graph, edge.From)
		kubernetesNode(t, graph, edge.To)
	}
}

func TestKubernetesMalformedSupportedFieldsProduceValueFreeErrors(t *testing.T) {
	for name, input := range map[string]string{
		"metadata":          "apiVersion: v1\nkind: Pod\nmetadata: PRIVATE_SENTINEL\n",
		"name":              "apiVersion: v1\nkind: Pod\nmetadata: {name: '${PRIVATE_SENTINEL}'}\n",
		"missing-spec":      "apiVersion: v1\nkind: Pod\nmetadata: {name: app}\n",
		"containers":        "apiVersion: v1\nkind: Pod\nmetadata: {name: app}\nspec: {containers: PRIVATE_SENTINEL}\n",
		"empty-containers":  "apiVersion: v1\nkind: Pod\nmetadata: {name: app}\nspec: {containers: []}\n",
		"replicas":          "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: app}\nspec: {replicas: PRIVATE_SENTINEL}\n",
		"negative-replicas": "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: app}\nspec: {replicas: -1}\n",
		"selector":          "apiVersion: v1\nkind: Service\nmetadata: {name: app}\nspec: {selector: {app: [PRIVATE_SENTINEL]}}\n",
		"ports":             "apiVersion: v1\nkind: Service\nmetadata: {name: app}\nspec: {ports: PRIVATE_SENTINEL}\n",
		"port-range":        "apiVersion: v1\nkind: Service\nmetadata: {name: app}\nspec: {ports: [{port: 65536}]}\n",
		"target-port":       "apiVersion: v1\nkind: Service\nmetadata: {name: app}\nspec: {ports: [{port: 80, targetPort: '${PRIVATE_SENTINEL}'}]}\n",
		"protocol":          "apiVersion: v1\nkind: Service\nmetadata: {name: app}\nspec: {ports: [{port: 80, protocol: PRIVATE_SENTINEL}]}\n",
		"list":              "apiVersion: v1\nkind: List\nitems: PRIVATE_SENTINEL\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := kubernetesFixture(input)
			if !errors.Is(err, ErrInput) {
				t.Fatalf("malformed supported field must fail as input error: %v", err)
			}
			if strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
				t.Fatal("input error exposed a raw field value")
			}
		})
	}
}

func TestKubernetesUnrenderedIdentitiesAndEqualLabelsCannotBind(t *testing.T) {
	for _, identity := range []string{
		"{name: '${PRIVATE_SENTINEL}'}",
		"{name: '{{PRIVATE_SENTINEL}}'}",
		"{name: app, namespace: '${PRIVATE_SENTINEL}'}",
		"{name: app, namespace: '{{PRIVATE_SENTINEL}}'}",
	} {
		_, err := kubernetesFixture("apiVersion: v1\nkind: Pod\nmetadata: " + identity + "\nspec: {containers: [{name: app, image: app:1}]}\n")
		if !errors.Is(err, ErrInput) || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
			t.Fatalf("unrendered identity must fail without exposing its value: %v", err)
		}
	}
	for _, label := range []string{"${PRIVATE_SENTINEL}", "{{PRIVATE_SENTINEL}}"} {
		for _, selector := range []string{"{role: '" + label + "'}", "{'" + label + "': api}"} {
			input := "apiVersion: v1\nkind: Service\nmetadata: {name: app}\nspec: {selector: " + selector + "}\n---\napiVersion: v1\nkind: Pod\nmetadata: {name: app, labels: " + selector + "}\nspec: {containers: [{name: app, image: app:1}]}\n"
			graph := requireKubernetes(t, input)
			for _, edge := range graph.Edges {
				if edge.Kind == "selector-candidate" {
					t.Fatal("equal unrendered labels created a false selector binding")
				}
			}
			if !graph.Incomplete {
				t.Fatal("unrendered selector was silently considered complete")
			}
			encoded, err := json.Marshal(graph)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "PRIVATE_SENTINEL") {
				t.Fatal("unrendered label escaped transient matching state")
			}
		}
	}
}

func TestKubernetesAbsentReplicasStayUnspecified(t *testing.T) {
	graph := requireKubernetes(t, "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: app}\nspec: {template: {spec: {containers: [{name: app, image: app:1}]}}}\n")
	node := kubernetesNode(t, graph, ID{Kind: "Deployment", Name: "app"})
	if node.Replicas != nil {
		t.Fatal("missing desired replicas were replaced by inferred instances")
	}
}
