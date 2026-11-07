package deployment

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func deploymentInput(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadManifestRejectsUnsafeYAMLWithoutQuotingValues(t *testing.T) {
	sentinel := "TOP_SECRET_NEVER_PRINT"
	for _, input := range []string{
		"services:\n  api:\n    image: [\"" + sentinel + "\"\n",
		"services:\n  api: &a {image: app:v1}\n  worker: *a\n",
		"services:\n  api: {image: app:v1, image: " + sentinel + "}\n",
		"services:\n  api: {image: !execute " + sentinel + "}\n",
	} {
		path := deploymentInput(t, "compose.yaml", input)
		_, err := LoadManifest(path, "auto")
		if !errors.Is(err, ErrInput) || strings.Contains(err.Error(), sentinel) {
			t.Fatalf("unsafe YAML accepted or error disclosed input: %v", err)
		}
	}
}

func TestKubernetesEmptyDocumentsKeepPhysicalProvenance(t *testing.T) {
	path := deploymentInput(t, "cluster.yaml", "---\n---\napiVersion: v1\nkind: Pod\nmetadata: {name: api}\nspec:\n  containers: [{name: api, image: app:v1}]\n")
	graph, err := LoadManifest(path, "auto")
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range graph.Nodes {
		if node.ID.Kind == "Pod" {
			if node.Origin.Document != 2 || node.Origin.Line < 3 {
				t.Fatalf("physical document identity lost: %+v", node.Origin)
			}
			return
		}
	}
	t.Fatal("Pod omitted from supported topology")
}

func TestExplicitBindingsDoNotInferNamesOrMutateCapturedGraph(t *testing.T) {
	manifest := deploymentInput(t, "compose.yaml", "services:\n  api: {image: app:v1}\n  other: {image: app:v1}\n")
	source, err := LoadManifest(manifest, "auto")
	if err != nil {
		t.Fatal(err)
	}
	boundary := deploymentInput(t, "architecture.yaml", "version: 1\ncomponents:\n  - name: api\n    match: [{language: java, package_prefix: unrelated}]\n  - name: Worker\n    match: [{language: java, package_prefix: other}]\n")
	before, _ := json.Marshal(source)
	noBindings, err := BindComponents(source, boundary, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range noBindings.Edges {
		if edge.Kind == "component-mapping" {
			t.Fatal("equal names inferred a component/workload binding")
		}
	}
	mapping := deploymentInput(t, "mapping.yaml", "version: 1\nbindings:\n  - component: Worker\n    target: {kind: compose-service, name: api}\n  - component: api\n    target: {kind: compose-service, name: api}\n")
	graph, err := BindComponents(source, boundary, mapping)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, edge := range graph.Edges {
		if edge.Kind != "component-mapping" {
			continue
		}
		count++
		if edge.To != (ID{Kind: "compose-service", Name: "api"}) || edge.Origin.Hash == "" || edge.Origin.Line == 0 {
			t.Fatalf("explicit target or mapping provenance lost: %+v", edge)
		}
	}
	if count != 2 {
		t.Fatalf("explicit many-to-many bindings lost: %+v", graph.Edges)
	}
	after, _ := json.Marshal(source)
	if string(before) != string(after) {
		t.Fatal("binding projection mutated captured manifest graph")
	}
	for _, binding := range []string{
		"component: Missing\n    target: {kind: compose-service, name: api}",
		"component: api\n    target: {kind: compose-service, name: missing}",
		"component: api\n    target: {kind: compose-service, namespace: wrong, name: api}",
		"component: api\n    target: {kind: network, name: default}",
	} {
		bad := deploymentInput(t, "bad.yaml", "version: 1\nbindings:\n  - "+binding+"\n")
		if _, err := BindComponents(source, boundary, bad); !errors.Is(err, ErrInput) {
			t.Fatalf("invented mapping accepted: %v", err)
		}
	}
	duplicate := deploymentInput(t, "duplicate.yaml", "version: 1\nbindings:\n  - component: api\n    target: {kind: compose-service, name: api}\n  - component: api\n    target: {kind: compose-service, name: api}\n")
	if _, err := BindComponents(source, boundary, duplicate); !errors.Is(err, ErrInput) {
		t.Fatalf("duplicate binding accepted: %v", err)
	}
}
