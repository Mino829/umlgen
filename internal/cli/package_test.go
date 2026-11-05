package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

type packageCLIResult struct {
	code   int
	err    error
	stdout string
	stderr string
}

func packageCLIRun(args ...string) packageCLIResult {
	var stdout, stderr bytes.Buffer
	code, err := Run(args, &stdout, &stderr)
	return packageCLIResult{code: code, err: err, stdout: stdout.String(), stderr: stderr.String()}
}

func packageCLISuccess(t *testing.T, args ...string) packageCLIResult {
	t.Helper()
	result := packageCLIRun(args...)
	if result.code != exitOK || result.err != nil {
		t.Fatalf("Run(%q): code=%d err=%v\nstdout=%s\nstderr=%s", args, result.code, result.err, result.stdout, result.stderr)
	}
	return result
}

func packageCLIWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func packageCLIRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func packageCLIChdir(t *testing.T, path string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Error(err)
		}
	})
}

func packageCLIConfig(t *testing.T, root, content string) string {
	t.Helper()
	path := filepath.Join(root, "settings.yaml")
	packageCLIWrite(t, path, content)
	return path
}

var packageCLINodePattern = regexp.MustCompile(`(?m)^package "([^"]*)" as ([A-Za-z0-9_]+)`)
var packageCLIUnicodePattern = regexp.MustCompile(`<U\+([0-9A-F]{4,6})>`)

func packageCLINodes(diagram string) map[string]string {
	nodes := make(map[string]string)
	for _, match := range packageCLINodePattern.FindAllStringSubmatch(diagram, -1) {
		label := packageCLIUnicodePattern.ReplaceAllStringFunc(match[1], func(encoded string) string {
			value, _ := strconv.ParseUint(encoded[3:len(encoded)-1], 16, 32)
			return string(rune(value))
		})
		nodes[label] = match[2]
	}
	return nodes
}

func packageCLIRequireNodes(t *testing.T, diagram string, names ...string) {
	t.Helper()
	nodes := packageCLINodes(diagram)
	if len(nodes) != len(names) {
		t.Fatalf("got %d packages, want %d (%q):\n%s", len(nodes), len(names), names, diagram)
	}
	for _, name := range names {
		found := false
		for label := range nodes {
			if label == name || strings.HasPrefix(label, name+`\n`) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing package %q:\n%s", name, diagram)
		}
	}
}

func packageCLIRequireEdge(t *testing.T, diagram, from, to string, types, imports int) {
	t.Helper()
	nodes := packageCLINodes(diagram)
	alias := func(name string) string {
		for label, id := range nodes {
			if label == name || strings.HasPrefix(label, name+`\n`) {
				return id
			}
		}
		t.Fatalf("missing endpoint %q:\n%s", name, diagram)
		return ""
	}
	want := fmt.Sprintf("%s --> %s : types=%d; imports=%d", alias(from), alias(to), types, imports)
	if !strings.Contains(diagram, want) {
		t.Fatalf("missing dependency %q:\n%s", want, diagram)
	}
}

func TestPackageCLIFlagValuesAroundTarget(t *testing.T) {
	for _, position := range []string{"before", "after", "equals before", "equals after", "mixed"} {
		t.Run(position, func(t *testing.T) {
			root := t.TempDir()
			writeJava(t, root, "API.java", "package app; class API { domain.Model model; }")
			writeJava(t, root, "Model.java", "package domain; class Model {}")
			cfg := packageCLIConfig(t, root, "")
			output := filepath.Join(root, "docs", "packages.puml")
			flags := []string{"--config", cfg, "--language", "java", "--dependency-source", "declared-types", "--external", "hide", "--cycles", "off", "--focus", "app", "--depth", "1", "--direction", "out", "--relations", "field", "--title", "Package View", "--layout-direction", "left-to-right", "--skinparam", "shadowing=false", "--renderer", "local", "--render-timeout", "1s", "--format", "plantuml", "--output", output}
			args := []string{"package"}
			switch position {
			case "before":
				args = append(args, flags...)
				args = append(args, root)
			case "after":
				args = append(args, root)
				args = append(args, flags...)
			case "equals before", "equals after":
				if position == "equals after" {
					args = append(args, root)
				}
				for i := 0; i < len(flags); i += 2 {
					args = append(args, flags[i]+"="+flags[i+1])
				}
				if position == "equals before" {
					args = append(args, root)
				}
			case "mixed":
				args = append(args, flags[:6]...)
				args = append(args, root)
				args = append(args, flags[6:]...)
			}
			result := packageCLISuccess(t, args...)
			if result.stderr != "" {
				t.Fatalf("unexpected stderr: %s", result.stderr)
			}
			diagram := packageCLIRead(t, output)
			packageCLIRequireNodes(t, diagram, "app", "domain")
			packageCLIRequireEdge(t, diagram, "app", "domain", 1, 0)
			for _, want := range []string{"title Package View", "left to right direction", "skinparam shadowing false"} {
				if !strings.Contains(diagram, want) {
					t.Errorf("missing explicit setting %q:\n%s", want, diagram)
				}
			}
		})
	}
}

