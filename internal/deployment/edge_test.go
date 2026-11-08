package deployment

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCredentialLikeDigestAuthorityIsNeverRetained(t *testing.T) {
	const secret = "PRIVATE_PASSWORD_DO_NOT_PRINT"
	path := deploymentInput(t, "compose.yaml", "services:\n  api:\n    image: 'user:"+secret+"@sha256:5000/team/image:1'\n")
	graph, err := LoadManifest(path, "auto")
	if err != nil {
		t.Fatal(err)
	}
	captured, _ := json.Marshal(graph)
	if strings.Contains(string(captured), secret) {
		t.Fatal("credential-like image reference retained in graph")
	}
	if !graph.Incomplete {
		t.Fatal("unsupported image was treated as a resolved literal artifact")
	}
}

func TestUnresolvedInheritanceCannotInventDefaultNetwork(t *testing.T) {
	path := deploymentInput(t, "compose.yaml", "services:\n  base:\n    image: app:1\n    networks: [back]\n  api:\n    image: app:1\n    extends: {service: base}\nnetworks:\n  back: {}\n")
	graph, err := LoadManifest(path, "auto")
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range graph.Edges {
		if edge.From == (ID{Kind: "compose-service", Name: "api"}) && edge.Kind == "network-member" {
			t.Fatalf("unresolved inheritance invented network membership: %+v", edge)
		}
	}
	if !graph.Incomplete {
		t.Fatal("unevaluated inheritance not diagnosed as incomplete")
	}
}

func TestComposeEmptyDocumentsKeepPhysicalProvenance(t *testing.T) {
	path := deploymentInput(t, "compose.yaml", "---\n---\nservices:\n  api: {image: app:1}\n")
	graph, err := LoadManifest(path, "auto")
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range graph.Nodes {
		if node.ID.Kind == "compose-service" && node.Origin.Document != 2 {
			t.Fatalf("Compose origin uses nonphysical document ordinal: %+v", node.Origin)
		}
	}
}

func TestEmptyNetworkDeclarationCannotEraseUnresolvedInheritance(t *testing.T) {
	for _, value := range []string{"null", "[]", "{}"} {
		path := deploymentInput(t, "compose.yaml", "services:\n  base: {image: app:1, networks: [back]}\n  api:\n    image: app:1\n    extends: {service: base}\n    networks: "+value+"\nnetworks: {back: {}}\n")
		graph, err := LoadManifest(path, "auto")
		if err != nil {
			t.Fatal(err)
		}
		for _, edge := range graph.Edges {
			if edge.From == (ID{Kind: "compose-service", Name: "api"}) && edge.Kind == "network-member" {
				t.Fatalf("empty networks %s inferred membership under inheritance: %+v", value, edge)
			}
		}
	}
}
