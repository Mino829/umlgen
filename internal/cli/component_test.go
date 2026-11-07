package cli

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func componentFixture(t *testing.T, language, root string) string {
	t.Helper()
	prefix := "sample"
	separator := "."
	if language == "go" {
		prefix = "example.test/architecture"
		separator = "/"
		packageCLIWrite(t, filepath.Join(root, "go.mod"), "module "+prefix+"\n\ngo 1.25\n")
		packageCLIWrite(t, filepath.Join(root, "api", "api.go"), `package api
import (
 "example.test/architecture/domain"
 "example.test/architecture/store"
 "example.test/architecture/api/support"
 _ "fmt"
)
type Handler struct { Order domain.Order; Store store.Repository; Check support.Checker }
`)
		packageCLIWrite(t, filepath.Join(root, "api", "support", "support.go"), "package support\ntype Checker struct{}\n")
		packageCLIWrite(t, filepath.Join(root, "api2", "api.go"), "package api2\ntype Sibling struct{}\n")
		packageCLIWrite(t, filepath.Join(root, "domain", "order.go"), "package domain\ntype Order struct{}\n")
		packageCLIWrite(t, filepath.Join(root, "store", "store.go"), "package store\nimport \"example.test/architecture/domain\"\ntype Repository struct { Last domain.Order }\n")
		packageCLIWrite(t, filepath.Join(root, "orphan", "orphan.go"), "package orphan\nimport \"example.test/architecture/store\"\ntype Orphan struct { Store store.Repository }\n")
	} else {
		packageCLIWrite(t, filepath.Join(root, "api", "Handler.java"), `package sample.api;
import sample.domain.Order;
import sample.store.Repository;
import sample.api.support.Checker;
import java.util.List;
class Handler { Order order; Repository store; Checker check; List<Order> orders; }
`)
		packageCLIWrite(t, filepath.Join(root, "api", "support", "Checker.java"), "package sample.api.support; class Checker {}")
		packageCLIWrite(t, filepath.Join(root, "api2", "Sibling.java"), "package sample.api2; class Sibling {}")
		packageCLIWrite(t, filepath.Join(root, "domain", "Order.java"), "package sample.domain; class Order {}")
		packageCLIWrite(t, filepath.Join(root, "store", "Repository.java"), "package sample.store; import sample.domain.Order; class Repository { Order last; }")
		packageCLIWrite(t, filepath.Join(root, "orphan", "Orphan.java"), "package sample.orphan; import sample.store.Repository; class Orphan { Repository store; }")
	}
	definition := filepath.Join(root, "architecture.yaml")
	text := "version: 1\ncomponents:\n"
	for _, row := range []struct{ name, part string }{{"API", "api"}, {"Domain", "domain"}, {"Storage", "store"}} {
		text += fmt.Sprintf("  - name: %s\n    match:\n      - language: %s\n        package_prefix: %s%s%s\n", row.name, language, prefix, separator, row.part)
		if language == "go" {
			text += "        go_module: " + prefix + "\n"
		}
		if row.name == "API" {
			text += "    provides: [Orders]\n    requires: [Storage]\n"
		}
		if row.name == "Storage" {
			text += "    provides: [Storage]\n"
		}
	}
	packageCLIWrite(t, definition, text)
	return definition
}

func TestComponentCLIPreservesOwnershipEvidenceAndCacheParity(t *testing.T) {
	for _, language := range []string{"go", "java"} {
		t.Run(language, func(t *testing.T) {
			root := t.TempDir()
			definition := componentFixture(t, language, root)
			output := filepath.Join(t.TempDir(), "component.puml")
			args := []string{"component", root, "--language", language, "--boundary-config", definition, "--quiet", "--explain", "-o", output}
			cold := packageCLISuccess(t, args...)
			diagram := packageCLIRead(t, output)
			for _, args := range [][]string{args, append(args, "--no-cache")} {
				warm := packageCLISuccess(t, args...)
				if diagram != packageCLIRead(t, output) || cold.stdout != warm.stdout {
					t.Fatal("syntax cache changed component ownership/evidence")
				}
			}
			for _, evidence := range []string{"API [component]; 2 source package identities", "Internal package dependencies", "Original package edge:", "Type relation records:", "Import declaration:", "api2 [unassigned]", "orphan [unassigned]", "Hidden external dependencies"} {
				if !strings.Contains(cold.stdout, evidence) {
					t.Errorf("lost boundary/source evidence %q:\n%s", evidence, cold.stdout)
				}
			}
			// Equal configured API names must not produce a provider binding.
			contracts := regexp.MustCompile(`(?m)^interface "Storage" as (\S+)$`).FindAllStringSubmatch(diagram, -1)
			if len(contracts) != 2 || contracts[0][1] == contracts[1][1] {
				t.Fatal("equal contract names merged provider/requirement identities")
			}
			for _, line := range strings.Split(diagram, "\n") {
				if strings.Contains(line, contracts[0][1]) && strings.Contains(line, contracts[1][1]) {
					t.Fatal("equal interface names created an inferred provider binding")
				}
			}
			for _, flags := range [][]string{{"--focus", "API"}, {"--include", "sample"}, {"--cycles", "only"}, {"--relations", "field"}, {"--dependency-source", "invalid"}} {
				r := packageCLIRun(append(args, flags...)...)
				if r.code != exitArgs || r.err == nil || packageCLIRead(t, output) != diagram {
					t.Fatalf("unsupported controls overwrote diagram: %v %+v", flags, r)
				}
			}
			// A broad component and a narrower one are an error, never precedence.
			text := packageCLIRead(t, definition)
			prefix := "sample.api.support"
			if language == "go" {
				prefix = "example.test/architecture/api/support"
			}
			packageCLIWrite(t, definition, text+fmt.Sprintf("  - name: Shadow\n    match: [{language: %s, package_prefix: %s}]\n", language, prefix))
			r := packageCLIRun(args...)
			if r.code != exitArgs || r.err == nil || packageCLIRead(t, output) != diagram {
				t.Fatalf("ambiguous ownership selected a winner: %+v", r)
			}
		})
	}
}

