package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLoadAndResolvePaths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".umlgen.yaml")
	input := `language: java
source:
  - src/main/java
exclude:
  - target
output:
  file: docs/model.puml
  format: plantuml
visibility:
  private: false
members:
  methods: false
`
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, loaded, err := Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	ResolvePaths(&cfg, loaded)
	if cfg.Source[0] != filepath.Join(dir, "src/main/java") {
		t.Fatalf("source was not resolved: %q", cfg.Source[0])
	}
	if cfg.Output.File != filepath.Join(dir, "docs/model.puml") {
		t.Fatalf("output was not resolved: %q", cfg.Output.File)
	}
	if cfg.Visibility.Private || cfg.Members.Methods {
		t.Fatalf("booleans were not loaded: %#v", cfg)
	}
}

func TestMissingImplicitConfigUsesDefaults(t *testing.T) {
	cfg, loaded, err := Load(filepath.Join(t.TempDir(), "missing.yaml"), false)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != "" || cfg.Output.File != "class-diagram.puml" || cfg.Language != "auto" {
		t.Fatalf("unexpected defaults: loaded=%q cfg=%#v", loaded, cfg)
	}
}

func TestLoadSupportsGoLanguage(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".umlgen.yaml")
	if err := os.WriteFile(path, []byte("language: go\nsource:\n  - .\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Language != "go" {
		t.Fatalf("language = %q", cfg.Language)
	}
}

func TestLoadRejectsUnsupportedLanguage(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".umlgen.yaml")
	if err := os.WriteFile(path, []byte("language: ruby\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(path, true); err == nil {
		t.Fatal("expected unsupported language error")
	}
}

func TestLoadReadsSnakeCaseKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".umlgen.yaml")
	input := `visibility:
  package_private: false
relations:
  field_dependency: false
  parameter_dependency: false
  return_dependency: false
renderer:
  type: server
  server_url: http://example.com:8080
  timeout: 45s
`
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Visibility.PackagePrivate {
		t.Fatal("package_private was ignored")
	}
	if cfg.Relations.FieldDependency || cfg.Relations.ParameterDependency || cfg.Relations.ReturnDependency {
		t.Fatalf("relations flags were ignored: %#v", cfg.Relations)
	}
	if cfg.Renderer.ServerURL != "http://example.com:8080" {
		t.Fatalf("server_url = %q", cfg.Renderer.ServerURL)
	}
	if cfg.Renderer.Timeout != 45*time.Second {
		t.Fatalf("timeout = %v", cfg.Renderer.Timeout)
	}
}

func TestLoadNormalizesRendererType(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".umlgen.yaml")
	if err := os.WriteFile(path, []byte("renderer:\n  type: SERVER\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Renderer.Type != "server" {
		t.Fatalf("renderer type = %q", cfg.Renderer.Type)
	}
}

func TestLoadPartialLayout(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		packageGrouping string
		direction       string
		skinParams      map[string]string
	}{
		{
			name:            "grouping",
			input:           "layout:\n  package_grouping: hierarchical\n",
			packageGrouping: "hierarchical",
			direction:       "top-to-bottom",
		},
		{
			name:            "direction",
			input:           "layout:\n  direction: left-to-right\n",
			packageGrouping: "flat",
			direction:       "left-to-right",
		},
		{
			name: "skinparams",
			input: `layout:
  skinparams:
    linetype: ortho
    shadowing: false
    classAttributeIconSize: 0
`,
			packageGrouping: "flat",
			direction:       "top-to-bottom",
			skinParams: map[string]string{
				"linetype":               "ortho",
				"shadowing":              "false",
				"classAttributeIconSize": "0",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".umlgen.yaml")
			if err := os.WriteFile(path, []byte(tt.input), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, _, err := Load(path, true)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Layout.PackageGrouping != tt.packageGrouping || cfg.Layout.Direction != tt.direction {
				t.Fatalf("layout = %#v", cfg.Layout)
			}
			if !reflect.DeepEqual(cfg.Layout.SkinParams, tt.skinParams) {
				t.Fatalf("skinparams = %#v, want %#v", cfg.Layout.SkinParams, tt.skinParams)
			}
		})
	}
}

func TestLoadRejectsLayoutYAMLTypeErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"layout scalar", "layout: flat\n"},
		{"skinparam mapping", "layout:\n  skinparams:\n    linetype: {value: ortho}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".umlgen.yaml")
			if err := os.WriteFile(path, []byte(tt.input), 0o644); err != nil {
				t.Fatal(err)
			}
			_, _, err := Load(path, true)
			if err == nil {
				t.Fatal("expected YAML type error")
			}
		})
	}
}

