package renderer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Type != TypeAuto {
		t.Fatalf("default type = %q", cfg.Type)
	}
	if cfg.Timeout == 0 {
		t.Fatal("default timeout is zero")
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		cfg    Config
		wantOK bool
	}{
		{Defaults(), true},
		{Config{Type: TypeLocal}, true},
		{Config{Type: TypeJar}, true},
		{Config{Type: TypeServer, ServerURL: "http://example.com"}, true},
		{Config{Type: TypeServer}, false},
		{Config{Type: "unknown"}, false},
	} {
		err := tc.cfg.Validate()
		if tc.wantOK && err != nil {
			t.Errorf("%#v: unexpected error: %v", tc.cfg, err)
		}
		if !tc.wantOK && err == nil {
			t.Errorf("%#v: expected error", tc.cfg)
		}
	}
}

func TestRenderUnsupportedFormat(t *testing.T) {
	_, err := Render("gif", "", Defaults())
	if err == nil || !strings.Contains(err.Error(), "unsupported render format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEncodePlantUMLCharset(t *testing.T) {
	encoded, err := encodePlantUML([]byte("@startuml\nA -> B\n@enduml"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(encoded); i++ {
		c := encoded[i]
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '-' || c == '_') {
			t.Fatalf("invalid character %q at position %d in %q", c, i, encoded)
		}
	}
}

func TestRenderLocalCommandNotFound(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("PLANTUML_JAR", "")
	_, err := Render("svg", "diagram.puml", Defaults())
	if err == nil || !strings.Contains(err.Error(), "PlantUML renderer was not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderLocalSVG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock shell script is not executable on Windows")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "plantuml")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"mock\" > \""+dir+"/diagram.svg\""), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	pumlPath := filepath.Join(dir, "diagram.puml")
	if err := os.WriteFile(pumlPath, []byte("@startuml\n@enduml"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := Render("svg", pumlPath, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if out != filepath.Join(dir, "diagram.svg") {
		t.Fatalf("output path = %q", out)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "mock" {
		t.Fatalf("unexpected output: %s", data)
	}
}

func TestRenderJar(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mock shell script is not executable on Windows")
	}
	dir := t.TempDir()
	jar := filepath.Join(dir, "plantuml.jar")
	if err := os.WriteFile(jar, []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	java := filepath.Join(dir, "java")
	if err := os.WriteFile(java, []byte("#!/bin/sh\necho \"mock\" > \""+dir+"/diagram.png\""), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("PLANTUML_JAR", jar)

	pumlPath := filepath.Join(dir, "diagram.puml")
	if err := os.WriteFile(pumlPath, []byte("@startuml\n@enduml"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := Render("png", pumlPath, Config{Type: TypeJar})
	if err != nil {
		t.Fatal(err)
	}
	if out != filepath.Join(dir, "diagram.png") {
		t.Fatalf("output path = %q", out)
	}
}

func TestRenderServerRequiresURL(t *testing.T) {
	_, err := Render("svg", "diagram.puml", Config{Type: TypeServer})
	if err == nil || !strings.Contains(err.Error(), "server_url") {
		t.Fatalf("unexpected error: %v", err)
	}
}
