package cli

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The Linux demo CI job installs PlantUML and runs this test explicitly. Local
// unit-only environments may omit the renderer; no fake image can prove that
// structural Go types are valid PlantUML member text.
func TestGoStructuralMemberTypesRenderSVG(t *testing.T) {
	if _, err := exec.LookPath("plantuml"); err != nil {
		t.Skip("real PlantUML command is not installed")
	}
	for _, grouping := range []string{"flat", "hierarchical", "none"} {
		t.Run(grouping, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "diagram.svg")
			var stdout, stderr bytes.Buffer
			code, err := Run([]string{
				"class", filepath.Join("..", "..", "testdata", "go", "anonymousstruct"),
				"--language", "go", "--format", "svg", "--renderer", "local", "--no-cache",
				"--package-grouping", grouping, "--output", output,
			}, &stdout, &stderr)
			if err != nil || code != 0 {
				t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
			}
			labels := readSVGLabels(t, output)
			for member, contents := range map[string][]string{
				"Location:":     {"*struct", "Latitude", "Longitude", `json:"latitude"`},
				"CheckpointID:": {"string"},
				"Nested:":       {"map[string][]struct", "Point", "X, Y int"},
				"Handler:":      {"interface", "Handle(value struct", "Name string"},
				"Empty:":        {"struct{}"},
				"Transform(":    {"input: struct", "Value string", "Result string"},
				"Accept(":       {"next: string"},
			} {
				var matched string
				for _, label := range labels {
					if strings.Contains(label, member) {
						matched = label
						break
					}
				}
				if matched == "" {
					t.Errorf("rendered SVG lost member %q; labels: %q", member, labels)
					continue
				}
				for _, want := range contents {
					if !strings.Contains(matched, want) {
						t.Errorf("rendered member %q lost %q", matched, want)
					}
				}
			}
			for _, label := range labels {
				if strings.Contains(label, "Syntax Error") {
					t.Errorf("renderer produced an error diagram: %s", label)
				}
			}
		})
	}
}

func TestDefaultPackageClassesRenderSVG(t *testing.T) {
	if _, err := exec.LookPath("plantuml"); err != nil {
		t.Skip("real PlantUML command is not installed")
	}
	for _, mixed := range []bool{false, true} {
		name := "default only"
		if mixed {
			name = "mixed packages"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "src")
			packageCLIWrite(t, filepath.Join(source, "Default.java"), "public class Default { public int value; }")
			if mixed {
				packageCLIWrite(t, filepath.Join(source, "named", "Named.java"), "package named; public class Named {}")
				// A default-package name sorted after a named package exercises
				// closing the named wrapper before writing another root type.
				packageCLIWrite(t, filepath.Join(source, "zDefault.java"), "class zDefault {}")
			}
			output := filepath.Join(root, "diagram.svg")
			var stdout, stderr bytes.Buffer
			code, err := Run([]string{"class", source, "--language", "java", "--format", "svg",
				"--renderer", "local", "--package-grouping", "flat", "--no-cache", "--output", output,
			}, &stdout, &stderr)
			if err != nil || code != exitOK {
				t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
			}
			labels := readSVGLabels(t, output)
			text := strings.Join(labels, "\n")
			for _, want := range []string{"Default", "value: int"} {
				if !strings.Contains(text, want) {
					t.Errorf("default-package class lost %q; labels: %q", want, labels)
				}
			}
			if mixed && (!strings.Contains(text, "Named") || !strings.Contains(text, "zDefault")) {
				t.Errorf("mixed package grouping lost types: %q", labels)
			}
		})
	}
}

func readSVGLabels(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var labels []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return labels
		}
		if err != nil {
			t.Fatal(err)
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "text" {
			var label string
			if err := decoder.DecodeElement(&label, &start); err != nil {
				t.Fatal(err)
			}
			labels = append(labels, label)
		}
	}
}
