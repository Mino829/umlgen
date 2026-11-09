package renderer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The test executable doubles as plantuml/java so command failures are tested
// through Render on every platform without requiring shell scripts or Java.
func TestMain(m *testing.M) {
	if os.Getenv("UMLGEN_RENDERER_HELPER") == "1" {
		name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
		mode := os.Getenv("UMLGEN_RENDERER_HELPER_" + strings.ToUpper(name))
		switch mode {
		case "failure":
			fmt.Fprintln(os.Stderr, "Syntax error at line 3")
			os.Exit(7)
		case "missing":
			os.Exit(0)
		case "timeout":
			fmt.Fprintln(os.Stderr, "renderer started")
			time.Sleep(time.Minute)
			os.Exit(0)
		case "success", "empty":
			format := "svg"
			if os.Args[len(os.Args)-2] == "-tpng" {
				format = "png"
			}
			var image []byte
			if mode == "success" {
				image = []byte("mock image")
			}
			if err := os.WriteFile(outputPath(os.Args[len(os.Args)-1], format), image, 0o644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(11)
			}
			os.Exit(0)
		default:
			fmt.Fprintln(os.Stderr, "unexpected renderer helper mode:", mode)
			os.Exit(12)
		}
	}
	os.Exit(m.Run())
}

func installRendererHelpers(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		source, err := os.Open(executable)
		if err != nil {
			t.Fatal(err)
		}
		destination, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o755)
		if err != nil {
			source.Close()
			t.Fatal(err)
		}
		_, copyErr := io.Copy(destination, source)
		source.Close()
		closeErr := destination.Close()
		if copyErr != nil {
			t.Fatal(copyErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	return dir
}

func TestRenderAutoAvailableCommandFailure(t *testing.T) {
	dir := installRendererHelpers(t, "plantuml")
	t.Setenv("PATH", dir)
	t.Setenv("PLANTUML_JAR", "")
	t.Setenv("UMLGEN_RENDERER_HELPER", "1")
	for _, tc := range []struct {
		mode string
		want string
	}{
		{"failure", "Syntax error at line 3"},
		{"missing", "did not produce output file"},
		{"empty", "empty output file"},
		{"timeout", "timed out"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv("UMLGEN_RENDERER_HELPER_PLANTUML", tc.mode)
			path := filepath.Join(t.TempDir(), "diagram.puml")
			source := "@startuml\nclass A\n@enduml\n"
			if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg := Config{Type: TypeAuto, Timeout: 5 * time.Second}
			if tc.mode == "timeout" {
				cfg.Timeout = time.Second
			}
			_, err := Render("svg", path, cfg)
			if err == nil || !strings.Contains(err.Error(), "PlantUML renderer failed") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected renderer failure containing %q, got: %v", tc.want, err)
			}
			if strings.Contains(err.Error(), "renderer was not found") || strings.Contains(err.Error(), "Install the plantuml command") {
				t.Fatalf("available command failure was reported as unavailable: %v", err)
			}
			switch tc.mode {
			case "failure":
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
					t.Fatalf("exit status cause was lost: %v", err)
				}
			case "missing":
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("missing output cause was lost: %v", err)
				}
			case "timeout":
				if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "renderer started") {
					t.Fatalf("timeout cause or stderr was lost: %v", err)
				}
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil || string(data) != source {
				t.Fatalf("input was not preserved: data=%q err=%v", data, readErr)
			}
		})
	}
}

func TestRenderAutoJarFallback(t *testing.T) {
	available := installRendererHelpers(t, "plantuml", "java")
	missing := installRendererHelpers(t, "java")
	t.Setenv("UMLGEN_RENDERER_HELPER", "1")
	t.Setenv("UMLGEN_RENDERER_HELPER_PLANTUML", "failure")
	for _, local := range []string{"available", "missing"} {
		for _, jar := range []string{"success", "failure"} {
			t.Run(local+"/"+jar, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "diagram.puml")
				if err := os.WriteFile(path, []byte("@startuml\n@enduml\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				commandDir := available
				if local == "missing" {
					commandDir = missing
				}
				t.Setenv("PATH", commandDir)
				t.Setenv("PLANTUML_JAR", filepath.Join(filepath.Dir(path), "plantuml.jar"))
				t.Setenv("UMLGEN_RENDERER_HELPER_JAVA", jar)
				out, err := Render("png", path, Config{Type: TypeAuto, Timeout: 5 * time.Second})
				if jar == "success" {
					if err != nil {
						t.Fatal(err)
					}
					data, readErr := os.ReadFile(out)
					if readErr != nil || string(data) != "mock image" {
						t.Fatalf("unexpected fallback output: data=%q err=%v", data, readErr)
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), "local command:") || !strings.Contains(err.Error(), "PLANTUML_JAR:") || !strings.Contains(err.Error(), "Syntax error at line 3") {
					t.Fatalf("fallback failure lost renderer diagnostics: %v", err)
				}
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
					t.Fatalf("fallback exit cause was lost: %v", err)
				}
				if local == "missing" && !errors.Is(err, exec.ErrNotFound) {
					t.Fatalf("local lookup cause was lost: %v", err)
				}
				if strings.Contains(err.Error(), "Install the plantuml command") {
					t.Fatalf("fallback execution failure recommended installation: %v", err)
				}
			})
		}
	}
}
