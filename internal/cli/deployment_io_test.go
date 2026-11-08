package cli

import (
	"path/filepath"
	"testing"
)

func TestDeploymentBoundaryReadFailureIsIOAndPreservesOutput(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "compose.yaml")
	output := filepath.Join(root, "diagram.puml")
	packageCLIWrite(t, manifest, "services:\n  api: {image: api:1}\n")
	packageCLIWrite(t, output, "previous valid topology")
	result := packageCLIRun("deployment", "--manifest", manifest, "--boundary-config", filepath.Join(root, "missing.yaml"), "-o", output)
	if result.code != exitError || result.err == nil || packageCLIRead(t, output) != "previous valid topology" {
		t.Fatalf("boundary read failure was not classified/preserved: %+v", result)
	}
}