func TestPackageCLIConfigPathsAndCWDOverrides(t *testing.T) {
	root := t.TempDir()
	packageCLIChdir(t, root)
	packageCLIWrite(t, filepath.Join(root, "project", "sources", "Configured.java"), "package configured; class Configured {}")
	packageCLIWrite(t, filepath.Join(root, "cli-source", "Override.java"), "package overridden; class Override {}")
	packageCLIWrite(t, filepath.Join(root, "project", "settings.yaml"), "language: java\nsource: [sources]\noutput:\n  file: artifacts/configured.puml\n  format: plantuml\n")
	packageCLISuccess(t, "--config", "project/settings.yaml", "package")
	configuredPath := filepath.Join(root, "project", "artifacts", "configured.puml")
	configured := packageCLIRead(t, configuredPath)
	packageCLIRequireNodes(t, configured, "configured")
	packageCLISuccess(t, "package", "--config=project/settings.yaml", "cli-source", "--output=cli-output.puml")
	packageCLIRequireNodes(t, packageCLIRead(t, filepath.Join(root, "cli-output.puml")), "overridden")
	if got := packageCLIRead(t, configuredPath); got != configured {
		t.Fatal("CLI output override rewrote the configured output")
	}
	if _, err := os.Stat(filepath.Join(root, "project", "cli-output.puml")); !os.IsNotExist(err) {
		t.Fatalf("CLI output was resolved relative to config: %v", err)
	}
}

func TestPackageCLIDefaultOutputPreservesClassDiagram(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprintf("config=%v", configured), func(t *testing.T) {
			root := t.TempDir()
			packageCLIChdir(t, root)
			writeJava(t, root, "Model.java", "package sample; class Model {}")
			if configured {
				packageCLIWrite(t, filepath.Join(root, ".umlgen.yaml"), "source: [.]\noutput:\n  format: plantuml\n")
			}
			packageCLISuccess(t, "class", ".")
			classPath := filepath.Join(root, "class-diagram.puml")
			classDiagram := packageCLIRead(t, classPath)
			if !strings.Contains(classDiagram, `class "Model"`) {
				t.Fatalf("class command did not generate the fixture: %s", classDiagram)
			}
			packageCLISuccess(t, "package", ".")
			packageCLIRequireNodes(t, packageCLIRead(t, filepath.Join(root, "package-diagram.puml")), "sample")
			if packageCLIRead(t, classPath) != classDiagram {
				t.Fatal("package default output overwrote the existing class diagram")
			}
		})
	}
}

