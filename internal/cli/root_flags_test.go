package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGlobalConfigValueCanBeACommand(t *testing.T) {
	const helper = "UMLGEN_ROOT_CONFIG_COMMAND_HELPER"
	if os.Getenv(helper) == "1" {
		at := slices.Index(os.Args, "--")
		if at < 0 {
			t.Fatal("missing CLI arguments")
		}
		var stdout, stderr bytes.Buffer
		code, err := Run(os.Args[at+1:], &stdout, &stderr)
		fmt.Print(stdout.String())
		if code != exitOK || err != nil {
			t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
		}
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--config", "class", "class", "src"},
		{"-config", "package", "class", "src"},
		{"--config=version", "class", "src"},
		{"--quiet=false", "--config", "diff", "class", "src"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := t.TempDir()
			packageCLIWrite(t, filepath.Join(root, "src", "C.java"), "package app; public class C {}")
			for _, name := range []string{"class", "package", "version", "diff"} {
				packageCLIWrite(t, filepath.Join(root, name), "language: java\noutput:\n  file: diagram.puml\n")
			}
			command := exec.Command(executable, append([]string{"-test.run=^TestGlobalConfigValueCanBeACommand$", "--"}, args...)...)
			command.Dir = root
			command.Env = append(os.Environ(), helper+"=1")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("CLI subprocess failed: %v\n%s", err, output)
			}
			if output := packageCLIRead(t, filepath.Join(root, "diagram.puml")); !strings.Contains(output, `class "C"`) {
				t.Fatalf("global config value prevented class generation:\n%s", output)
			}
		})
	}
}

func TestGlobalFlagsRejectMissingValuesAndUnknownCommand(t *testing.T) {
	for _, args := range [][]string{
		{"--config"}, {"--verbose"}, {"unknown", "class"}, {"--unknown", "class"}, {"--", "--help"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code, err := Run(args, &stdout, &stderr)
			if code != exitArgs || err == nil {
				t.Fatalf("expected argument failure, got code=%d err=%v stdout=%s", code, err, stdout.String())
			}
		})
	}
}
