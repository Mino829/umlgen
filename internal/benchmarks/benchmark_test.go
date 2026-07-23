package benchmarks_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/java"
	"github.com/Mino829/umlgen/internal/javacache"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/relations"
	"github.com/Mino829/umlgen/internal/scanner"
)

func BenchmarkScanJavaFiles(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprintf("files_%d", count), func(b *testing.B) {
			root := writeJavaFiles(b, count)
			b.ResetTimer()
			for range b.N {
				files, err := scanner.JavaFiles([]string{root}, nil)
				if err != nil || len(files) != count {
					b.Fatalf("files=%d err=%v", len(files), err)
				}
			}
			b.ReportMetric(float64(count), "files/op")
		})
	}
}

func BenchmarkParseJavaSource(b *testing.B) {
	for _, count := range []int{10, 100} {
		b.Run(fmt.Sprintf("types_%d", count), func(b *testing.B) {
			source := javaSource(count)
			b.SetBytes(int64(len(source)))
			b.ResetTimer()
			for range b.N {
				types, err := java.ParseSource("Benchmark.java", source)
				if err != nil || len(types) != count {
					b.Fatalf("types=%d err=%v", len(types), err)
				}
			}
		})
	}
}

func BenchmarkResolveRelations(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprintf("types_%d", count), func(b *testing.B) {
			types := modelTypes(count)
			b.ResetTimer()
			for range b.N {
				got := relations.Build(types)
				if len(got) != count {
					b.Fatalf("relations=%d", len(got))
				}
			}
			b.ReportMetric(float64(count), "types/op")
		})
	}
}

func BenchmarkGeneratePlantUML(b *testing.B) {
	options := plantuml.Options{
		ShowFields: true, ShowMethods: true, ShowPrivate: true, ShowPublic: true,
		ShowProtected: true, ShowPackage: true, ShowRelations: true,
		Inheritance: true, Implementation: true, FieldDependency: true,
		ParamDependency: true, ReturnDependency: true,
	}
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprintf("types_%d", count), func(b *testing.B) {
			project := model.Project{Types: modelTypes(count)}
			b.ResetTimer()
			for range b.N {
				if got := plantuml.Generate(project, options); got == "" {
					b.Fatal("empty PlantUML")
				}
			}
			b.ReportMetric(float64(count), "types/op")
		})
	}
}

func BenchmarkWarmJavaCache(b *testing.B) {
	b.Setenv("UMLGEN_CACHE_DIR", filepath.Join(b.TempDir(), "cache"))
	sourcePath := filepath.Join(b.TempDir(), "Benchmark.java")
	source := javaSource(100)
	if err := os.WriteFile(sourcePath, source, 0o600); err != nil {
		b.Fatal(err)
	}
	cache, err := javacache.Open("benchmark", map[string]string{"language": "java"})
	if err != nil {
		b.Fatal(err)
	}
	if result, err := cache.ParseFile(sourcePath); err != nil || result.Hit {
		b.Fatalf("prime result=%#v err=%v", result, err)
	}
	b.ResetTimer()
	for range b.N {
		result, err := cache.ParseFile(sourcePath)
		if err != nil || !result.Hit || len(result.Types) != 100 {
			b.Fatalf("result=%#v err=%v", result, err)
		}
	}
}

func writeJavaFiles(b *testing.B, count int) string {
	b.Helper()
	root := b.TempDir()
	for i := range count {
		source := fmt.Sprintf("package bench; class Type%d { Type%d next; }", i, (i+1)%count)
		path := filepath.Join(root, fmt.Sprintf("Type%04d.java", i))
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	return root
}

func javaSource(count int) []byte {
	var source strings.Builder
	source.WriteString("package bench;\n")
	for i := range count {
		fmt.Fprintf(&source, "class Type%d { Type%d next; }\n", i, (i+1)%count)
	}
	return []byte(source.String())
}

func modelTypes(count int) []model.Type {
	types := make([]model.Type, count)
	for i := range count {
		types[i] = model.Type{
			Package: "bench",
			Name:    fmt.Sprintf("Type%d", i),
			Kind:    model.Class,
			Fields: []model.Field{{
				Name:       "next",
				Type:       fmt.Sprintf("Type%d", (i+1)%count),
				Visibility: model.Private,
			}},
		}
	}
	return types
}
