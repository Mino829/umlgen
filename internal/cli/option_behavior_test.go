package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompactShortFlagsGenerateSelectedDiagram(t *testing.T) {
	for _, form := range []string{"attached", "cluster", "cluster next value", "single-dash long", "opaque value", "root cluster"} {
		t.Run(form, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "Model.java")
			packageCLIWrite(t, source, "package sample; public class Model {}")
			output := filepath.Join(root, "diagram.puml")
			args := []string{"class", source}
			switch form {
			case "attached":
				args = append(args, "-fplantuml", "-o"+output)
			case "cluster":
				args = append(args, "-vo"+output, "-fplantuml")
			case "cluster next value":
				args = append(args, "-vvo", output)
			case "single-dash long":
				args = append(args, "-verbose=false", "-output", output, "-format=plantuml")
			case "opaque value":
				args = append(args, "--title", "-vq", "-o"+output)
			case "root cluster":
				args = append([]string{"-vv"}, args...)
				args = append(args, "-o"+output)
			}
			var stdout, stderr bytes.Buffer
			code, err := Run(args, &stdout, &stderr)
			if code != exitOK || err != nil {
				t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
			}
			if diagram := packageCLIRead(t, output); !strings.Contains(diagram, `class "Model"`) {
				t.Fatalf("compact flags lost selected class:\n%s", diagram)
			}
		})
	}
}

func TestCompactShortFlagsDoNotHideConflictsOrUnknownOptions(t *testing.T) {
	for _, args := range [][]string{
		{"-vq", "version"}, {"class", ".", "-vq"}, {"-vX", "version"}, {"class", ".", "-vX"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code, err := Run(args, &stdout, &stderr)
			if code != exitArgs || err == nil {
				t.Fatalf("invalid compact flags accepted: code=%d err=%v stdout=%s", code, err, stdout.String())
			}
			if strings.Contains(strings.Join(args, " "), "-vq") &&
				(!strings.Contains(err.Error(), "--verbose") || !strings.Contains(err.Error(), "--quiet")) {
				t.Fatalf("cluster did not report conflicting flags: %v", err)
			}
		})
	}
}

func TestCompactFlagsRespectTerminatorAndFlagLookingValue(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	packageCLIWrite(t, "-vo.java", "package sample; public class Opaque {}")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", "-o-vq", "--", "-vo.java"}, &stdout, &stderr)
	if code != exitOK || err != nil {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	if diagram := packageCLIRead(t, "-vq.puml"); !strings.Contains(diagram, `class "Opaque"`) {
		t.Fatalf("flag-looking target/value was reinterpreted:\n%s", diagram)
	}
}

func TestDottedPathExcludesApplyBeforeClassParsing(t *testing.T) {
	root := t.TempDir()
	packageCLIWrite(t, filepath.Join(root, "src", "User.java"), "package app; class User {}")
	packageCLIWrite(t, filepath.Join(root, "src", "SuperUser.java"), "package app; class SuperUser {}")
	packageCLIWrite(t, filepath.Join(root, "src", ".generated", "Hidden.java"), "not valid Java [")
	output := filepath.Join(root, "diagram.puml")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", filepath.Join(root, "src"), "--exclude", "User.java", "--exclude", ".generated", "--no-cache", "-o", output}, &stdout, &stderr)
	if code != exitOK || err != nil || strings.Contains(stderr.String(), "failed to parse") {
		t.Fatalf("excluded source was parsed: code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	diagram := packageCLIRead(t, output)
	if strings.Contains(diagram, `class "User"`) || !strings.Contains(diagram, `class "SuperUser"`) {
		t.Fatalf("dotted filename exclusion did not preserve component boundaries:\n%s", diagram)
	}
}

func TestNegativeRenderTimeoutPreservesPreviousOutputs(t *testing.T) {
	for _, format := range []string{"plantuml", "svg"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "Model.java")
			packageCLIWrite(t, source, "package sample; class Model {}")
			output := filepath.Join(root, "diagram.puml")
			image := filepath.Join(root, "diagram.svg")
			packageCLIWrite(t, output, "previous diagram")
			packageCLIWrite(t, image, "previous image")
			var stdout, stderr bytes.Buffer
			code, err := Run([]string{"class", source, "--render-timeout=-1s", "--format", format, "--output", output}, &stdout, &stderr)
			if code != exitArgs || err == nil {
				t.Fatalf("negative timeout accepted: code=%d err=%v stderr=%s", code, err, stderr.String())
			}
			for path, want := range map[string]string{output: "previous diagram", image: "previous image"} {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != want {
					t.Errorf("negative timeout replaced %s: data=%q err=%v", path, data, err)
				}
			}
		})
	}
}