func TestPackageCLIConfiguredDependencyModesAndOverrides(t *testing.T) {
	root := t.TempDir()
	packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/modes\n\ngo 1.22\n")
	packageCLIWrite(t, filepath.Join(root, "a", "a.go"), "package a\nimport (\n \"example.test/modes/b\"\n _ \"example.test/remote\"\n)\ntype A struct { B b.B }\n")
	packageCLIWrite(t, filepath.Join(root, "b", "b.go"), "package b\nimport \"example.test/modes/a\"\ntype B struct { A a.A }\n")
	packageCLIWrite(t, filepath.Join(root, "c", "c.go"), "package c\ntype C struct{}\n")
	const a, b, c, remote = "example.test/modes/a", "example.test/modes/b", "example.test/modes/c", "example.test/remote"
	cfg := packageCLIConfig(t, root, "language: go\ndiagrams:\n  package:\n    dependency_source: imports\n    external: show\n    cycles: only\n")
	output := filepath.Join(root, "packages.puml")
	packageCLISuccess(t, "package", root, "--config", cfg, "-o", output)
	configured := packageCLIRead(t, output)
	packageCLIRequireNodes(t, configured, a, b)
	packageCLIRequireEdge(t, configured, a, b, 0, 1)
	packageCLIRequireEdge(t, configured, b, a, 0, 1)
	if strings.Count(configured, "[declared cycle in scope]") != 2 {
		t.Fatalf("configured cycle-only projection lost cycle members:\n%s", configured)
	}
	packageCLISuccess(t, "package", root, "--config", cfg, "-o", output, "--cycles=mark")
	external := packageCLIRead(t, output)
	packageCLIRequireNodes(t, external, a, b, c, remote)
	packageCLIRequireEdge(t, external, a, remote, 0, 1)
	packageCLISuccess(t, "package", root, "--config", cfg, "-o", output, "--dependency-source=declared-types", "--external=hide", "--cycles=off")
	overridden := packageCLIRead(t, output)
	packageCLIRequireNodes(t, overridden, a, b, c)
	packageCLIRequireEdge(t, overridden, a, b, 1, 0)
	packageCLIRequireEdge(t, overridden, b, a, 1, 0)
	if strings.Contains(overridden, "[declared cycle in scope]") {
		t.Fatalf("--cycles=off retained cycle annotations:\n%s", overridden)
	}
	packageCLISuccess(t, "package", root, "--config", cfg, "-o", output, "--dependency-source=both", "--external=show", "--cycles=mark")
	both := packageCLIRead(t, output)
	packageCLIRequireNodes(t, both, a, b, c, remote)
	packageCLIRequireEdge(t, both, a, b, 1, 1)
	packageCLIRequireEdge(t, both, a, remote, 0, 1)
	packageCLIWrite(t, cfg, "language: go\ndiagrams:\n  package:\n    dependency_source: future\n    external: invalid\n    cycles: invalid\n")
	packageCLISuccess(t, "package", root, "--config", cfg, "-o", output, "--dependency-source=declared-types", "--external=hide", "--cycles=off")
	packageCLIRequireNodes(t, packageCLIRead(t, output), a, b, c)
}

func TestPackageCLIExcludeReplacesConfiguredList(t *testing.T) {
	root := t.TempDir()
	packageCLIWrite(t, filepath.Join(root, "drop", "Drop.java"), "package drop; class Drop {}")
	packageCLIWrite(t, filepath.Join(root, "keep", "Keep.java"), "package keep; class Keep {}")
	cfg := packageCLIConfig(t, root, "exclude: [drop]\n")
	output := filepath.Join(root, "packages.puml")
	packageCLISuccess(t, "package", root, "--config", cfg, "-o", output)
	packageCLIRequireNodes(t, packageCLIRead(t, output), "keep")
	packageCLISuccess(t, "package", root, "--config", cfg, "--exclude=keep", "-o", output)
	packageCLIRequireNodes(t, packageCLIRead(t, output), "drop")
}

func TestPackageCLIInvalidOptionsPreserveOutput(t *testing.T) {
	cases := []struct {
		name   string
		config string
		flags  []string
	}{
		{name: "imports relations", flags: []string{"--dependency-source=imports", "--relations=field"}},
		{name: "configured imports relations", config: "diagrams:\n  package:\n    dependency_source: imports\n", flags: []string{"--relations=field"}},
		{name: "hide private", flags: []string{"--hide-private"}},
		{name: "hide fields", flags: []string{"--hide-fields"}},
		{name: "class relations", flags: []string{"--no-relations"}},
		{name: "relation labels", flags: []string{"--show-relation-labels"}},
		{name: "class grouping", flags: []string{"--package-grouping=none"}},
		{name: "class skinparam", flags: []string{"--skinparam=classAttributeIconSize=0"}},
		{name: "sequence entry", flags: []string{"--entry=main"}},
		{name: "activity function", flags: []string{"--function=processOrder"}},
		{name: "state field", flags: []string{"--state-field=status"}},
		{name: "deployment manifest", flags: []string{"--manifest=deployment.yaml"}},
		{name: "misspelled package namespace", config: "diagrams:\n  packages: {}\n"},
		{name: "unknown package setting", config: "diagrams:\n  package:\n    dependency_soruce: imports\n"},
		{name: "invalid dependency source", flags: []string{"--dependency-source=calls"}},
		{name: "invalid external", flags: []string{"--external=all"}},
		{name: "invalid cycles", flags: []string{"--cycles=ignore"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeJava(t, root, "A.java", "package a; class A {}")
			cfg := packageCLIConfig(t, root, test.config)
			output := filepath.Join(root, "existing.puml")
			const existing = "existing diagram must survive\n"
			packageCLIWrite(t, output, existing)
			args := append([]string{"package", root, "--config", cfg, "--output", output}, test.flags...)
			result := packageCLIRun(args...)
			if result.code != exitArgs || result.err == nil {
				t.Fatalf("code=%d err=%v stdout=%s stderr=%s", result.code, result.err, result.stdout, result.stderr)
			}
			if packageCLIRead(t, output) != existing {
				t.Fatal("invalid arguments/configuration modified existing output")
			}
		})
	}
}

