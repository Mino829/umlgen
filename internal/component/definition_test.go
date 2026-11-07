package component

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeBoundaryDefinition(t *testing.T, dir, text string) string {
	t.Helper()
	path := filepath.Join(dir, "boundaries.yaml")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefinitionStrictSchema(t *testing.T) {
	valid := "version: 1\ncomponents:\n  - name: API\n    match:\n      - language: go\n        go_module: example.test/app\n"
	cases := map[string]string{
		"empty":                     "",
		"root list":                 "[]",
		"unknown root":              valid + "unknown: true\n",
		"duplicate root":            valid + "version: 1\n",
		"quoted version":            strings.Replace(valid, "version: 1", "version: '1'", 1),
		"wrong version":             strings.Replace(valid, "version: 1", "version: 2", 1),
		"boolean name":              strings.Replace(valid, "name: API", "name: true", 1),
		"numeric module":            strings.Replace(valid, "example.test/app", "123", 1),
		"null match":                "version: 1\ncomponents: [{name: API, match: null}]\n",
		"mapping match":             "version: 1\ncomponents: [{name: API, match: {language: go}}]\n",
		"scalar row":                "version: 1\ncomponents: [{name: API, match: [go]}]\n",
		"empty rows":                "version: 1\ncomponents: [{name: API, match: []}]\n",
		"unknown selector":          strings.Replace(valid, "go_module:", "module:", 1),
		"duplicate selector":        valid + "        language: java\n",
		"duplicate component field": valid + "    name: Other\n",
		"duplicate component":       valid + "  - name: API\n    match: [{language: java, source_root: .}]\n",
		"missing language":          strings.Replace(valid, "      - language: go\n        go_module:", "      - go_module:", 1),
		"unsupported language":      strings.Replace(valid, "language: go", "language: python", 1),
		"java module":               strings.Replace(valid, "language: go", "language: java", 1),
		"empty prefix":              valid + "        package_prefix: ''\n",
		"glob prefix":               valid + "        package_prefix: example.test/*\n",
		"regex prefix":              valid + "        package_prefix: '^example'\n",
		"root prefix trick":         valid + "        package_prefix: /\n",
		"contracts mapping":         valid + "    provides: {Orders: true}\n",
		"contracts boolean":         valid + "    requires: [true]\n",
		"duplicate contracts":       valid + "    provides: [Orders, Orders]\n",
		"empty contract":            valid + "    requires: ['']\n",
		"extra document":            valid + "---\n{}\n",
		"alias selector":            "version: 1\ncomponents: [{name: API, match: [&s {language: go, source_root: .}, *s]}]\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			d, err := LoadDefinition(writeBoundaryDefinition(t, t.TempDir(), text))
			if !errors.Is(err, ErrBoundary) {
				t.Fatalf("expected boundary error, got %v", err)
			}
			if !reflect.DeepEqual(d, Definition{}) {
				t.Fatalf("failure returned partial definition: %#v", d)
			}
		})
	}
}

func TestLoadDefinitionProvenanceAndPhysicalRoots(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "physical")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "alias")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("source-root symlink fixture unavailable: %v", err)
	}
	text := "version: 1\ncomponents:\n  - name: API\n    match:\n      - language: go\n        source_root: ./alias\n      - language: java\n        source_root: alias/missing/../unused\n    provides: [Orders]\n    requires: [Orders]\n"
	path := writeBoundaryDefinition(t, dir, text)
	d, err := LoadDefinition(path)
	if err != nil {
		t.Fatal(err)
	}
	expectedRoot, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	if d.Source != path || !filepath.IsAbs(d.Source) {
		t.Fatalf("source = %q", d.Source)
	}
	hash := sha256.Sum256([]byte(text))
	if d.Hash != hex.EncodeToString(hash[:]) {
		t.Fatalf("hash = %q", d.Hash)
	}
	b := d.Components[0]
	if b.Line != 3 || b.Match[0].Line != 5 || b.Match[1].Line != 7 {
		t.Fatalf("missing YAML provenance: %#v", b)
	}
	if b.Match[0].SourceRoot != expectedRoot || b.Match[1].SourceRoot != filepath.Join(expectedRoot, "unused") {
		t.Fatalf("roots = %#v", b.Match)
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("loaded definition fails manual validation: %v", err)
	}
	// Loading again must not share mutable backing storage with the first value.
	d.Components[0].Match[0].Language = "changed"
	next, err := LoadDefinition(path)
	if err != nil {
		t.Fatal(err)
	}
	if next.Components[0].Match[0].Language != "go" {
		t.Fatal("load shares mutable state")
	}
}

func TestDefinitionDisplayContractsAreInert(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	payload := "$(touch " + marker + ")"
	text := "version: 1\ncomponents:\n  - name: '`echo architecture`'\n    match: [{language: java, source_root: .}]\n    provides: ['" + payload + "']\n    requires: ['" + payload + "']\n"
	_, err := LoadDefinition(writeBoundaryDefinition(t, dir, text))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("display contract executed: %v", err)
	}
}

func TestManualDefinitionValidation(t *testing.T) {
	valid := func() Definition {
		return Definition{Version: 1, Components: []Boundary{{Name: "Root", Match: []Selector{{Language: "java", SourceRoot: "."}}}}}
	}
	if err := valid().Validate(); err != nil {
		t.Fatalf("default package context selector rejected: %v", err)
	}
	cases := map[string]func(*Definition){
		"version":              func(d *Definition) { d.Version = 0 },
		"components":           func(d *Definition) { d.Components = nil },
		"name":                 func(d *Definition) { d.Components[0].Name = " " },
		"duplicate":            func(d *Definition) { d.Components = append(d.Components, d.Components[0]) },
		"rows":                 func(d *Definition) { d.Components[0].Match = nil },
		"constraint":           func(d *Definition) { d.Components[0].Match[0].SourceRoot = "" },
		"language":             func(d *Definition) { d.Components[0].Match[0].Language = "" },
		"java module":          func(d *Definition) { d.Components[0].Match[0].GoModule = "example.test/app" },
		"glob":                 func(d *Definition) { d.Components[0].Match[0].PackagePrefix = "com.*" },
		"empty prefix segment": func(d *Definition) { d.Components[0].Match[0].PackagePrefix = "com..app" },
		"blank root":           func(d *Definition) { d.Components[0].Match[0].SourceRoot = " " },
		"contracts":            func(d *Definition) { d.Components[0].Requires = []string{"Storage", "Storage"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := valid()
			mutate(&d)
			if err := d.Validate(); !errors.Is(err, ErrBoundary) {
				t.Fatalf("expected boundary error, got %v", err)
			}
		})
	}
}
