package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/javacache"
)

func TestMain(m *testing.M) {
	cacheDir, err := os.MkdirTemp("", "umlgen-cli-test-cache-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("UMLGEN_CACHE_DIR", cacheDir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(cacheDir)
	os.Exit(code)
}

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

func TestDiffCommandNoChangedTypes(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "umlgen@example.test")
	runGit(t, root, "config", "user.name", "umlgen test")
	writeJava(t, root, "package-info.java", `package sample;`)
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	writeJava(t, root, "package-info.java", `package sample;
/** module docs */`)

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
	code, runErr := Run([]string{"diff", "HEAD", "-o", out}, &stdout, &stderr)
	if runErr != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, runErr, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "No displayable changed types in the selected range") {
		t.Fatalf("missing placeholder note:\n%s", text)
	}
	if !strings.Contains(stdout.String(), "No displayable changed types") {
		t.Fatalf("missing stdout notice:\n%s", stdout.String())
	}
}

func TestDiffCommandNoChangedTypesAfterDeletion(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "umlgen@example.test")
	runGit(t, root, "config", "user.name", "umlgen test")
	writeJava(t, root, "package-info.java", `package sample;`)
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	if err := os.Remove(filepath.Join(root, "package-info.java")); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "remove package-info")

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
	code, runErr := Run([]string{"diff", "HEAD~1", "-o", out}, &stdout, &stderr)
	if runErr != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, runErr, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "No displayable changed types in the selected range") {
		t.Fatalf("missing placeholder note:\n%s", text)
	}
}

func TestDiffCommandFailsWhenChangedFileCannotBeParsed(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "umlgen@example.test")
	runGit(t, root, "config", "user.name", "umlgen test")
	writeJava(t, root, "Valid.java", `package sample; class Valid {}`)
	writeJava(t, root, "Broken.java", `package sample; class Broken {}`)
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	writeJava(t, root, "Broken.java", `package sample; class Broken {`)

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
	code, runErr := Run([]string{"diff", "HEAD", "-o", out}, &stdout, &stderr)
	if code != exitParse || runErr == nil || !strings.Contains(runErr.Error(), "failed to parse changed Java files") {
		t.Fatalf("code=%d err=%v stderr=%s", code, runErr, stderr.String())
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("diagram should not be generated after a parse failure, stat err=%v", err)
	}
}

