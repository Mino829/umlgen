package config

import (
	"os"
	"path/filepath"
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
