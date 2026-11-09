package scanner

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestJavaFilesRecursesAndExcludes(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{
		"src/main/User.java",
		"src/main/readme.txt",
		"src/test/UserTest.java",
		"target/Generated.java",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("class X {}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := JavaFiles([]string{root}, []string{"src/test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "User.java" {
		t.Fatalf("unexpected files: %#v", files)
	}
}

func TestSourceFilesSupportsGo(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"main.go", "service_test.go", "User.java", "vendor/dependency.go"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package sample"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := SourceFiles([]string{root}, nil, ".go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %#v", files)
	}
}

func TestSourceFilesReportsPathResolutionErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses a different absolute-path implementation")
	}
	const helperTargetEnv = "UMLGEN_SCANNER_CWD_TARGET"
	if target := os.Getenv(helperTargetEnv); target != "" {
		t.Setenv("PWD", "")
		// Darwin can still resolve a deleted cwd. A sufficiently deep cwd
		// instead exceeds Getwd's path lookup limits while relative I/O works.
		component := strings.Repeat("d", 32)
		depth := 0
		defer func() {
			_ = os.Remove("Source.java")
			for ; depth > 0; depth-- {
				if err := os.Chdir(".."); err != nil {
					t.Error(err)
					return
				}
				if err := os.Remove(component); err != nil {
					t.Error(err)
					return
				}
			}
		}()
		var lookupErr error
		for depth < 512 {
			if err := os.Mkdir(component, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(component); err != nil {
				t.Fatal(err)
			}
			depth++
			if depth%32 == 0 {
				if _, lookupErr = filepath.Abs("Source.java"); lookupErr != nil {
					break
				}
			}
		}
		if lookupErr == nil {
			t.Fatal("fixture did not reach an absolute-path lookup failure")
		}
		if err := os.WriteFile("Source.java", []byte("class Source {}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("relative target must remain accessible: %v", err)
		}
		files, err := SourceFiles([]string{target}, nil, ".java")
		if err == nil || errors.Unwrap(err) == nil {
			t.Fatalf("files = %#v, error = %v; want a wrapped path resolution error", files, err)
		}
		if files != nil {
			t.Fatalf("failed scan returned source paths: %#v", files)
		}
		return
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"Source.java", "."} {
		t.Run(target, func(t *testing.T) {
			cmd := exec.Command(executable, "-test.run=^TestSourceFilesReportsPathResolutionErrors$")
			cmd.Dir = t.TempDir()
			cmd.Env = append(os.Environ(), helperTargetEnv+"="+target)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("scanner helper failed: %v\n%s", err, output)
			}
		})
	}
}