func TestPlaceholderDiagramSanitizesTitle(t *testing.T) {
	diagram := placeholderDiagram("first\nsecond\rthird")
	prefix := "@startuml\ntitle "
	if !strings.HasPrefix(diagram, prefix) {
		t.Fatalf("unexpected output: %q", diagram)
	}
	rest := diagram[len(prefix):]
	if strings.ContainsAny(rest[:strings.Index(rest, "\n")], "\n\r") {
		t.Fatalf("title should not contain newlines, got: %q", rest[:strings.Index(rest, "\n")])
	}
	if !strings.Contains(diagram, "first second third") {
		t.Fatalf("newlines should be replaced with spaces, got: %q", diagram)
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

func TestPNGFormatWithMockRenderer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock shell script is not executable on Windows")
	}
	dir := t.TempDir()
	writeJava(t, dir, "User.java", `package sample; class User {}`)

	binDir := t.TempDir()
	script := filepath.Join(binDir, "plantuml")
	pngPath := filepath.Join(dir, "class-diagram.png")
	if err := os.WriteFile(script, []byte(fmt.Sprintf("#!/bin/sh\n/usr/bin/touch %q", pngPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(script); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("mock script is not executable: %v", err)
	}
	t.Setenv("PATH", binDir)
	if _, err := exec.LookPath("plantuml"); err != nil {
		t.Fatalf("mock plantuml not found in PATH: %v", err)
	}

	out := filepath.Join(dir, "class-diagram.puml")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", dir, "--output", out, "--format", "png"}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("puml file was not kept: %v", err)
	}
	if _, err := os.Stat(pngPath); err != nil {
		t.Fatalf("png file was not generated: %v", err)
	}
}

func TestPNGFormatRendererNotFound(t *testing.T) {
	dir := t.TempDir()
	writeJava(t, dir, "User.java", `package sample; class User {}`)
	t.Setenv("PATH", "")
	t.Setenv("PLANTUML_JAR", "")

	out := filepath.Join(dir, "class-diagram.puml")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", dir, "--output", out, "--format", "png"}, &stdout, &stderr)
	if code != exitRender || err != nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("puml file should be kept even when rendering fails: %v", err)
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

func TestCompatibilityFixtureGoldenDiagram(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "java", "compatibility")
	out := filepath.Join(t.TempDir(), "class-diagram.puml")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{
		"class", filepath.Join(root, "project"),
		"--hide-fields", "--hide-methods", "--show-relation-labels",
		"--output", out,
	}, &stdout, &stderr)
	if err != nil || code != exitOK {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Found 8 Java files") ||
		!strings.Contains(stdout.String(), "Detected 6 classes and 3 interfaces") {
		t.Fatalf("stdout = %s", stdout.String())
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(root, "expected", "class-diagram.puml"))
	if err != nil {
		t.Fatal(err)
	}
	wantText := strings.ReplaceAll(string(want), "\r\n", "\n")
	if string(got) != wantText {
		t.Fatalf("diagram does not match golden file\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestGoCompatibilityFixture(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "go", "compatibility")
	out := filepath.Join(t.TempDir(), "go-class-diagram.puml")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{
		"class", root, "--language", "auto", "--no-cache",
		"--show-relation-labels", "--output", out,
	}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`package "example.com/shop/domain"`,
		`class "User" as T_example_com_shop_domain_User <<struct>>`,
		`interface "Repository"`,
		`T_example_com_shop_service_BaseService <|-- T_example_com_shop_service_UserService`,
		`T_example_com_shop_service_Repository <|.. T_example_com_shop_service_MemoryRepository`,
		`T_example_com_shop_service_UserService --> "1" T_example_com_shop_service_Repository`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output does not contain %q:\n%s", want, text)
		}
	}
	if !strings.Contains(stdout.String(), "Found 3 Go files") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestGoDiffCommand(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "umlgen@example.test")
	runGit(t, root, "config", "user.name", "umlgen test")
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/app\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeGo(t, root, "service.go", "package app\ntype Service struct{}\n")
	writeGo(t, root, "service_methods.go", "package app\nfunc (s *Service) Revision() int { return 1 }\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	writeGo(t, root, "service_methods.go", "package app\nfunc (s *Service) Revision() int { return 2 }\n")

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
	code, runErr := Run([]string{"diff", "HEAD", "--language", "go", "-o", out}, &stdout, &stderr)
	if runErr != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, runErr, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `class "Service" as T_example_test_app_Service <<struct>> #lightyellow`) {
		t.Fatalf("modified Go struct was not colored:\n%s", data)
	}
}

func writeGo(t *testing.T, root, name, source string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyntaxErrorsWarnOrFailConsistently(t *testing.T) {
	broken, err := os.ReadFile(filepath.Join(
		"..", "..", "testdata", "java", "compatibility", "broken", "Broken.java",
	))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("continues when another file is valid", func(t *testing.T) {
		root := t.TempDir()
		writeJava(t, root, "Valid.java", "package sample; public class Valid {}")
		writeJava(t, root, "Broken.java", string(broken))
		out := filepath.Join(root, "diagram.puml")
		var stdout, stderr bytes.Buffer
		code, runErr := Run([]string{"class", root, "--output", out}, &stdout, &stderr)
		if runErr != nil || code != exitOK {
			t.Fatalf("code=%d err=%v stderr=%s", code, runErr, stderr.String())
		}
		if !strings.Contains(stderr.String(), "Warning: failed to parse") ||
			!strings.Contains(stderr.String(), "Java syntax error at line 4") {
			t.Fatalf("stderr = %s", stderr.String())
		}
		if !strings.Contains(stdout.String(), "1 warning(s)") {
			t.Fatalf("stdout = %s", stdout.String())
		}
		data, readErr := os.ReadFile(out)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !strings.Contains(string(data), `class "Valid"`) {
			t.Fatalf("valid type was not generated:\n%s", data)
		}
	})

	t.Run("fails when every file is invalid", func(t *testing.T) {
		root := t.TempDir()
		writeJava(t, root, "Broken.java", string(broken))
		var stdout, stderr bytes.Buffer
		code, runErr := Run([]string{"class", root}, &stdout, &stderr)
		if code != exitParse || runErr == nil || runErr.Error() != "failed to parse all Java files" {
			t.Fatalf("code=%d err=%v stderr=%s", code, runErr, stderr.String())
		}
		if !strings.Contains(stderr.String(), "Java syntax error at line 4") {
			t.Fatalf("stderr = %s", stderr.String())
		}
	})
}

func TestParseCacheHitsAndInvalidates(t *testing.T) {
	t.Setenv("UMLGEN_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
	root := t.TempDir()
	writeJava(t, root, "Type.java", "package sample; public class First {}")
	out := filepath.Join(root, "diagram.puml")

	run := func(extra ...string) (string, string) {
		t.Helper()
		args := []string{"class", root, "--verbose", "--output", out}
		args = append(args, extra...)
		var stdout, stderr bytes.Buffer
		code, err := Run(args, &stdout, &stderr)
		if err != nil || code != exitOK {
			t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
		}
		return stdout.String(), stderr.String()
	}

	stdout, stderr := run()
	if stderr != "" || !strings.Contains(stdout, "Cache hits: 0, misses: 1") {
		t.Fatalf("first stdout=%s stderr=%s", stdout, stderr)
	}
	stdout, stderr = run()
	if stderr != "" || !strings.Contains(stdout, "Cache hits: 1, misses: 0") {
		t.Fatalf("second stdout=%s stderr=%s", stdout, stderr)
	}

	writeJava(t, root, "Type.java", "package sample; public class Second {}")
	stdout, stderr = run()
	if stderr != "" || !strings.Contains(stdout, "Cache hits: 0, misses: 1") {
		t.Fatalf("changed stdout=%s stderr=%s", stdout, stderr)
	}
	stdout, stderr = run("--hide-methods")
	if stderr != "" || !strings.Contains(stdout, "Cache hits: 0, misses: 1") {
		t.Fatalf("settings stdout=%s stderr=%s", stdout, stderr)
	}
	stdout, stderr = run("--no-cache")
	if stderr != "" || !strings.Contains(stdout, "Cache: disabled") {
		t.Fatalf("disabled stdout=%s stderr=%s", stdout, stderr)
	}
}

func TestCacheCleanCommand(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	t.Setenv("UMLGEN_CACHE_DIR", root)
	if _, err := javacache.Open(Version, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "java", "entry.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"cache", "clean"}, &stdout, &stderr)
	if err != nil || code != exitOK {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Cleared cache:") {
		t.Fatalf("stdout = %s", stdout.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("cache still exists: %v", err)
	}
}

func TestPNGFormatWithPNGExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock shell script is not executable on Windows")
	}
	dir := t.TempDir()
	writeJava(t, dir, "User.java", `package sample; class User {}`)

	binDir := t.TempDir()
	script := filepath.Join(binDir, "plantuml")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n/usr/bin/printf '' > \"${2%.puml}.png\""), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	out := filepath.Join(dir, "class-diagram.png")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", dir, "--output", out, "--format", "png"}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	puml := filepath.Join(dir, "class-diagram.puml")
	if _, err := os.Stat(puml); err != nil {
		t.Fatalf("puml file was not kept: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("png file was not generated: %v", err)
	}
}

func TestRendererFlagsBeforeTarget(t *testing.T) {
	dir := t.TempDir()
	writeJava(t, dir, "User.java", `package sample; class User {}`)
	out := filepath.Join(dir, "diagram.puml")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{
		"class", dir,
		"--renderer", "local",
		"--server-url", "http://example.com",
		"--render-timeout", "1s",
		"--format", "plantuml",
		"--output", out,
	}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output not written: %v", err)
	}
}

func TestRendererFlagsEqualsForm(t *testing.T) {
	dir := t.TempDir()
	writeJava(t, dir, "User.java", `package sample; class User {}`)
	out := filepath.Join(dir, "diagram.puml")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{
		"class", dir,
		"--renderer=local",
		"--server-url=http://example.com",
		"--render-timeout=1s",
		"--format=plantuml",
		"--output", out,
	}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output not written: %v", err)
	}
}
