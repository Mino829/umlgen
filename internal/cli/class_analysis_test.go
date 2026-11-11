package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassAnalysisFocusContextAndCacheParity(t *testing.T) {
	for _, language := range []string{"java", "go"} {
		t.Run(language, func(t *testing.T) {
			root := t.TempDir()
			source := `package sample;
class Root { Target next; Unknown missing; }
class Target {}
class Consumer { void use(Root value) {} }
`
			name, qualified := "Types.java", "sample"
			if language == "go" {
				name, qualified = "types.go", "example.test/focus"
				source = `package sample
type Root struct { Next Target; Missing Unknown }
type Target struct{}
type Consumer struct{}
func (Consumer) Use(value Root) {}
`
				packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/focus\n")
			}
			packageCLIWrite(t, filepath.Join(root, name), source)
			t.Setenv("UMLGEN_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
			run := func(mode string, extra ...string) (string, []byte) {
				t.Helper()
				output := filepath.Join(t.TempDir(), "diagram.puml")
				args := []string{"class", root, "--language", language, "--quiet", "--explain", "--focus", "Root", "--output", output}
				args = append(args, extra...)
				if mode == "disabled" {
					args = append(args, "--no-cache")
				}
				var stdout, stderr bytes.Buffer
				code, err := Run(args, &stdout, &stderr)
				if err != nil || code != exitOK {
					t.Fatalf("%s: code=%d err=%v stderr=%s", mode, code, err, stderr.String())
				}
				content, err := os.ReadFile(output)
				if err != nil {
					t.Fatal(err)
				}
				return stdout.String(), content
			}
			var wantExplanation string
			var wantDiagram []byte
			for _, mode := range []string{"cold", "warm", "disabled"} {
				explanation, diagram := run(mode, "--depth", "0", "--relations", "field")
				for _, want := range []string{
					`[field, outside-diagram]: reference="Target", target=` + qualified + ".Target",
					`[field, unresolved]: reference="Unknown"`,
				} {
					if !strings.Contains(explanation, want) {
						t.Errorf("%s lost hidden/unknown distinction %q:\n%s", mode, want, explanation)
					}
				}
				if strings.Contains(string(diagram), `class "Target"`) || strings.Contains(string(diagram), " --> ") {
					t.Fatalf("%s drew a hidden target:\n%s", mode, diagram)
				}
				if mode == "cold" {
					wantExplanation, wantDiagram = explanation, diagram
				} else if explanation != wantExplanation || !bytes.Equal(diagram, wantDiagram) {
					t.Fatalf("%s changed the shared analysis result", mode)
				}
			}
			// Rendering only fields (or no relations) must not remove a parameter
			// edge from the class focus traversal policy.
			for _, flags := range [][]string{{"--relations", "field"}, {"--no-relations"}} {
				extra := append([]string{"--direction", "in", "--depth", "1", "--hide-methods", "--hide-fields"}, flags...)
				explanation, diagram := run("warm", extra...)
				if !strings.Contains(string(diagram), `class "Consumer"`) || strings.Contains(string(diagram), " ..> ") {
					t.Fatalf("display filters changed focus traversal:\n%s", diagram)
				}
				if flags[0] == "--no-relations" && (strings.Contains(explanation, " -> ") || strings.Contains(explanation, "reference=")) {
					t.Fatalf("hidden relation details leaked:\n%s", explanation)
				}
			}
		})
	}
}