func TestLoadPackageDiagramDefaultsAndOverrides(t *testing.T) {
	defaults := PackageDiagramConfig{DependencySource: "declared-types", External: "hide", Cycles: "mark"}
	tests := []struct {
		name  string
		input string
		want  PackageDiagramConfig
	}{
		{"omitted", "language: auto\n", defaults},
		{"empty diagrams", "diagrams: {}\n", defaults},
		{"empty package", "diagrams:\n  package: {}\n", defaults},
		{
			"dependency source only",
			"diagrams:\n  package:\n    dependency_source: imports\n",
			PackageDiagramConfig{DependencySource: "imports", External: "hide", Cycles: "mark"},
		},
		{
			"external only",
			"diagrams:\n  package:\n    external: show\n",
			PackageDiagramConfig{DependencySource: "declared-types", External: "show", Cycles: "mark"},
		},
		{
			"cycles only",
			"diagrams:\n  package:\n    cycles: only\n",
			PackageDiagramConfig{DependencySource: "declared-types", External: "hide", Cycles: "only"},
		},
		{
			"normalized strings",
			"diagrams:\n  package:\n    dependency_source: ' BOTH '\n    external: ' SHOW '\n    cycles: ' OFF '\n",
			PackageDiagramConfig{DependencySource: "both", External: "show", Cycles: "off"},
		},
		{
			"unsupported values deferred to command",
			"diagrams:\n  package:\n    dependency_source: ' FUTURE '\n    external: mystery\n    cycles: ''\n",
			PackageDiagramConfig{DependencySource: "future", External: "mystery", Cycles: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := loadConfigYAML(t, tt.input)
			if cfg.Diagrams.Package != tt.want {
				t.Fatalf("package settings = %#v, want %#v", cfg.Diagrams.Package, tt.want)
			}
			if cfg.Output.File != "class-diagram.puml" || cfg.Output.FileSet {
				t.Fatalf("package settings changed class output defaults: %#v", cfg.Output)
			}
		})
	}
}

func TestLoadRejectsInvalidDiagramYAML(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"unsupported namespace", "diagrams:\n  class: {}\n"},
		{"unknown component setting", "diagrams:\n  component:\n    package: app\n"},
		{"component scalar", "diagrams:\n  component: imports\n"},
		{"component non-string path", "diagrams:\n  component:\n    boundary_config: [architecture.yaml]\n"},
		{"unknown package setting", "diagrams:\n  package:\n    focus: example\n"},
		{"misspelled package setting", "diagrams:\n  package:\n    dependency_sources: imports\n"},
		{"diagrams scalar", "diagrams: package\n"},
		{"diagrams sequence", "diagrams: [package]\n"},
		{"diagrams null", "diagrams: null\n"},
		{"package scalar", "diagrams:\n  package: imports\n"},
		{"package sequence", "diagrams:\n  package: [imports]\n"},
		{"package null", "diagrams:\n  package: null\n"},
		{"dependency source integer", "diagrams:\n  package:\n    dependency_source: 1\n"},
		{"external boolean", "diagrams:\n  package:\n    external: false\n"},
		{"cycles null", "diagrams:\n  package:\n    cycles: null\n"},
		{"cycles sequence", "diagrams:\n  package:\n    cycles: [mark]\n"},
		{"external mapping", "diagrams:\n  package:\n    external: {mode: show}\n"},
		{"duplicate namespace", "diagrams:\n  package: {}\n  package: {}\n"},
		{"duplicate package setting", "diagrams:\n  package:\n    cycles: mark\n    cycles: off\n"},
		{"duplicate diagrams", "diagrams: {}\ndiagrams: {}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".umlgen.yaml")
			if err := os.WriteFile(path, []byte(tt.input), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, _, err := Load(path, true); err == nil {
				t.Fatal("expected invalid diagram YAML to be rejected")
			}
		})
	}
}

func TestLoadPreservesLegacyUnknownKeys(t *testing.T) {
	cfg, _ := loadConfigYAML(t, `legacy_extension:
  anything: [one, two]
visibility:
  private: false
  unknown_visibility: true
output:
  format: svg
  unknown_output: unused
layout:
  direction: left-to-right
  unknown_layout: unused
diagrams:
  package:
    external: show
`)
	if cfg.Visibility.Private || cfg.Output.Format != "svg" || cfg.Layout.Direction != "left-to-right" {
		t.Fatalf("legacy settings were not preserved: %#v", cfg)
	}
	if cfg.Diagrams.Package.External != "show" {
		t.Fatalf("package settings = %#v", cfg.Diagrams.Package)
	}
}