func TestPackageCLIFocusDisambiguatesSourceContexts(t *testing.T) {
	for _, language := range []string{"java", "go"} {
		t.Run(language, func(t *testing.T) {
			root := t.TempDir()
			left, right := filepath.Join(root, "left"), filepath.Join(root, "right")
			name := "shared"
			for _, context := range []string{left, right} {
				if language == "java" {
					packageCLIWrite(t, filepath.Join(context, "A.java"), "package shared; class A {}")
				} else {
					name = "example.test/shared"
					packageCLIWrite(t, filepath.Join(context, "go.mod"), "module example.test/shared\n\ngo 1.22\n")
					writeGo(t, context, "a.go", "package shared\ntype A struct{}\n")
				}
			}
			cfg := packageCLIConfig(t, root, fmt.Sprintf("language: %s\nsource:\n  - left\n  - right\n", language))
			output := filepath.Join(root, "packages.puml")
			packageCLISuccess(t, "package", "--config", cfg, "-o", output)
			if len(packageCLINodes(packageCLIRead(t, output))) != 2 {
				t.Fatal("same-name packages from distinct source contexts were merged")
			}
			const existing = "preserve ambiguous-focus output\n"
			packageCLIWrite(t, output, existing)
			result := packageCLIRun("package", "--config", cfg, "--focus", name, "-o", output)
			if result.code != exitArgs || result.err == nil {
				t.Fatalf("ambiguous focus: code=%d err=%v", result.code, result.err)
			}
			for _, context := range []string{left, right} {
				if !strings.Contains(result.err.Error(), context+"::"+name) {
					t.Errorf("ambiguity did not expose canonical selector %q: %v", context+"::"+name, result.err)
				}
			}
			if packageCLIRead(t, output) != existing {
				t.Fatal("ambiguous focus rewrote existing output")
			}
			selected := packageCLISuccess(t, "package", "--config", cfg, "--focus", left+"::"+name, "--depth=0", "--explain", "--quiet", "-o", output)
			packageCLIRequireNodes(t, packageCLIRead(t, output), name)
			if !strings.Contains(selected.stdout, left+"::"+name+" [internal]") || !strings.Contains(selected.stdout, right+"::"+name+" [internal, hidden]") {
				t.Fatalf("full-ID focus lost visible/known-hidden context identity:\n%s", selected.stdout)
			}
		})
	}
}

