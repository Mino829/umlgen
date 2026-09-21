package config

import (
	"os"
	"path/filepath"
	"testing"
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