func TestLoadOutputFilePresence(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		fileSet bool
	}{
		{"omitted output", "language: auto\n", "class-diagram.puml", false},
		{"empty output", "output: {}\n", "class-diagram.puml", false},
		{"null output", "output: null\n", "class-diagram.puml", false},
		{"omitted file", "output:\n  format: svg\n", "class-diagram.puml", false},
		{"null file", "output:\n  file: null\n", "class-diagram.puml", false},
		{"implicit null file", "output:\n  file:\n", "class-diagram.puml", false},
		{"empty file", "output:\n  file: ''\n", "", true},
		{"explicit default file", "output:\n  file: class-diagram.puml\n", "class-diagram.puml", true},
		{"relative file", "output:\n  file: docs/packages.puml\n", "docs/packages.puml", true},
		{"aliased null file", "unused: &unset null\noutput:\n  file: *unset\n", "class-diagram.puml", false},
		{"aliased empty file", "unused: &empty ''\noutput:\n  file: *empty\n", "", true},
		{"merged file", "unused: &output\n  file: shared/model.puml\noutput:\n  <<: *output\n", "shared/model.puml", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, path := loadConfigYAML(t, tt.input)
			if cfg.Output.File != tt.want || cfg.Output.FileSet != tt.fileSet {
				t.Fatalf("output = %#v, want file %q, explicit %v", cfg.Output, tt.want, tt.fileSet)
			}
			ResolvePaths(&cfg, path)
			want := tt.want
			if want != "" {
				want = filepath.Join(filepath.Dir(path), want)
			}
			if cfg.Output.File != want || cfg.Output.FileSet != tt.fileSet {
				t.Fatalf("resolved output = %#v, want file %q, explicit %v", cfg.Output, want, tt.fileSet)
			}
		})
	}
	if cfg := Defaults(); cfg.Output.FileSet {
		t.Fatal("default output must not be explicitly set")
	}
}

func TestResolvePathsPreservesAbsolutePaths(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	output := filepath.Join(dir, "docs", "packages.puml")
	cfg := Defaults()
	cfg.Source = []string{source, "relative"}
	cfg.Output.File = output
	cfg.Output.FileSet = true
	configPath := filepath.Join(dir, "settings", ".umlgen.yaml")
	ResolvePaths(&cfg, configPath)
	if cfg.Source[0] != source || cfg.Source[1] != filepath.Join(dir, "settings", "relative") {
		t.Fatalf("resolved sources = %#v", cfg.Source)
	}
	if cfg.Output.File != output || !cfg.Output.FileSet {
		t.Fatalf("absolute output changed: %#v", cfg.Output)
	}
}

func TestTemplateLeavesCommandOutputDefaultsAvailable(t *testing.T) {
	cfg, path := loadConfigYAML(t, Template)
	if cfg.Output.FileSet || cfg.Output.File != "class-diagram.puml" || cfg.Output.Format != "plantuml" {
		t.Fatalf("template pins or changes output defaults: %#v", cfg.Output)
	}
	if cfg.Diagrams.Package != Defaults().Diagrams.Package {
		t.Fatalf("template package settings = %#v", cfg.Diagrams.Package)
	}
	if !reflect.DeepEqual(cfg.Source, []string{"."}) {
		t.Fatalf("template sources = %#v", cfg.Source)
	}
	class := cfg
	ResolvePaths(&class, path)
	if class.Output.File != filepath.Join(filepath.Dir(path), "class-diagram.puml") {
		t.Fatalf("class output = %q", class.Output.File)
	}
	// A package command can select its own default before config-relative resolution.
	packages := cfg
	if !packages.Output.FileSet {
		packages.Output.File = "package-diagram.puml"
	}
	ResolvePaths(&packages, path)
	if packages.Output.File != filepath.Join(filepath.Dir(path), "package-diagram.puml") {
		t.Fatalf("package output = %q", packages.Output.File)
	}
}

func loadConfigYAML(t *testing.T, input string) (Config, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".umlgen.yaml")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, loaded, err := Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != path {
		t.Fatalf("loaded path = %q, want %q", loaded, path)
	}
	return cfg, loaded
}