func TestComponentCLIConfigRelativePathsAndPartialFailure(t *testing.T) {
	root := t.TempDir()
	componentFixture(t, "go", root)
	configPath := filepath.Join(root, ".umlgen.yaml")
	packageCLIWrite(t, configPath, "language: go\nsource: [.]\noutput:\n  file: diagrams/components.puml\ndiagrams:\n  component:\n    boundary_config: architecture.yaml\n    dependency_source: invalid\n")
	args := []string{"component", "--config", configPath, "--dependency-source", "both", "--quiet", "--explain"}
	packageCLISuccess(t, args...)
	output := filepath.Join(root, "diagrams", "components.puml")
	packageCLIWrite(t, filepath.Join(root, "broken.go"), "package broken\nfunc Broken( {\n")
	r := packageCLIRun(args...)
	if r.code != exitParse || r.err == nil || !strings.Contains(r.stdout, "Source failure:") || !strings.Contains(packageCLIRead(t, output), "@enduml") {
		t.Fatalf("partial parsing became proof of complete architecture: %+v", r)
	}
}

func TestComponentCLIAllFailedAnalysisPreservesDiagram(t *testing.T) {
	root := t.TempDir()
	definition := filepath.Join(root, "architecture.yaml")
	packageCLIWrite(t, definition, "version: 1\ncomponents:\n  - name: App\n    match: [{language: go, package_prefix: example.test/app}]\n")
	packageCLIWrite(t, filepath.Join(root, "broken.go"), "package broken\nfunc Broken( {\n")
	output := filepath.Join(t.TempDir(), "component.puml")
	packageCLIWrite(t, output, "previous valid diagram")
	r := packageCLIRun("component", root, "--language", "go", "--boundary-config", definition, "--quiet", "-o", output)
	if r.code != exitParse || r.err == nil || packageCLIRead(t, output) != "previous valid diagram" {
		t.Fatalf("all-failed analysis overwrote prior output: %+v", r)
	}
}

func TestComponentCLIPhysicalContextIncludesDefaultPackage(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "source [root]")
	packageCLIWrite(t, filepath.Join(root, "App.java"), "class App {}")
	definition := filepath.Join(dir, "architecture.yaml")
	packageCLIWrite(t, definition, "version: 1\ncomponents:\n  - name: DefaultApp\n    match: [{language: java, source_root: './source [root]'}]\n")
	r := packageCLISuccess(t, "component", root, "--language", "java", "--boundary-config", definition, "--quiet", "--explain", "-o", filepath.Join(dir, "components.puml"))
	if !strings.Contains(r.stdout, "DefaultApp [component]; 1 source package identities") || strings.Contains(r.stderr, "unassigned-package") {
		t.Fatalf("default package/literal root lost ownership: %+v", r)
	}
}

func TestComponentCachedSourcesRebindChangedGoModule(t *testing.T) {
	root := t.TempDir()
	definition := componentFixture(t, "go", root)
	args := []string{"component", root, "--language", "go", "--boundary-config", definition, "--quiet", "--explain", "-o", filepath.Join(t.TempDir(), "components.puml")}
	packageCLISuccess(t, args...)
	packageCLIWrite(t, filepath.Join(root, "go.mod"), "module example.test/renamed\n\ngo 1.25\n")
	warm := packageCLISuccess(t, args...)
	if !strings.Contains(warm.stdout, "API [component]; 0 source package identities") || !strings.Contains(warm.stderr, "unassigned-package") {
		t.Fatalf("changed module retained cached component ownership: %+v", warm)
	}
	uncached := packageCLISuccess(t, append(args, "--no-cache")...)
	if warm.stdout != uncached.stdout {
		t.Fatal("module metadata and cached syntax disagree about boundary ownership")
	}
}