func TestPackageCLIZeroTypeSources(t *testing.T) {
	t.Run("Go functions and import declarations", func(t *testing.T) {
		root := t.TempDir()
		packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/zero\n\ngo 1.22\n")
		writeGo(t, root, "main.go", "package main\nimport (\n named \"example.test/zero/helper\"\n _ \"example.test/blank\"\n . \"example.test/dot\"\n)\nfunc main() { named.Do() }\n")
		packageCLIWrite(t, filepath.Join(root, "helper", "helper.go"), "package helper\nfunc Do() {}\n")
		cfg := packageCLIConfig(t, root, "language: go\n")
		output := filepath.Join(root, "packages.puml")
		result := packageCLISuccess(t, "package", root, "--config", cfg, "--dependency-source=imports", "--external=show", "--explain", "--quiet", "-o", output)
		diagram := packageCLIRead(t, output)
		packageCLIRequireNodes(t, diagram, "example.test/zero", "example.test/zero/helper", "example.test/blank", "example.test/dot")
		for _, dependency := range []string{"example.test/zero/helper", "example.test/blank", "example.test/dot"} {
			packageCLIRequireEdge(t, diagram, "example.test/zero", dependency, 0, 1)
		}
		for _, alias := range []string{`alias="named"`, `alias="_"`, `alias="."`} {
			if !strings.Contains(result.stdout, alias) {
				t.Errorf("missing original Go import alias %s:\n%s", alias, result.stdout)
			}
		}
		for _, line := range []int{3, 4, 5} {
			if !strings.Contains(result.stdout, fmt.Sprintf("%s:%d:", filepath.Join(root, "main.go"), line)) {
				t.Errorf("missing import span on line %d:\n%s", line, result.stdout)
			}
		}
		packageCLISuccess(t, "package", root, "--config", cfg, "--dependency-source=declared-types", "-o", output)
		declared := packageCLIRead(t, output)
		packageCLIRequireNodes(t, declared, "example.test/zero", "example.test/zero/helper")
		if strings.Contains(declared, " --> ") || strings.Contains(declared, " ..> ") {
			t.Fatalf("function bodies/imports invented declared-type dependencies:\n%s", declared)
		}
	})
	t.Run("Java package only", func(t *testing.T) {
		root := t.TempDir()
		writeJava(t, root, "package-info.java", "package documented;\n")
		cfg := packageCLIConfig(t, root, "language: java\n")
		output := filepath.Join(root, "packages.puml")
		packageCLISuccess(t, "package", root, "--config", cfg, "-o", output)
		packageCLIRequireNodes(t, packageCLIRead(t, output), "documented")
	})
}

func TestPackageCLICacheAndExplanationParity(t *testing.T) {
	for _, language := range []string{"java", "go"} {
		t.Run(language, func(t *testing.T) {
			root := t.TempDir()
			cache := filepath.Join(t.TempDir(), "cache")
			t.Setenv("UMLGEN_CACHE_DIR", cache)
			if language == "java" {
				writeJava(t, root, "A.java", "package a;\nimport b.B;\nclass A { B member; }\n")
				writeJava(t, root, "B.java", "package b; class B {}\n")
			} else {
				packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/cache\n\ngo 1.22\n")
				writeGo(t, root, "a.go", "package a\nimport \"example.test/cache/b\"\ntype A struct { Member b.B }\n")
				packageCLIWrite(t, filepath.Join(root, "b", "b.go"), "package b\ntype B struct{}\n")
			}
			cfg := packageCLIConfig(t, root, "language: "+language+"\n")
			output := filepath.Join(root, "packages.puml")
			args := []string{"package", root, "--config", cfg, "--dependency-source=both", "--quiet", "--explain", "-o", output}
			cold := packageCLISuccess(t, args...)
			diagram := packageCLIRead(t, output)
			entries, err := filepath.Glob(filepath.Join(cache, language, "*.json"))
			if err != nil || len(entries) != 2 {
				t.Fatalf("cold run did not cache the two source files: entries=%v err=%v", entries, err)
			}
			warm := packageCLISuccess(t, args...)
			if got := packageCLIRead(t, output); got != diagram || warm.stdout != cold.stdout {
				t.Fatalf("warm cache changed diagram or explanation\ncold=%s\nwarm=%s", cold.stdout, warm.stdout)
			}
			uncached := packageCLISuccess(t, append(args, "--no-cache")...)
			if got := packageCLIRead(t, output); got != diagram || uncached.stdout != cold.stdout {
				t.Fatalf("--no-cache changed diagram or explanation\ncold=%s\nuncached=%s", cold.stdout, uncached.stdout)
			}
			stats := packageCLISuccess(t, "package", root, "--config", cfg, "--dependency-source=imports", "--external=show", "--cycles=off", "--verbose", "-o", output)
			if !strings.Contains(stats.stdout, "Cache hits: 2, misses: 0") {
				t.Fatalf("package presentation flags invalidated parse cache:\n%s", stats.stdout)
			}
			ordinary := packageCLISuccess(t, "package", root, "--config", cfg, "--dependency-source=both", "--quiet", "-o", output)
			if ordinary.stdout != "" || packageCLIRead(t, output) != diagram {
				t.Fatal("--explain changed the diagram or quiet mode emitted an unsolicited explanation")
			}
		})
	}
}

