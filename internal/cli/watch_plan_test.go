package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/language"
)

func watchTestRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func watchTestRequest(t *testing.T, args ...string) watchRequest {
	t.Helper()
	req, help, err := parseWatchRequest(args, commonOptions{})
	if err != nil || help {
		t.Fatalf("parseWatchRequest(%q): help=%v err=%v", args, help, err)
	}
	return req
}

func watchTestPlan(t *testing.T, req watchRequest) watchPlan {
	t.Helper()
	plan, err := resolveWatchPlan(req)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestWatchSourceMembershipAndContent(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	source := filepath.Join(root, "src", "A.java")
	packageCLIWrite(t, source, "class A {}")
	req := watchTestRequest(t, "src", "--language", "java")
	before := watchTestPlan(t, req)
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	packageCLIWrite(t, source, "class B {}") // Same length and restored timestamp.
	if err := os.Chtimes(source, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	after := watchTestPlan(t, req)
	if reflect.DeepEqual(before.snapshot, after.snapshot) {
		t.Fatal("same-size, same-mtime source replacement was not observed")
	}
	second := filepath.Join(root, "src", "C.java")
	packageCLIWrite(t, second, "class C {}")
	added := watchTestPlan(t, req)
	if reflect.DeepEqual(after.snapshot, added.snapshot) {
		t.Fatal("source addition was not observed")
	}
	if err := os.Rename(second, filepath.Join(root, "src", "D.java")); err != nil {
		t.Fatal(err)
	}
	renamed := watchTestPlan(t, req)
	if reflect.DeepEqual(added.snapshot, renamed.snapshot) {
		t.Fatal("source rename was not observed")
	}
	if err := os.RemoveAll(filepath.Join(root, "src")); err != nil {
		t.Fatal(err)
	}
	missing := watchTestPlan(t, req)
	if !missing.empty || !missing.missing {
		t.Fatalf("deleted root: empty=%v missing=%v", missing.empty, missing.missing)
	}
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	empty := watchTestPlan(t, req)
	if !empty.empty || empty.missing || reflect.DeepEqual(empty.snapshot, missing.snapshot) {
		t.Fatal("empty recreated directory was not distinguished from a missing root")
	}
	packageCLIWrite(t, source, "class A {}")
	if watchTestPlan(t, req).empty {
		t.Fatal("recreated source was not observed")
	}
}

func TestWatchIgnoresUnselectedGeneratedAndTemporaryFiles(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	packageCLIWrite(t, "src/A.java", "class A {}")
	req := watchTestRequest(t, "src", "--language", "java", "--exclude", "ignored", "--output", "src/generated.svg", "--format", "svg", "--include", "visible")
	before := watchTestPlan(t, req)
	for path, content := range map[string]string{
		"src/generated.puml":               "generated diagram",
		"src/generated.svg":                "generated image",
		"src/.umlgen-watch-attempt/A.java": "staged content",
		"src/.umlgen-watch-attempt/go.mod": "module staged",
		"src/ignored/B.java":               "class B {}",
		"src/vendor/C.java":                "class C {}",
		"src/notes/notes.txt":              "irrelevant nested file",
		"src/unselected.go":                "package other",
	} {
		packageCLIWrite(t, path, content)
	}
	after := watchTestPlan(t, req)
	if !reflect.DeepEqual(before.snapshot, after.snapshot) {
		t.Fatalf("unselected/generated/staged files changed the snapshot\nbefore=%v\nafter=%v", before.snapshot, after.snapshot)
	}
	packageCLIWrite(t, "src/dependency/B.java", "package dependency; class B {}")
	if reflect.DeepEqual(after.snapshot, watchTestPlan(t, req).snapshot) {
		t.Fatal("include incorrectly excluded dependency source files from observation")
	}
}

func TestWatchImplicitConfigurationCreationAndDynamicScope(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	packageCLIWrite(t, "first/A.java", "class A {}")
	packageCLIWrite(t, "second/B.java", "class B {}")
	req := watchTestRequest(t, "--language", "java")
	initial := watchTestPlan(t, req)
	implicit := filepath.Join(root, config.DefaultFile)
	if digest, exists := initial.snapshot[implicit]; !exists || digest != [32]byte{} {
		t.Fatal("absent implicit configuration was not observed")
	}
	packageCLIWrite(t, config.DefaultFile, "source: [first]\noutput:\n  file: first-result.svg\n  format: svg\n")
	first := watchTestPlan(t, req)
	if first.output != "first-result.puml" || first.format != "svg" {
		t.Fatalf("unexpected configured output: %s (%s)", first.output, first.format)
	}
	if _, exists := first.snapshot[filepath.Join(root, "second/B.java")]; exists {
		t.Fatal("old configuration scope retained")
	}
	packageCLIWrite(t, config.DefaultFile, "source: [second]\noutput:\n  file: second-result.puml\n")
	second := watchTestPlan(t, req)
	if _, exists := second.snapshot[filepath.Join(root, "first/A.java")]; exists {
		t.Fatal("previous source scope retained")
	}
	if _, exists := second.snapshot[filepath.Join(root, "second/B.java")]; !exists {
		t.Fatal("new configured source scope missing")
	}
	packageCLIWrite(t, "first/A.java", "class Changed {}")
	if !reflect.DeepEqual(second.snapshot, watchTestPlan(t, req).snapshot) {
		t.Fatal("file outside current configuration scope triggered an update")
	}
	packageCLIWrite(t, config.DefaultFile, "source: [\n")
	if _, err := resolveWatchPlan(req); err == nil {
		t.Fatal("invalid replacement configuration accepted")
	}
	packageCLIWrite(t, config.DefaultFile, "source: [first]\n")
	if watchTestPlan(t, req).empty {
		t.Fatal("valid configuration did not recover observation")
	}
}

func TestWatchConfigurationAndCLIPathPrecedence(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	packageCLIWrite(t, "settings/sources/A.java", "class A {}")
	packageCLIWrite(t, "override/B.java", "class B {}")
	path := filepath.Join(root, "settings/custom.yaml")
	packageCLIWrite(t, path, "source: [sources]\noutput:\n  file: diagrams/config.svg\n  format: svg\nrenderer:\n  type: server\n  server_url: https://example.invalid/plantuml\n  timeout: 3s\n")
	req, _, err := parseWatchRequest([]string{"package"}, commonOptions{configPath: path})
	if err != nil {
		t.Fatal(err)
	}
	configured := watchTestPlan(t, req)
	if configured.output != filepath.Join(root, "settings/diagrams/config.puml") {
		t.Fatalf("config output did not resolve relative to config: %s", configured.output)
	}
	if _, exists := configured.snapshot[filepath.Join(root, "settings/sources/A.java")]; !exists {
		t.Fatal("config source did not resolve relative to config")
	}
	if configured.renderer.ServerURL != "https://example.invalid/plantuml" || configured.renderer.Timeout != 3*time.Second {
		t.Fatalf("renderer configuration was not resolved: %+v", configured.renderer)
	}
	override := watchTestPlan(t, watchTestRequest(t, "package", "override", "--config", path, "-o", "cli-result.png", "-f", "PNG", "--renderer", "local", "--render-timeout", "7s"))
	if override.output != "cli-result.puml" || override.format != "png" {
		t.Fatalf("CLI output did not take precedence: %s (%s)", override.output, override.format)
	}
	if _, exists := override.snapshot[filepath.Join(root, "override/B.java")]; !exists {
		t.Fatal("CLI target did not override configured source")
	}
	if _, exists := override.snapshot[filepath.Join(root, "settings/sources/A.java")]; exists {
		t.Fatal("configured target survived CLI override")
	}
	if override.renderer.Type != "local" || override.renderer.Timeout != 7*time.Second {
		t.Fatalf("CLI renderer options did not take precedence: %+v", override.renderer)
	}
}

func TestWatchDefaultOutputMatchesNormalDiagramCommands(t *testing.T) {
	for _, diagram := range []string{"class", "package"} {
		t.Run(diagram, func(t *testing.T) {
			root := watchTestRoot(t)
			packageCLIChdir(t, root)
			packageCLIWrite(t, "settings/src/A.java", "package sample; class A {}")
			packageCLIWrite(t, "settings/custom.yaml", "source: [src]\n")
			req := watchTestRequest(t, diagram, "--config", "settings/custom.yaml")
			plan := watchTestPlan(t, req)
			var stdout, stderr bytes.Buffer
			code, err := Run([]string{diagram, "--config", "settings/custom.yaml"}, &stdout, &stderr)
			if err != nil || code != exitOK {
				t.Fatalf("normal command: code=%d err=%v stderr=%s", code, err, stderr.String())
			}
			content := packageCLIRead(t, plan.output)
			if !strings.Contains(content, "@startuml") {
				t.Fatalf("planned default is not the normal output: %s", plan.output)
			}
		})
	}
}

func TestWatchModuleChangesRebindCachedSyntax(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	source := filepath.Join(root, "src/nested/example.go")
	packageCLIWrite(t, source, "package nested\ntype Example struct{}\n")
	req := watchTestRequest(t, "src", "--language", "go")
	backend, err := language.ForName("go")
	if err != nil {
		t.Fatal(err)
	}
	analysis := newSourceAnalysis(sourceAnalysisOptions{Config: config.Defaults(), Parser: backend}, io.Discard, io.Discard)
	before := watchTestPlan(t, req)
	initial := analysis.parseFile(source)
	if initial.Err != nil {
		t.Fatal(initial.Err)
	}
	module := filepath.Join(root, "go.mod")
	if digest, exists := before.snapshot[module]; !exists || digest != [32]byte{} {
		t.Fatal("absent ancestor module is not observed")
	}
	packageCLIWrite(t, module, "module example.com/outer\n")
	after := watchTestPlan(t, req)
	bound := analysis.parseFile(source)
	if bound.Err != nil {
		t.Fatal(bound.Err)
	}
	if reflect.DeepEqual(before.snapshot, after.snapshot) || bound.Unit.PackagePath != "example.com/outer/src/nested" {
		t.Fatalf("new ancestor module did not update identity: %s", bound.Unit.PackagePath)
	}
	packageCLIWrite(t, "src/nested/go.mod", "module example.com/inner\n")
	nested := watchTestPlan(t, req)
	inner := analysis.parseFile(source)
	if inner.Err != nil {
		t.Fatal(inner.Err)
	}
	if reflect.DeepEqual(after.snapshot, nested.snapshot) || inner.Unit.PackagePath != "example.com/inner" {
		t.Fatalf("new nested module did not update identity: %s", inner.Unit.PackagePath)
	}
	if analysis.hits == 0 {
		t.Fatal("module-only changes discarded reusable syntax cache")
	}
	if err := os.Remove("src/nested/go.mod"); err != nil {
		t.Fatal(err)
	}
	deleted := watchTestPlan(t, req)
	rebound := analysis.parseFile(source)
	if rebound.Err != nil {
		t.Fatal(rebound.Err)
	}
	if reflect.DeepEqual(nested.snapshot, deleted.snapshot) || rebound.Unit.PackagePath != bound.Unit.PackagePath {
		t.Fatal("module deletion did not restore ancestor identity")
	}
}

func TestWatchAutoDetectionReevaluatesProjectMarkers(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	packageCLIWrite(t, "src/A.java", "class A {}")
	packageCLIWrite(t, "src/example.go", "package example\ntype Example struct{}\n")
	packageCLIWrite(t, "src/pom.xml", "<project/>")
	req := watchTestRequest(t, "src")
	java := watchTestPlan(t, req)
	if _, exists := java.snapshot[filepath.Join(root, "src/A.java")]; !exists {
		t.Fatal("Java marker did not select Java sources")
	}
	if _, exists := java.snapshot[filepath.Join(root, "src/example.go")]; exists {
		t.Fatal("Java marker selected Go source")
	}
	packageCLIWrite(t, "src/go.mod", "module example.com/project\n")
	goPlan := watchTestPlan(t, req)
	if _, exists := goPlan.snapshot[filepath.Join(root, "src/example.go")]; !exists {
		t.Fatal("new Go marker did not select Go sources")
	}
	if _, exists := goPlan.snapshot[filepath.Join(root, "src/A.java")]; exists {
		t.Fatal("new Go marker retained Java source scope")
	}
	if reflect.DeepEqual(java.snapshot, goPlan.snapshot) {
		t.Fatal("automatic backend transition was not observed")
	}
}

func TestWatchRefusesInputOutputCollisions(t *testing.T) {
	for _, collision := range []string{"source", "config", "module", "hardlink"} {
		t.Run(collision, func(t *testing.T) {
			root := watchTestRoot(t)
			packageCLIChdir(t, root)
			packageCLIWrite(t, "src/A.java", "class A {}")
			packageCLIWrite(t, "settings.yaml", "source: [src]\n")
			output := "src/A.java"
			switch collision {
			case "config":
				output = "settings.yaml"
			case "module":
				output = "src/go.mod"
			case "hardlink":
				output = "alias.puml"
				if err := os.Link("src/A.java", output); err != nil {
					t.Fatal(err)
				}
			}
			req := watchTestRequest(t, "--config", "settings.yaml", "--language", "java", "-o", output)
			if _, err := resolveWatchPlan(req); err == nil {
				t.Fatalf("accepted %s output collision", collision)
			}
			if got := packageCLIRead(t, "src/A.java"); got != "class A {}" {
				t.Fatal("planner modified the source input")
			}
		})
	}
}

func TestWatchPreservesFlagLikeValues(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	packageCLIWrite(t, "src/A.java", "class A {}")
	req := watchTestRequest(t, "src", "--title", "--interval", "--interval", "1ns", "--debounce=0", "-o", "result.puml")
	plan := watchTestPlan(t, req)
	if plan.title != "--interval" || req.interval != time.Nanosecond || req.debounce != 0 {
		t.Fatalf("flag-like value changed semantics: title=%q interval=%s debounce=%s", plan.title, req.interval, req.debounce)
	}
	var stdout, stderr bytes.Buffer
	code, err := runClass(req.args, req.common, &stdout, &stderr)
	if err != nil || code != exitOK {
		t.Fatalf("generation: code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	if !strings.Contains(packageCLIRead(t, plan.output), "title --interval") {
		t.Fatal("flag-like title was not retained in generated diagram")
	}
	packageCLIWrite(t, "--debounce", "source: [src]\n")
	configReq := watchTestRequest(t, "--config", "--debounce", "--interval=1ns")
	if watchTestPlan(t, configReq).empty {
		t.Fatal("flag-like config filename was consumed as a watcher option")
	}
}

func TestWatchPlanningDoesNotParseOrExecuteSources(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	packageCLIWrite(t, "src/example.go", "this is not valid Go\nfunc init() { panic(\"must never execute\") }\n")
	plan := watchTestPlan(t, watchTestRequest(t, "src", "--language", "go", "--output", "result.puml"))
	if plan.empty {
		t.Fatal("unparsed source was not selected")
	}
	if _, err := os.Stat(plan.output); !os.IsNotExist(err) {
		t.Fatalf("planning generated output: %v", err)
	}
}

func TestWatchSourceReadErrorsAreNotEmptyScopes(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	if err := os.Mkdir("src", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing.java", "src/A.java"); err != nil {
		t.Fatal(err)
	}
	_, err := resolveWatchPlan(watchTestRequest(t, "src", "--language", "java"))
	var attempt *watchAttemptError
	if !errors.As(err, &attempt) || attempt.code != exitError {
		t.Fatalf("source read failure did not remain an input failure: %v", err)
	}
}

func TestWatchRejectsInvalidOptionsAndHelpAvoidsFilesystem(t *testing.T) {
	for _, args := range [][]string{
		{"--interval", "0"}, {"--interval", "-1ns"}, {"--debounce", "-1ns"},
		{"--interval", "not-a-duration"}, {"--unknown"}, {"a", "b"},
		{"--depth", "2"}, {"--depth", "-1"}, {"--direction", "sideways"},
		{"--verbose", "--quiet"}, {"--relations", "unknown"},
		{"package", "--focus="},
	} {
		if _, help, err := parseWatchRequest(args, commonOptions{}); err == nil || help {
			t.Errorf("accepted invalid arguments %q: help=%v err=%v", args, help, err)
		}
	}
	for _, diagram := range []string{"class", "package"} {
		if _, help, err := parseWatchRequest([]string{diagram, "--config", "/does/not/exist.yaml", "--help"}, commonOptions{}); err != nil || !help {
			t.Errorf("help required filesystem/configuration: help=%v err=%v", help, err)
		}
	}
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	for _, args := range [][]string{
		{"--output="}, {"--format", "pdf"}, {"--language", "rust"},
		{"--layout-direction", "sideways"}, {"--skinparam", "invalid"},
		{"package", "--cycles", "unknown"}, {"package", "--external", "unknown"},
		{"package", "--dependency-source", "imports", "--relations", "field"},
	} {
		req := watchTestRequest(t, args...)
		if _, err := resolveWatchPlan(req); err == nil {
			t.Errorf("accepted invalid planned options %q", args)
		}
	}
}

func TestWatchRejectsUnsafeOutputDestinations(t *testing.T) {
	for _, destination := range []string{"symlink", "directory", "future-java-source", "future-go-source", "image-symlink"} {
		t.Run(destination, func(t *testing.T) {
			root := watchTestRoot(t)
			packageCLIChdir(t, root)
			packageCLIWrite(t, "src/A.java", "class A {}")
			args := []string{"src", "--output", "result.puml"}
			switch destination {
			case "symlink":
				if err := os.Symlink("src/A.java", "result.puml"); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir("result.puml", 0o755); err != nil {
					t.Fatal(err)
				}
			case "future-java-source":
				args = []string{"src", "--output", "src/future.java"}
			case "future-go-source":
				args = []string{"src", "--output", "src/future.go"}
			case "image-symlink":
				args = append(args, "--format", "svg")
				if err := os.Symlink("src/A.java", "result.svg"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := resolveWatchPlan(watchTestRequest(t, args...)); err == nil {
				t.Fatalf("accepted unsafe output destination %s", destination)
			}
			if got := packageCLIRead(t, "src/A.java"); got != "class A {}" {
				t.Fatal("planning changed a source")
			}
		})
	}
}

func TestWatchAutomaticGoScopeIgnoresTestAndStagingFiles(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	packageCLIWrite(t, "src/example.go", "package example\n")
	req := watchTestRequest(t, "src")
	before := watchTestPlan(t, req)
	for path, content := range map[string]string{
		"src/example_test.go":              "package example\n",
		"src/.umlgen-watch-attempt/A.java": "class A {}",
		"src/.umlgen-watch-attempt/go.mod": "module staged\n",
		"src/notes/readme.txt":             "not an observed input",
	} {
		packageCLIWrite(t, path, content)
	}
	if !reflect.DeepEqual(before.snapshot, watchTestPlan(t, req).snapshot) {
		t.Fatal("test/staging/unrelated files changed automatic Go observation")
	}
	packageCLIWrite(t, "src/nested/go.mod", "module example.com/nested\n")
	if reflect.DeepEqual(before.snapshot, watchTestPlan(t, req).snapshot) {
		t.Fatal("new module below a selected root was not observed")
	}
}

func TestWatchConfigDeletionAndExcludeOverride(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	packageCLIWrite(t, "src/hidden/A.java", "class A {}")
	packageCLIWrite(t, "src/visible/B.java", "class B {}")
	packageCLIWrite(t, config.DefaultFile, "source: [src]\nexclude: [hidden]\n")
	configured := watchTestPlan(t, watchTestRequest(t, "package"))
	hidden := filepath.Join(root, "src/hidden/A.java")
	visible := filepath.Join(root, "src/visible/B.java")
	if _, exists := configured.snapshot[hidden]; exists {
		t.Fatal("configured path exclusion was ignored")
	}
	req := watchTestRequest(t, "package", "--exclude", "visible")
	override := watchTestPlan(t, req)
	if _, exists := override.snapshot[hidden]; !exists {
		t.Fatal("CLI exclusion did not replace configured exclusion")
	}
	if _, exists := override.snapshot[visible]; exists {
		t.Fatal("CLI exclusion was ignored")
	}
	if err := os.Remove(config.DefaultFile); err != nil {
		t.Fatal(err)
	}
	deleted := watchTestPlan(t, req)
	if reflect.DeepEqual(override.snapshot, deleted.snapshot) {
		t.Fatal("configuration deletion was not observed")
	}
	if digest := deleted.snapshot[filepath.Join(root, config.DefaultFile)]; digest != [32]byte{} {
		t.Fatal("deleted implicit configuration retained content")
	}
}

func TestWatchStagedExecutionRespectsOptionTerminatorAndFlagLikeTitle(t *testing.T) {
	for _, diagram := range []string{"class", "package"} {
		t.Run(diagram, func(t *testing.T) {
			root := watchTestRoot(t)
			packageCLIChdir(t, root)
			packageCLIWrite(t, "src/A.java", "package sample; class A {}")
			req := watchTestRequest(t, diagram, "--title", "--output", "-o", "final.puml", "--", "src")
			plan := watchTestPlan(t, req)
			stage := filepath.Join(root, ".umlgen-watch-attempt", "final.puml")
			args := append([]string{"--output", stage}, plan.args...)
			var stdout, stderr bytes.Buffer
			var code int
			var err error
			if diagram == "package" {
				code, err = runPackage(args, plan.common, &stdout, &stderr)
			} else {
				code, err = runClass(args, plan.common, &stdout, &stderr)
			}
			if err != nil || code != exitOK {
				t.Fatalf("staged generation: code=%d err=%v stderr=%s", code, err, stderr.String())
			}
			if _, err := os.Stat(plan.output); !os.IsNotExist(err) {
				t.Fatalf("staged generation wrote the final path: %v", err)
			}
		})
	}
}

func TestWatchPartialConfiguredRootDeletionKeepsRemainingScope(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	packageCLIWrite(t, "first/A.java", "class A {}")
	packageCLIWrite(t, "second/B.java", "class B {}")
	packageCLIWrite(t, config.DefaultFile, "language: java\nsource: [first, second]\n")
	req := watchTestRequest(t)
	before := watchTestPlan(t, req)
	if err := os.RemoveAll("first"); err != nil {
		t.Fatal(err)
	}
	after := watchTestPlan(t, req)
	if !after.missing || after.empty {
		t.Fatalf("partial root deletion: missing=%v empty=%v", after.missing, after.empty)
	}
	if _, exists := after.snapshot[filepath.Join(root, "first/A.java")]; exists {
		t.Fatal("deleted root retained a source input")
	}
	if _, exists := after.snapshot[filepath.Join(root, "second/B.java")]; !exists {
		t.Fatal("surviving root lost its source input")
	}
	if reflect.DeepEqual(before.snapshot, after.snapshot) {
		t.Fatal("partial root deletion was not observed")
	}
	packageCLIWrite(t, "first/C.java", "class C {}")
	recreated := watchTestPlan(t, req)
	if recreated.missing || recreated.empty {
		t.Fatal("recreated configured root did not recover")
	}
	if _, exists := recreated.snapshot[filepath.Join(root, "first/C.java")]; !exists {
		t.Fatal("recreated source input was not selected")
	}
}

func TestWatchPlannedGenerationExcludesDeletedAndStagingScopes(t *testing.T) {
	for _, diagram := range []string{"class", "package"} {
		t.Run(diagram, func(t *testing.T) {
			root := watchTestRoot(t)
			packageCLIChdir(t, root)
			packageCLIWrite(t, "first/A.java", "package removed; class A {}")
			packageCLIWrite(t, "second/B.java", "package surviving; class B {}")
			packageCLIWrite(t, "second/.umlgen-watch-old/Broken.java", "this is not Java")
			packageCLIWrite(t, config.DefaultFile, "language: java\nsource: [first, second]\n")
			if err := os.RemoveAll("first"); err != nil {
				t.Fatal(err)
			}
			plan := watchTestPlan(t, watchTestRequest(t, diagram))
			common := plan.common
			common.watchTargets, common.watchExcludes, common.strictParse = plan.targets, plan.excludes, true
			args := append([]string{"--output", "attempt.puml"}, plan.args...)
			var stdout, stderr bytes.Buffer
			var code int
			var err error
			if diagram == "package" {
				code, err = runPackage(args, common, &stdout, &stderr)
			} else {
				code, err = runClass(args, common, &stdout, &stderr)
			}
			if err != nil || code != exitOK {
				t.Fatalf("live-scope generation: code=%d err=%v stderr=%s", code, err, stderr.String())
			}
			content := packageCLIRead(t, "attempt.puml")
			if !strings.Contains(content, "surviving") || strings.Contains(content, "removed") {
				t.Fatalf("generated diagram retained a deleted source scope:\n%s", content)
			}
		})
	}
}

func TestWatchRefusesAliasedInputOutputCollisions(t *testing.T) {
	for _, diagram := range []string{"class", "package"} {
		for _, collision := range []string{"implicit-config", "implicit-config-case", "explicit-config", "module-case", "existing-source", "missing-source", "missing-source-case", "aliased-source-root", "future-java-source", "future-go-source", "source-parent-traversal"} {
			t.Run(diagram+"/"+collision, func(t *testing.T) {
				root := watchTestRoot(t)
				packageCLIChdir(t, root)
				packageCLIWrite(t, "src/A.java", "class A {}")
				packageCLIWrite(t, "src/example.go", "package example\n")
				if err := os.MkdirAll("src/nested", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(root, "alias"); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
				args := []string{diagram, "src", "--language", "java", "--output"}
				output := "alias/.umlgen.yaml"
				switch collision {
				case "implicit-config-case":
					output = "alias/.UMLGEN.YAML"
				case "explicit-config":
					packageCLIWrite(t, "settings.yaml", "source: [src]\n")
					output = "alias/settings.yaml"
					args = append(args[:len(args)-1], "--config", "settings.yaml", "--output")
				case "module-case":
					output = "alias/src/GO.MOD"
					args = []string{diagram, "src", "--language", "go", "--output"}
				case "existing-source":
					output = "alias/src/A.java"
				case "missing-source":
					output = "alias/src/Future.java"
					args = []string{diagram, "src/Future.java", "--language", "java", "--output"}
				case "missing-source-case":
					output = "alias/src/future.java"
					args = []string{diagram, "src/Future.java", "--language", "java", "--output"}
				case "aliased-source-root":
					output = "src/Future.java"
					args = []string{diagram, "alias/src", "--language", "java", "--output"}
				case "future-java-source":
					output = "alias/src/future/nested/New.java"
				case "future-go-source":
					output = "alias/src/future/nested/new.go"
					args = []string{diagram, "src", "--language", "go", "--output"}
				case "source-parent-traversal":
					if err := os.Symlink(filepath.Join(root, "src", "nested"), "source-link"); err != nil {
						t.Fatal(err)
					}
					output = "source-link/../Future.java"
				}
				args = append(args, output)
				if _, err := resolveWatchPlan(watchTestRequest(t, args...)); err == nil || !strings.Contains(err.Error(), "collides") {
					t.Fatalf("expected an input collision for aliased %s output %s, got %v", collision, output, err)
				}
				if got := packageCLIRead(t, "src/A.java"); got != "class A {}" {
					t.Fatal("planning modified the Java source")
				}
				if got := packageCLIRead(t, "src/example.go"); got != "package example\n" {
					t.Fatal("planning modified the Go source")
				}
				if _, err := os.Stat(".umlgen.yaml"); !os.IsNotExist(err) {
					t.Fatalf("planning created an implicit config: %v", err)
				}
				if _, err := os.Stat("src/go.mod"); !os.IsNotExist(err) {
					t.Fatalf("planning created module metadata: %v", err)
				}
			})
		}
	}
}

func TestWatchAliasedFutureOutputRespectsSourceExclusions(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	packageCLIWrite(t, "src/A.java", "class A {}")
	if err := os.Symlink(filepath.Join(root, "src"), "alias"); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	before := watchTestPlan(t, watchTestRequest(t, "src", "--language", "java", "--exclude", "src/generated"))
	after := watchTestPlan(t, watchTestRequest(t, "src", "--language", "java", "--exclude", "src/generated", "--output", "alias/generated/future.java"))
	if !reflect.DeepEqual(before.snapshot, after.snapshot) {
		t.Fatal("an aliased output in an excluded directory changed the selected inputs")
	}
	if _, ok := after.snapshot[filepath.Join(root, "src", "A.java")]; !ok || after.empty {
		t.Fatal("allowing an excluded output lost the selected Java source")
	}
	if _, err := os.Stat("src/generated/future.java"); !os.IsNotExist(err) {
		t.Fatalf("planning created the future output: %v", err)
	}
}

func TestWatchOutputParentTraversalUsesPhysicalStagingParent(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	packageCLIWrite(t, "src/A.java", "class A {}")
	if err := os.MkdirAll("physical/nested", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "physical", "nested"), "alias"); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	plan := watchTestPlan(t, watchTestRequest(t, "src", "--language", "java", "--output", "alias/../diagrams/result.puml"))
	want := filepath.Join(root, "physical", "diagrams", "result.puml")
	if plan.output != want {
		t.Fatalf("output would stage/publish through the lexical parent: got %s, want %s", plan.output, want)
	}
	if _, ok := plan.snapshot[filepath.Join(root, "src", "A.java")]; !ok || plan.empty {
		t.Fatal("resolving the output parent changed the selected source")
	}
	if _, err := os.Stat(filepath.Dir(want)); !os.IsNotExist(err) {
		t.Fatalf("planning created the staging parent: %v", err)
	}
}

func TestWatchOutputSafetyDoesNotFoldDistinctSourceRoots(t *testing.T) {
	root := watchTestRoot(t)
	packageCLIChdir(t, root)
	if err := os.Mkdir("src", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir("Src", 0o755); err != nil {
		if os.IsExist(err) {
			t.Skip("filesystem is case-insensitive")
		}
		t.Fatal(err)
	}
	packageCLIWrite(t, "src/A.java", "class A {}")
	packageCLIWrite(t, "Src/Other.java", "class Other {}")
	plan := watchTestPlan(t, watchTestRequest(t, "src", "--language", "java", "--output", "Src/Future.java"))
	if _, ok := plan.snapshot[filepath.Join(root, "src", "A.java")]; !ok || plan.empty {
		t.Fatal("case-distinct output root changed the selected source")
	}
	if _, ok := plan.snapshot[filepath.Join(root, "Src", "Other.java")]; ok {
		t.Fatal("source scope expanded into a case-distinct physical directory")
	}
	if _, err := os.Stat("Src/Future.java"); !os.IsNotExist(err) {
		t.Fatalf("planning created the output: %v", err)
	}
}
