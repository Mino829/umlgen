package cli

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
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

func TestPNGFormatRejectsEmptyOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock shell script is not executable on Windows")
	}
	dir := t.TempDir()
	writeJava(t, dir, "User.java", `package sample; class User {}`)

	binDir := t.TempDir()
	script := filepath.Join(binDir, "plantuml")
	pngPath := filepath.Join(dir, "class-diagram.png")
	if err := os.WriteFile(script, []byte(fmt.Sprintf("#!/bin/sh\n: > %q\n", pngPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	out := filepath.Join(dir, "class-diagram.puml")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", dir, "--output", out, "--format", "png", "--renderer", "local"}, &stdout, &stderr)
	if err != nil || code != exitRender {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("puml file was not kept: %v", err)
	}
	if !strings.Contains(stderr.String(), "empty output file") {
		t.Fatalf("missing empty output diagnostic: %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "Generated "+pngPath) {
		t.Fatalf("reported empty image as generated: %s", stdout.String())
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
	// Changing output settings (like --hide-methods) should NOT invalidate cache
	stdout, stderr = run("--hide-methods")
	if stderr != "" || !strings.Contains(stdout, "Cache hits: 1, misses: 0") {
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

	var fixture bytes.Buffer
	bounds := image.Rect(0, 0, 2, 3)
	if err := png.Encode(&fixture, image.NewRGBA(bounds)); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	fixturePath := filepath.Join(binDir, "fixture.png")
	if err := os.WriteFile(fixturePath, fixture.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(binDir, "plantuml")
	if err := os.WriteFile(script, []byte(fmt.Sprintf("#!/bin/sh\n/bin/cp %q \"${2%%.puml}.png\"\n", fixturePath)), 0o755); err != nil {
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
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("png file was not generated: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("invalid PNG output: %v", err)
	}
	if img.Bounds() != bounds {
		t.Fatalf("PNG bounds = %v, want %v", img.Bounds(), bounds)
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

func TestExcludePackageWithVariousFormats(t *testing.T) {
	tests := []struct {
		name    string
		pkg     string
		exclude []string
		want    bool
	}{
		// Java package names (with dots)
		{"Java exact match", "com.example.test", []string{"com.example.test"}, true},
		{"Java prefix match", "com.example.test.sub", []string{"com.example.test"}, true},
		{"Java no match", "com.example.other", []string{"com.example.test"}, false},

		// Go package names (no dots)
		{"Go exact match", "cmd", []string{"cmd"}, true},
		{"Go prefix match", "cmd/app", []string{"cmd"}, true},
		{"Go internal prefix", "internal/util", []string{"internal"}, true},
		{"Go no match", "pkg", []string{"cmd"}, false},

		// Path-based excludes
		{"Path prefix match", "src/test/java/com/example", []string{"src/test"}, true},
		{"Path no match", "src/main/java/com/example", []string{"src/test"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := excludePackage(tt.pkg, tt.exclude)
			if got != tt.want {
				t.Errorf("excludePackage(%q, %v) = %v, want %v", tt.pkg, tt.exclude, got, tt.want)
			}
		})
	}
}

func TestExplainLocationsAndDiagramSelection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("UMLGEN_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
	source := filepath.Join(dir, "Service.java")
	writeJava(t, dir, "Service.java", `package sample;
class Base {}
interface Contract {}
class Target {}
class Service extends Base implements Contract {
    Target primary;
    Target load(
        Target input) { return null; }
    Target load(Target input, int limit) { return null; }
}
class Other { Target ignored; }
`)
	run := func(name string, extra ...string) (string, []byte) {
		t.Helper()
		path := filepath.Join(dir, name+".puml")
		args := append([]string{"class", dir, "--quiet", "--output", path}, extra...)
		var stdout, stderr bytes.Buffer
		code, err := Run(args, &stdout, &stderr)
		if err != nil || code != exitOK {
			t.Fatalf("%s: code=%d err=%v stderr=%s", name, code, err, stderr.String())
		}
		diagram, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return stdout.String(), diagram
	}
	_, ordinary := run("ordinary")
	explained, withExplain := run("explained", "--explain")
	if !bytes.Equal(ordinary, withExplain) {
		t.Fatal("--explain changed the PlantUML diagram")
	}
	for _, line := range []int{5, 6, 7, 8, 9, 11} {
		if !strings.Contains(explained, fmt.Sprintf("%s:%d:", source, line)) {
			t.Errorf("missing declaration at line %d:\n%s", line, explained)
		}
	}
	for _, member := range []string{"load(Target)", "load(Target, int)", "primary", "ignored"} {
		if !strings.Contains(explained, member) {
			t.Errorf("missing member %q:\n%s", member, explained)
		}
	}
	filtered, _ := run("filtered", "--explain", "--focus", "Service", "--direction", "out", "--relations", "field")
	if !strings.Contains(filtered, "sample.Service -> sample.Target [field") {
		t.Fatalf("missing selected relation:\n%s", filtered)
	}
	for _, excluded := range []string{"sample.Other", "[return", "[parameter", "[inheritance", "[implementation"} {
		if strings.Contains(filtered, excluded) {
			t.Errorf("explained an excluded relation %q:\n%s", excluded, filtered)
		}
	}
	hidden, _ := run("hidden", "--explain", "--no-relations")
	if strings.Contains(hidden, " -> ") || strings.Contains(hidden, source) {
		t.Fatalf("explained relationships hidden from the diagram:\n%s", hidden)
	}
}

func TestLayoutFlagsOverrideConfigurationPerSetting(t *testing.T) {
	dir := t.TempDir()
	writeJava(t, dir, "Service.java", "package app.service; class Service { app.domain.User user; }")
	writeJava(t, dir, "User.java", "package app.domain; class User {}")
	configPath := filepath.Join(dir, ".umlgen.yaml")
	if err := os.WriteFile(configPath, []byte(`layout:
  package_grouping: hierarchical
  direction: left-to-right
  skinparams:
    shadowing: true
    linetype: ortho
`), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(name string, extra ...string) string {
		t.Helper()
		path := filepath.Join(dir, name+".puml")
		args := append([]string{"class", dir, "--config", configPath, "--output", path}, extra...)
		var stdout, stderr bytes.Buffer
		code, err := Run(args, &stdout, &stderr)
		if err != nil || code != exitOK {
			t.Fatalf("%s: code=%d err=%v stderr=%s", name, code, err, stderr.String())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	configured := run("configured")
	for _, want := range []string{`package "app"`, `package "service"`, `package "domain"`,
		"left to right direction", "skinparam shadowing true", "skinparam linetype ortho"} {
		if !strings.Contains(configured, want) {
			t.Errorf("missing configured setting %q:\n%s", want, configured)
		}
	}
	overridden := run("overridden", "--package-grouping", "none", "--layout-direction", "top-to-bottom",
		"--skinparam", "shadowing=false")
	if strings.Contains(overridden, "package ") || strings.Contains(overridden, "left to right direction") {
		t.Fatalf("CLI layout overrides were ignored:\n%s", overridden)
	}
	for _, want := range []string{"skinparam shadowing false", "skinparam linetype ortho",
		`T_app_service_Service --> "1" T_app_domain_User`} {
		if !strings.Contains(overridden, want) {
			t.Errorf("missing overridden or retained setting %q:\n%s", want, overridden)
		}
	}
}

func TestInvalidLayoutPreservesExistingOutput(t *testing.T) {
	for _, args := range [][]string{
		{"--package-grouping", "invalid"},
		{"--layout-direction", "out"},
		{"--skinparam", "shadowing=false\n!include remote.puml"},
		{"--skinparam", "shadowing"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := t.TempDir()
			writeJava(t, dir, "User.java", "package sample; class User {}")
			path := filepath.Join(dir, "existing.puml")
			const existing = "@startuml\nclass Existing\n@enduml\n"
			if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			code, err := Run(append([]string{"class", dir, "--output", path}, args...), &stdout, &stderr)
			if code != exitArgs || err == nil {
				t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != existing {
				t.Fatalf("invalid settings modified output: data=%q err=%v", data, err)
			}
		})
	}
}

func TestExplainDiagnosticsRespectFocusAndRelationSelection(t *testing.T) {
	dir := t.TempDir()
	writeJava(t, dir, "Service.java", `package sample;
class Target {}
class Service {
    Target hidden;
    Missing missing;
    Target load(Missing parameter) { return null; }
}`)
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", dir, "--focus", "Service", "--depth", "0",
		"--relations", "field", "--quiet", "--explain", "-o", filepath.Join(dir, "diagram.puml")}, &stdout, &stderr)
	if code != exitOK || err != nil {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	text := stdout.String()
	for _, want := range []string{`[field, outside-diagram]: reference="Target", target=sample.Target`,
		`[field, unresolved]: reference="Missing"`, "4 |     Target hidden;", "5 |     Missing missing;"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing declaration diagnostic %q:\n%s", want, text)
		}
	}
	for _, absent := range []string{" -> ", "[parameter,", "[return,"} {
		if strings.Contains(text, absent) {
			t.Errorf("explained a non-selected relationship %q:\n%s", absent, text)
		}
	}
}

func TestExplainRetainsAnalysisFailuresWithPartialAndAllFailedInputs(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "Broken.java")
	writeJava(t, dir, "Broken.java", "package sample; class Broken {")
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprintf("partial=%v", partial), func(t *testing.T) {
			if partial {
				writeJava(t, dir, "Service.java", "package sample; class Service { Missing dependency; }")
			}
			output := filepath.Join(dir, "diagram.puml")
			var stdout, stderr bytes.Buffer
			code, err := Run([]string{"class", dir, "--quiet", "--explain", "-o", output}, &stdout, &stderr)
			if partial {
				if code != exitOK || err != nil {
					t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
				}
				if _, err := os.Stat(output); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(stdout.String(), `reference="Missing"`) {
					t.Fatalf("partial unresolved evidence was lost:\n%s", stdout.String())
				}
			} else {
				if code != exitParse || err == nil {
					t.Fatalf("code=%d err=%v", code, err)
				}
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatalf("all-failed analysis generated a diagram: %v", err)
				}
			}
			if !strings.Contains(stdout.String(), broken+" (working tree):") ||
				!strings.Contains(stdout.String(), "Analysis failures:") {
				t.Fatalf("analysis failure absent from explanation:\n%s", stdout.String())
			}
		})
	}
}

func TestDiffExplainUsesDeletedSourceSnapshot(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "umlgen@example.test")
	runGit(t, root, "config", "user.name", "umlgen test")
	writeJava(t, root, "Target.java", "package sample;\nclass Target {}\n")
	writeJava(t, root, "Service.java", "package sample;\nclass Service {\n    Target original;\n}\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "base")
	deleted := filepath.Join(root, "Service.java")
	if err := os.Remove(deleted); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"diff", "HEAD", "--quiet", "--explain", "-o", filepath.Join(root, "diff.puml")}, &stdout, &stderr)
	if code != exitOK || err != nil {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	for _, want := range []string{"Git base", "3 |     Target original;", "sample.Service -> sample.Target [field"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("missing deleted-source evidence %q:\n%s", want, stdout.String())
		}
	}
}