func TestPackageCLIParseFailuresAndOutputPreservation(t *testing.T) {
	for _, language := range []string{"java", "go"} {
		for _, partial := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/partial=%v", language, partial), func(t *testing.T) {
				root := t.TempDir()
				broken := filepath.Join(root, "Broken.java")
				if language == "java" {
					writeJava(t, root, "Broken.java", "package broken; class Broken {")
					if partial {
						writeJava(t, root, "Good.java", "package good; class Good {}")
					}
				} else {
					broken = filepath.Join(root, "broken.go")
					writeGo(t, root, "broken.go", "package broken\ntype Broken struct {\n")
					if partial {
						packageCLIWrite(t, filepath.Join(root, "good", "good.go"), "package good\ntype Good struct{}\n")
					}
				}
				cfg := packageCLIConfig(t, root, "language: "+language+"\n")
				output := filepath.Join(root, "packages.puml")
				const existing = "retain previous diagram on total failure\n"
				packageCLIWrite(t, output, existing)
				result := packageCLIRun("package", root, "--config", cfg, "--explain", "-o", output)
				if partial {
					if result.code != exitOK || result.err != nil {
						t.Fatalf("partial parse: code=%d err=%v stderr=%s", result.code, result.err, result.stderr)
					}
					if got := packageCLIRead(t, output); got == existing || len(packageCLINodes(got)) == 0 {
						t.Fatalf("partial parse discarded successful package analysis:\n%s", got)
					}
					if !strings.Contains(result.stdout, "1 warning(s)") {
						t.Fatalf("partial parse did not report its failure count:\n%s", result.stdout)
					}
				} else {
					if result.code != exitParse || result.err == nil {
						t.Fatalf("all-failed parse: code=%d err=%v", result.code, result.err)
					}
					if packageCLIRead(t, output) != existing {
						t.Fatal("all-failed parse overwrote the previous diagram")
					}
				}
				if !strings.Contains(result.stderr, broken) || !strings.Contains(result.stdout, broken) || !strings.Contains(result.stdout, "syntax-error") {
					t.Fatalf("failure source/diagnostic was lost: stdout=%s stderr=%s", result.stdout, result.stderr)
				}
			})
		}
	}
}

func TestPackageCLIExplainEvidenceAndKnownHiddenIdentity(t *testing.T) {
	root := t.TempDir()
	writeJava(t, root, "A.java", "package app.a;\nimport app.b.B;\nclass A {\n    B shown;\n    Missing missing;\n    other.c.C hidden;\n}\n")
	writeJava(t, root, "B.java", "package app.b; class B {}")
	writeJava(t, root, "C.java", "package other.c; class C {}")
	cfg := packageCLIConfig(t, root, "")
	output := filepath.Join(root, "packages.puml")
	result := packageCLISuccess(t, "package", root, "--config", cfg, "--dependency-source=both", "--include=app", "--explain", "--quiet", "-o", output)
	diagram := packageCLIRead(t, output)
	packageCLIRequireNodes(t, diagram, "app.a", "app.b")
	packageCLIRequireEdge(t, diagram, "app.a", "app.b", 1, 1)
	for _, evidence := range []string{
		"app.a.A -> app.b.B [field",
		filepath.Join(root, "A.java") + ":4",
		filepath.Join(root, "A.java") + ":2:",
		"4 |     B shown;",
		"2 | import app.b.B;",
		root + "::other.c [internal, hidden]",
		root + "::app.a -> java " + root + "::other.c [hidden]",
		`reference="Missing"`,
	} {
		if !strings.Contains(result.stdout, evidence) {
			t.Errorf("missing actual evidence/identity %q:\n%s", evidence, result.stdout)
		}
	}
	if strings.Contains(result.stdout, `reference="other.c.C"`) || strings.Contains(result.stdout, "Declared-type relation: app.a.A -> other.c.C") {
		t.Fatalf("known hidden target was treated as unresolved or explained as visible:\n%s", result.stdout)
	}
}

