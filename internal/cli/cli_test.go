package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassAcceptsOptionsAfterTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "User.java"), []byte(
		"package sample; public class User { private String name; public String getName(){ return name; } }",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "docs", "diagram.puml")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", src, "--output", out, "--hide-private"}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "-name: String") {
		t.Fatalf("private field was not hidden:\n%s", data)
	}
}

func TestDirectionAndRelationLabels(t *testing.T) {
	dir := t.TempDir()
	for name, source := range map[string]string{
		"Controller.java": `package sample; class Controller { Service service; }`,
		"Service.java":    `package sample; class Service { java.util.List<Repository> repositories; }`,
		"Repository.java": `package sample; class Repository {}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "outgoing.puml")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{
		"class", dir, "--focus", "Service", "--direction", "out",
		"--relations", "field", "--show-relation-labels", "-o", out,
	}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, `"Controller"`) {
		t.Fatalf("incoming type should be excluded:\n%s", text)
	}
	if !strings.Contains(text, `--> "*" T_sample_Repository : field repositories`) {
		t.Fatalf("missing labeled multiplicity:\n%s", text)
	}
}

func TestDiffCommand(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "umlgen@example.test")
	runGit(t, root, "config", "user.name", "umlgen test")
	writeJava(t, root, "Repository.java", `package sample; class Repository {}`)
	writeJava(t, root, "Service.java", `package sample; class Service { Repository repository; }`)
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	writeJava(t, root, "Service.java", `package sample; class Service { Repository repository; int revision; }`)

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)

	out := filepath.Join(root, "changes.puml")
	var stdout, stderr bytes.Buffer
	code, runErr := Run([]string{"diff", "HEAD", "--depth", "1", "-o", out}, &stdout, &stderr)
	if runErr != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, runErr, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `class "Service" as T_sample_Service #lightyellow`) {
		t.Fatalf("modified class was not colored:\n%s", text)
	}
	if !strings.Contains(text, `class "Repository"`) {
		t.Fatalf("related class was not included:\n%s", text)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func writeJava(t *testing.T, root, name, source string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", ".", "--format", "pdf"}, &stdout, &stderr)
	if code != exitArgs || err == nil || !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestVerboseQuietConflict(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", ".", "--verbose", "--quiet"}, &stdout, &stderr)
	if code != exitArgs || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestInvalidDirectionAndRelationKind(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", ".", "--focus", "User", "--direction", "sideways"}, &stdout, &stderr)
	if code != exitArgs || err == nil || !strings.Contains(err.Error(), "unsupported direction") {
		t.Fatalf("direction: code=%d err=%v", code, err)
	}
	code, err = Run([]string{"class", ".", "--relations", "calls"}, &stdout, &stderr)
	if code != exitArgs || err == nil || !strings.Contains(err.Error(), "unsupported relation kind") {
		t.Fatalf("relations: code=%d err=%v", code, err)
	}
}

func TestFocusAndDepth(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Controller.java": `package sample; class Controller { Service service; }`,
		"Service.java":    `package sample; class Service { Repository repository; }`,
		"Repository.java": `package sample; class Repository { User find(){ return null; } }`,
		"User.java":       `package sample; class User {}`,
		"Other.java":      `package sample; class Other {}`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "focus.puml")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", src, "--focus", "Service", "--depth", "1", "-o", out}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"Controller"`, `"Service"`, `"Repository"`} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{`"User"`, `"Other"`} {
		if strings.Contains(text, unwanted) {
			t.Errorf("unexpected %s:\n%s", unwanted, text)
		}
	}
}
