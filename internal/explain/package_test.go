package explain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mino829/umlgen/internal/language"
	"github.com/Mino829/umlgen/internal/packagegraph"
)

func TestPackageExplanationUsesCapturedSnapshotsAfterSourceChanges(t *testing.T) {
	root := t.TempDir()
	parser, err := language.Select("java", []string{root}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string]string{
		"A.java":      "package app;\nimport dep.Target;\nclass A {\n Target field;\n Missing unresolved;\n}\n",
		"Target.java": "package dep; class Target {}\n",
	}
	report := PackageReport{Sources: make(map[string]Source)}
	var sources []packagegraph.Source
	for name, content := range contents {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		unit, err := parser.ParseSource(path, []byte(content))
		if err != nil {
			t.Fatal(err)
		}
		parser.BindContext(unit)
		sources = append(sources, packagegraph.Source{Context: root, Unit: unit})
		report.Sources[path] = Source{Content: []byte(content), Origin: "working tree"}
	}
	report.Known, err = packagegraph.Build(sources, packagegraph.Both)
	if err != nil {
		t.Fatal(err)
	}
	report.View, err = packagegraph.Analyze(report.Known, packagegraph.ViewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	before := FormatPackage(report)
	requireContains(t, before, "4 |  Target field;", "2 | import dep.Target;", "5 |  Missing unresolved;", `reference="Missing"`)
	if err := os.WriteFile(filepath.Join(root, "A.java"), []byte("changed after analysis\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "Target.java")); err != nil {
		t.Fatal(err)
	}
	if after := FormatPackage(report); after != before {
		t.Fatalf("package explanation reread changed/deleted source files:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}