func TestPackageCLIRenderFailureRetainsPlantUMLWithoutImplicitServer(t *testing.T) {
	for _, mode := range []string{"auto unavailable", "local failure"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "local failure" && runtime.GOOS == "windows" {
				t.Skip("mock shell script is not executable on Windows")
			}
			root := t.TempDir()
			writeJava(t, root, "A.java", "package a; class A {}")
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				http.Error(w, "must not use server implicitly", http.StatusInternalServerError)
			}))
			defer server.Close()
			cfg := packageCLIConfig(t, root, fmt.Sprintf("renderer:\n  type: auto\n  server_url: %q\n", server.URL))
			t.Setenv("PLANTUML_SERVER", server.URL)
			t.Setenv("PLANTUML_JAR", "")
			bin := t.TempDir()
			t.Setenv("PATH", bin)
			args := []string{"package", root, "--config", cfg, "--format=png", "--output", filepath.Join(root, "packages.png")}
			if mode == "local failure" {
				if err := os.WriteFile(filepath.Join(bin, "plantuml"), []byte("#!/bin/sh\nprintf 'intentional package renderer failure\\n' >&2\nexit 7\n"), 0o755); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--renderer=local")
			}
			result := packageCLIRun(args...)
			if result.code != exitRender || result.err != nil {
				t.Fatalf("code=%d err=%v stderr=%s", result.code, result.err, result.stderr)
			}
			packageCLIRequireNodes(t, packageCLIRead(t, filepath.Join(root, "packages.puml")), "a")
			if count := requests.Load(); count != 0 {
				t.Fatalf("render failure implicitly sent PlantUML to a server (%d requests)", count)
			}
			if _, err := os.Stat(filepath.Join(root, "packages.png")); !os.IsNotExist(err) {
				t.Fatalf("failed renderer produced an image: %v", err)
			}
			if mode == "local failure" && !strings.Contains(result.stderr, "intentional package renderer failure") {
				t.Fatalf("renderer diagnostic was lost: %s", result.stderr)
			}
		})
	}
}

func TestPackageCLICommandNamedConfigPath(t *testing.T) {
	root := t.TempDir()
	packageCLIChdir(t, root)
	packageCLIWrite(t, "package", "language: java\nsource: [src]\n")
	packageCLIWrite(t, filepath.Join(root, "src", "Model.java"), "package sample; class Model {}")
	packageCLISuccess(t, "--config", "package", "package", "--no-cache")
	packageCLIRequireNodes(t, packageCLIRead(t, "package-diagram.puml"), "sample")
}

