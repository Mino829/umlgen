package cli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassTitleRendersLiteralMarkupAndPreprocessorText(t *testing.T) {
	if _, err := exec.LookPath("plantuml"); err != nil {
		t.Skip("real PlantUML command is not installed")
	}
	root := t.TempDir()
	source := filepath.Join(root, "Model.java")
	packageCLIWrite(t, source, "package sample; public class Visible { public int value; }")
	output := filepath.Join(root, "diagram.svg")
	title := "Review @enduml } [[link]] <b>literal</b> %date()"
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", source, "--title", title, "--format", "svg", "--renderer", "local", "--no-cache", "--output", output}, &stdout, &stderr)
	if err != nil || code != exitOK {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	labels := readSVGLabels(t, output)
	text := strings.Join(labels, "\n")
	for _, literal := range []string{title, "Visible", "value: int"} {
		if !strings.Contains(text, literal) {
			t.Errorf("SVG did not retain literal text %q; labels: %q", literal, labels)
		}
	}
}
