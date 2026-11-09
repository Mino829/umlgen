package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeploymentDeclaredTopologyWithoutTools(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("PATH", "")
	packageCLIWrite(t, "compose.yaml", "services:\n  api:\n    image: example/api:1\n    environment:\n      PASSWORD: PRIVATE_SENTINEL\n    env_file: missing.env\n")
	result := packageCLISuccess(t, "deployment", "--manifest", "compose.yaml", "--explain")
	diagram := packageCLIRead(t, "deployment-diagram.puml")
	if strings.Contains(diagram+result.stdout+result.stderr, "PRIVATE_SENTINEL") {
		t.Fatal("ignored environment value disclosed")
	}
}

func TestDeploymentInvalidInputPreservesDiagram(t *testing.T) {
	cases := []struct {
		name, manifest string
		extra          []string
	}{
		{"malformed", "services: [PRIVATE_SENTINEL", nil},
		{"wrong service shape", "services:\n  api: PRIVATE_SENTINEL\n", nil},
		{"no supported nodes", "apiVersion: v1\nkind: Secret\nmetadata:\n  name: hidden\ndata:\n  token: PRIVATE_SENTINEL\n", nil},
		{"positional target", "services:\n  api:\n    image: api:1\n", []string{"."}},
		{"source flag", "services:\n  api:\n    image: api:1\n", []string{"--language", "go"}},
		{"mapping without boundary", "services:\n  api:\n    image: api:1\n", []string{"--mapping-config", "missing.yaml"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			manifest, output := filepath.Join(root, "manifest.yaml"), filepath.Join(root, "diagram.puml")
			packageCLIWrite(t, manifest, tt.manifest)
			packageCLIWrite(t, output, "previous diagram")
			args := append([]string{"deployment", "--manifest", manifest, "--output", output}, tt.extra...)
			result := packageCLIRun(args...)
			if result.code != exitArgs || result.err == nil {
				t.Fatalf("code=%d error=%v", result.code, result.err)
			}
			if got := packageCLIRead(t, output); got != "previous diagram" {
				t.Fatalf("previous output replaced: %s", got)
			}
			if strings.Contains(fmt.Sprint(result.err)+result.stdout+result.stderr, "PRIVATE_SENTINEL") {
				t.Fatal("invalid input disclosed")
			}
		})
	}
}

func TestDeploymentIncompleteWritesDiagram(t *testing.T) {
	root := t.TempDir()
	manifest, output := filepath.Join(root, "compose.yaml"), filepath.Join(root, "diagram.puml")
	packageCLIWrite(t, manifest, "services:\n  api:\n    build: .\n")
	packageCLIWrite(t, output, "previous diagram")
	result := packageCLIRun("deployment", "--manifest", manifest, "--output", output, "--explain")
	if result.code != exitParse || result.err == nil {
		t.Fatalf("code=%d error=%v", result.code, result.err)
	}
	if got := packageCLIRead(t, output); got == "previous diagram" || !strings.Contains(got, "api") {
		t.Fatalf("missing partial topology: %s", got)
	}
}

func TestDeploymentOverrideReplacesInvalidConfigRelativeManifest(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	packageCLIWrite(t, filepath.Join(root, "settings", "declared.yaml"), "services: [\n")
	packageCLIWrite(t, "override.yaml", "services:\n  overridden:\n    image: overridden:1\n")
	cfg := packageCLIConfig(t, filepath.Join(root, "settings"), "diagrams:\n  deployment:\n    manifest: declared.yaml\noutput:\n  file: configured.puml\n")
	original := filepath.Join(root, "settings", "configured.puml")
	packageCLIWrite(t, original, "previous valid topology")
	rejected := packageCLIRun("deployment", "--config", cfg)
	if rejected.code != exitArgs || packageCLIRead(t, original) != "previous valid topology" {
		t.Fatalf("config-relative invalid manifest was not rejected safely: %+v", rejected)
	}
	overridden := packageCLIRun("deployment", "--config", cfg, "--manifest", "override.yaml", "--output", "override.puml")
	if overridden.code != exitOK || overridden.err != nil || packageCLIRead(t, original) != "previous valid topology" {
		t.Fatalf("cwd-relative CLI override did not replace invalid configured input: %+v", overridden)
	}
}

func TestDeploymentIOAndOutputFailures(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "compose.yaml")
	output := filepath.Join(root, "diagram.puml")
	packageCLIWrite(t, output, "previous diagram")
	missing := packageCLIRun("deployment", "--manifest", filepath.Join(root, "missing.yaml"), "--output", output)
	if missing.code != exitError || missing.err == nil {
		t.Fatalf("missing manifest: code=%d error=%v", missing.code, missing.err)
	}
	if got := packageCLIRead(t, output); got != "previous diagram" {
		t.Fatalf("I/O failure replaced previous output: %s", got)
	}
	packageCLIWrite(t, manifest, "services:\n  api:\n    image: api:1\n")
	packageCLIWrite(t, filepath.Join(root, "blocked"), "not a directory")
	blocked := packageCLIRun("deployment", "--manifest", manifest, "--output", filepath.Join(root, "blocked", "diagram.puml"))
	if blocked.code != exitOutput || blocked.err == nil {
		t.Fatalf("output failure: code=%d error=%v", blocked.code, blocked.err)
	}
}
