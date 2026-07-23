package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassAcceptsOptionsAfterTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "User.java"), []byte(
		"package sample; public class User { private String name; public String getName(){ return name; } }",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "docs", "diagram.puml")
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", src, "--output", out, "--hide-private"}, &stdout, &stderr)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "-name: String") {
		t.Fatalf("private field was not hidden:\n%s", data)
	}
}

func TestInvalidFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", ".", "--format", "pdf"}, &stdout, &stderr)
	if code != exitArgs || err == nil || !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestVerboseQuietConflict(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := Run([]string{"class", ".", "--verbose", "--quiet"}, &stdout, &stderr)
	if code != exitArgs || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}
