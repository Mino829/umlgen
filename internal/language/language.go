package language

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mino829/umlgen/internal/golang"
	"github.com/Mino829/umlgen/internal/java"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/scanner"
)

type Parser interface {
	Name() string
	DisplayName() string
	Extensions() []string
	MatchesPath(string) bool
	CacheSchema() string
	ParseFile(string) ([]model.Type, error)
	ParseSource(string, []byte) ([]model.Type, error)
	Finalize([]model.Type) []model.Type
}

type javaParser struct{}
type goParser struct{}

func ForName(name string) (Parser, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "java":
		return javaParser{}, nil
	case "go":
		return goParser{}, nil
	default:
		return nil, fmt.Errorf("unsupported language %q; supported languages: auto, java, go", name)
	}
}

func Select(configured string, targets, excludes, changedPaths []string) (Parser, error) {
	configured = strings.ToLower(strings.TrimSpace(configured))
	if configured != "" && configured != "auto" {
		return ForName(configured)
	}
	name, err := detect(targets, excludes, changedPaths)
	if err != nil {
		return nil, err
	}
	return ForName(name)
}

func detect(targets, excludes, changedPaths []string) (string, error) {
	if len(changedPaths) > 0 {
		if detected, ok, err := detectPaths(changedPaths); ok || err != nil {
			return detected, err
		}
	}
	files, err := scanner.SourceFiles(targets, excludes, ".java", ".go")
	if err != nil {
		return "", err
	}
	if detected, ok, detectErr := detectPaths(files); ok && detectErr == nil {
		return detected, detectErr
	} else if detectErr != nil {
		if marker := detectProjectMarker(targets); marker != "" {
			return marker, nil
		}
		return "", detectErr
	}
	if marker := detectProjectMarker(targets); marker != "" {
		return marker, nil
	}
	return "", errorsForDetection()
}

func detectPaths(paths []string) (string, bool, error) {
	javaFound, goFound := false, false
	for _, path := range paths {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".java":
			javaFound = true
		case ".go":
			if !strings.HasSuffix(strings.ToLower(path), "_test.go") {
				goFound = true
			}
		}
	}
	switch {
	case javaFound && goFound:
		return "", true, fmt.Errorf("both Java and Go sources were found; select one with --language java or --language go")
	case javaFound:
		return "java", true, nil
	case goFound:
		return "go", true, nil
	default:
		return "", false, nil
	}
}

func detectProjectMarker(targets []string) string {
	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			continue
		}
		directory := target
		if !info.IsDir() {
			directory = filepath.Dir(target)
		}
		absolute, err := filepath.Abs(directory)
		if err == nil {
			directory = absolute
		}
		for current := directory; ; current = filepath.Dir(current) {
			if exists(filepath.Join(current, "go.mod")) {
				return "go"
			}
			if exists(filepath.Join(current, "pom.xml")) ||
				exists(filepath.Join(current, "build.gradle")) ||
				exists(filepath.Join(current, "build.gradle.kts")) {
				return "java"
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
		}
	}
	return ""
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func errorsForDetection() error {
	return fmt.Errorf("no Java or Go source files found; select a source directory or set --language")
}

func (javaParser) Name() string         { return "java" }
func (javaParser) DisplayName() string  { return "Java" }
func (javaParser) Extensions() []string { return []string{".java"} }
func (javaParser) MatchesPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".java")
}
func (javaParser) CacheSchema() string { return "java-tree-sitter-v1" }
func (javaParser) ParseFile(path string) ([]model.Type, error) {
	return java.ParseFile(path)
}
func (javaParser) ParseSource(path string, source []byte) ([]model.Type, error) {
	return java.ParseSource(path, source)
}
func (javaParser) Finalize(types []model.Type) []model.Type {
	java.SortTypes(types)
	return types
}

func (goParser) Name() string         { return "go" }
func (goParser) DisplayName() string  { return "Go" }
func (goParser) Extensions() []string { return []string{".go"} }
func (goParser) MatchesPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".go") && !strings.HasSuffix(lower, "_test.go")
}
func (goParser) CacheSchema() string { return golang.SchemaVersion }
func (goParser) ParseFile(path string) ([]model.Type, error) {
	return golang.ParseFile(path)
}
func (goParser) ParseSource(path string, source []byte) ([]model.Type, error) {
	return golang.ParseSource(path, source)
}
func (goParser) Finalize(types []model.Type) []model.Type {
	return golang.Finalize(types)
}