// These syntax-only examples are also CLI inputs for the package documentation.
// Deliberate cycles, ambiguous names and absent dependencies must not be mistaken
// for compiler-checked projects or silently turned into guessed graph edges.
func TestPackageCLIRepresentativeFixtures(t *testing.T) {
	type dependency struct {
		from, to string
		types    int
		imports  int
	}
	for _, language := range []string{"java", "go"} {
		t.Run(language, func(t *testing.T) {
			root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "package", language))
			if err != nil {
				t.Fatal(err)
			}
			work := t.TempDir()
			t.Setenv("UMLGEN_CACHE_DIR", filepath.Join(work, "cache"))
			cfg := packageCLIConfig(t, work, "language: "+language+"\n")
			output := filepath.Join(work, "packages.puml")
			prefix := "sample."
			if language == "go" {
				prefix = "example.test/packagefixture/"
			}
			internal := []string{prefix + "app", prefix + "domain", prefix + "service", prefix + "alternate", prefix + "bootstrap"}
			for _, mode := range []string{"declared-types", "imports", "both"} {
				t.Run(mode, func(t *testing.T) {
					args := []string{"package", root, "--config", cfg, "--dependency-source=" + mode, "--external=show", "--cycles=mark", "--no-cache", "--quiet", "--explain", "-o", output}
					result := packageCLISuccess(t, args...)
					diagram := packageCLIRead(t, output)
					nodes := append([]string(nil), internal...)
					edges := []dependency{
						{prefix + "domain", prefix + "service", 1, 1},
						{prefix + "service", prefix + "domain", 1, 1},
					}
					if language == "java" {
						edges = append(edges, dependency{prefix + "app", prefix + "service", 1, 1})
						if mode != "declared-types" {
							edges = append(edges,
								dependency{prefix + "app", prefix + "domain", 0, 1},
								dependency{prefix + "app", prefix + "alternate", 0, 1},
								dependency{prefix + "bootstrap", prefix + "service", 0, 1},
							)
						}
					} else {
						edges = append(edges,
							dependency{prefix + "app", prefix + "domain", 1, 1},
							dependency{prefix + "app", prefix + "alternate", 1, 1},
						)
						if mode != "declared-types" {
							nodes = append(nodes, "example.test/telemetry")
							edges = append(edges,
								dependency{prefix + "bootstrap", prefix + "service", 0, 1},
								dependency{prefix + "bootstrap", "example.test/telemetry", 0, 1},
							)
						}
					}
					packageCLIRequireNodes(t, diagram, nodes...)
					if got := strings.Count(diagram, " --> ") + strings.Count(diagram, " ..> "); got != len(edges) {
						t.Fatalf("got %d edges, want %d:\n%s", got, len(edges), diagram)
					}
					for _, edge := range edges {
						types, imports := edge.types, edge.imports
						if mode == "imports" {
							types = 0
						}
						if mode == "declared-types" {
							imports = 0
						}
						packageCLIRequireEdge(t, diagram, edge.from, edge.to, types, imports)
					}
					for label := range packageCLINodes(diagram) {
						wantCycle := strings.HasPrefix(label, prefix+"domain"+`\n`) || strings.HasPrefix(label, prefix+"service"+`\n`)
						if gotCycle := strings.Contains(label, "[declared cycle in scope]"); gotCycle != wantCycle {
							t.Errorf("unexpected cycle membership for %q:\n%s", label, diagram)
						}
					}
					if strings.Count(diagram, "[declared cycle in scope]") != 2 {
						t.Errorf("expected exactly the domain/service cycle:\n%s", diagram)
					}
					if !strings.Contains(result.stdout, fmt.Sprintf("Visible diagram: %d packages, %d dependencies.", len(nodes), len(edges))) {
						t.Errorf("explanation disagrees with diagram counts:\n%s", result.stdout)
					}
					var evidence []string
					if mode != "imports" {
						evidence = append(evidence, `reference="Missing"`)
						if language == "java" {
							evidence = append(evidence,
								"sample.app.App -> sample.service.Service [field",
								filepath.Join(root, "app", "App.java")+":9",
								"9 |     Service service;",
								`, ambiguous] reference="Item"`, "type=sample.domain.Item", "type=sample.alternate.Item",
							)
						} else {
							evidence = append(evidence,
								prefix+"app.App -> "+prefix+"domain.Item [field",
								prefix+"app.Backup -> "+prefix+"alternate.Item [field",
								filepath.Join(root, "app", "app.go")+":7",
								filepath.Join(root, "app", "backup.go")+":7",
								"7 | \tItem model.Item",
							)
						}
					}
					if mode != "declared-types" {
						if language == "java" {
							evidence = append(evidence,
								filepath.Join(root, "bootstrap", "package-info.java")+":4:",
								"4 | import sample.service.Service;",
								`reference="missing.remote.Client"`,
							)
						} else {
							evidence = append(evidence,
								filepath.Join(root, "bootstrap", "bootstrap.go")+":5:",
								"5 | \t_ \"example.test/packagefixture/service\"",
								`alias="model"`, `alias="_"`,
							)
						}
					}
					for _, want := range evidence {
						if !strings.Contains(result.stdout, want) {
							t.Errorf("missing fixture evidence %q:\n%s", want, result.stdout)
						}
					}
					if language == "go" && mode != "imports" {
						for _, wrong := range []string{
							prefix + "app.App -> " + prefix + "alternate.Item",
							prefix + "app.Backup -> " + prefix + "domain.Item",
						} {
							if strings.Contains(result.stdout, wrong) {
								t.Errorf("import alias escaped its source file: %s\n%s", wrong, result.stdout)
							}
						}
					}
				})
			}
			t.Run("cycle-only projection", func(t *testing.T) {
				result := packageCLISuccess(t, "package", root, "--config", cfg, "--dependency-source=both", "--cycles=only", "--external=show", "--no-cache", "--quiet", "--explain", "-o", output)
				diagram := packageCLIRead(t, output)
				packageCLIRequireNodes(t, diagram, prefix+"domain", prefix+"service")
				packageCLIRequireEdge(t, diagram, prefix+"domain", prefix+"service", 1, 1)
				packageCLIRequireEdge(t, diagram, prefix+"service", prefix+"domain", 1, 1)
				if strings.Count(diagram, " --> ") != 2 || !strings.Contains(result.stdout, "Visible diagram: 2 packages, 2 dependencies.") {
					t.Fatalf("cycle-only projection retained non-cycle dependencies:\n%s\n%s", diagram, result.stdout)
				}
			})
		})
	}
}
