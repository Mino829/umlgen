package benchmarks_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mino829/umlgen/internal/language"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/plantuml"
)

// Package fixtures have ten types per file/package. Each package imports the
// next package in a ring, with one imported-type field per type. Thus 100/1000
// types produce 10/100 files, packages, directed edges and import declarations,
// with 100/1000 type relations. The ring is one declared SCC, intentionally a
// source-model workload rather than a compilable Go build.
// Existing class fixtures remain unchanged so their historical baselines hold.
const packageBenchmarkTypesPerPackage = 10
const packageBenchmarkModule = "example.com/umlgenbench"

type packageBenchmarkFile struct {
	path   string
	source []byte
}

type packageBenchmarkFixture struct {
	parser  language.Parser
	root    string
	files   []packageBenchmarkFile
	sources []packagegraph.Source
	graph   packagegraph.Graph
	types   int
	bytes   int64
}

// BenchmarkPackageParseContext parses the entire in-memory corpus and binds
// each SourceUnit through the same language adapter as the CLI. Go context
// binding includes go.mod discovery; scanning, source-file reads, cache I/O,
// graph construction and Go interface inference are not timed here.
func BenchmarkPackageParseContext(b *testing.B) {
	runPackageBenchmarkFixtures(b, func(b *testing.B, fixture packageBenchmarkFixture) {
		b.SetBytes(fixture.bytes)
		var sources []packagegraph.Source
		b.ResetTimer()
		for range b.N {
			sources = fixture.parse(b)
		}
		b.StopTimer()
		if len(sources) != len(fixture.files) {
			b.Fatalf("source units=%d, want %d", len(sources), len(fixture.files))
		}
		fixture.report(b)
	})
}

// BenchmarkPackageBuildResolve includes source snapshot preparation, indexing,
// file-scoped import/type resolution, evidence aggregation and graph ordering.
// Parsed, context-bound units are prepared before every subbenchmark's timer.
func BenchmarkPackageBuildResolve(b *testing.B) {
	runPackageBenchmarkFixtures(b, func(b *testing.B, fixture packageBenchmarkFixture) {
		for _, mode := range []packagegraph.DependencySource{packagegraph.DeclaredTypes, packagegraph.Imports, packagegraph.Both} {
			b.Run(string(mode), func(b *testing.B) {
				var graph packagegraph.Graph
				b.ResetTimer()
				for range b.N {
					var err error
					graph, err = packagegraph.Build(fixture.sources, mode)
					if err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				fixture.checkGraph(b, graph, mode)
				fixture.report(b)
			})
		}
	})
}

// BenchmarkPackageProjection separates projection without SCC detection, full
// SCC analysis, and SCC analysis followed by depth-two bidirectional focus.
// Focus shows five packages while cycle classification retains the full ring.
func BenchmarkPackageProjection(b *testing.B) {
	runPackageBenchmarkFixtures(b, func(b *testing.B, fixture packageBenchmarkFixture) {
		focus := fixture.graph.Packages[0].ID
		for _, scenario := range []struct {
			name string
			opts packagegraph.ViewOptions
		}{
			{"all_cycles_off", packagegraph.ViewOptions{Cycles: packagegraph.CyclesOff}},
			{"all_scc", packagegraph.ViewOptions{Cycles: packagegraph.CyclesMark}},
			{"focus_scc", packagegraph.ViewOptions{Cycles: packagegraph.CyclesMark, Focus: &focus, Depth: 2, Direction: packagegraph.DirectionBoth}},
		} {
			b.Run(scenario.name, func(b *testing.B) {
				var view packagegraph.View
				b.ResetTimer()
				for range b.N {
					var err error
					view, err = packagegraph.Analyze(fixture.graph, scenario.opts)
					if err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				fixture.checkView(b, view, scenario.opts)
				fixture.report(b)
				reportPackageBenchmarkView(b, view)
			})
		}
	})
}

// BenchmarkGeneratePackagePlantUML renders an already analyzed view: neither
// graph construction nor SCC/focus analysis is charged to PlantUML generation.
func BenchmarkGeneratePackagePlantUML(b *testing.B) {
	runPackageBenchmarkFixtures(b, func(b *testing.B, fixture packageBenchmarkFixture) {
		focus := fixture.graph.Packages[0].ID
		for _, focused := range []bool{false, true} {
			name := "all_scc"
			opts := packagegraph.ViewOptions{Cycles: packagegraph.CyclesMark}
			if focused {
				name = "focus_scc"
				opts.Focus, opts.Depth, opts.Direction = &focus, 2, packagegraph.DirectionBoth
			}
			b.Run(name, func(b *testing.B) {
				view, err := packagegraph.Analyze(fixture.graph, opts)
				if err != nil {
					b.Fatal(err)
				}
				fixture.checkView(b, view, opts)
				var output string
				b.ResetTimer()
				for range b.N {
					output, err = plantuml.GeneratePackage(view, plantuml.PackageOptions{})
					if err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				if output == "" {
					b.Fatal("empty package PlantUML")
				}
				fixture.report(b)
				reportPackageBenchmarkView(b, view)
				b.ReportMetric(float64(len(output)), "output-bytes/op")
			})
		}
	})
}

func runPackageBenchmarkFixtures(b *testing.B, run func(*testing.B, packageBenchmarkFixture)) {
	b.Helper()
	for _, name := range []string{"java", "go"} {
		b.Run(name, func(b *testing.B) {
			for _, count := range []int{100, 1000} {
				b.Run(fmt.Sprintf("types_%d", count), func(b *testing.B) {
					run(b, newPackageBenchmarkFixture(b, name, count))
				})
			}
		})
	}
}

func newPackageBenchmarkFixture(b *testing.B, name string, count int) packageBenchmarkFixture {
	b.Helper()
	parser, err := language.ForName(name)
	if err != nil {
		b.Fatal(err)
	}
	fixture := packageBenchmarkFixture{parser: parser, root: b.TempDir(), types: count}
	if name == "go" {
		if err := os.WriteFile(filepath.Join(fixture.root, "go.mod"), []byte("module "+packageBenchmarkModule+"\n\ngo 1.23\n"), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	packages := count / packageBenchmarkTypesPerPackage
	for pkg := range packages {
		directory := filepath.Join(fixture.root, fmt.Sprintf("p%03d", pkg))
		if err := os.MkdirAll(directory, 0o700); err != nil {
			b.Fatal(err)
		}
		next := (pkg + 1) % packages
		var source strings.Builder
		fileName := "types.go"
		if name == "java" {
			fileName = "Types.java"
			fmt.Fprintf(&source, "package bench.p%03d;\nimport bench.p%03d.*;\n", pkg, next)
		} else {
			fmt.Fprintf(&source, "package p%03d\nimport next \"%s/p%03d\"\n", pkg, packageBenchmarkModule, next)
		}
		for local := range packageBenchmarkTypesPerPackage {
			if name == "java" {
				index := pkg*packageBenchmarkTypesPerPackage + local
				target := next*packageBenchmarkTypesPerPackage + local
				fmt.Fprintf(&source, "class Type%04d { Type%04d next; }\n", index, target)
			} else {
				fmt.Fprintf(&source, "type Type%02d struct { Next *next.Type%02d }\n", local, local)
			}
		}
		file := packageBenchmarkFile{path: filepath.Join(directory, fileName), source: []byte(source.String())}
		if err := os.WriteFile(file.path, file.source, 0o600); err != nil {
			b.Fatal(err)
		}
		fixture.files = append(fixture.files, file)
		fixture.bytes += int64(len(file.source))
	}
	fixture.sources = fixture.parse(b)
	fixture.graph, err = packagegraph.Build(fixture.sources, packagegraph.Both)
	if err != nil {
		b.Fatal(err)
	}
	fixture.checkGraph(b, fixture.graph, packagegraph.Both)
	return fixture
}

func (fixture packageBenchmarkFixture) parse(b *testing.B) []packagegraph.Source {
	b.Helper()
	sources := make([]packagegraph.Source, 0, len(fixture.files))
	for _, file := range fixture.files {
		unit, err := fixture.parser.ParseSource(file.path, file.source)
		if err != nil {
			b.Fatal(err)
		}
		fixture.parser.BindContext(unit)
		if len(unit.Types) != packageBenchmarkTypesPerPackage {
			b.Fatalf("%s: types=%d, want %d", file.path, len(unit.Types), packageBenchmarkTypesPerPackage)
		}
		sources = append(sources, packagegraph.Source{Context: fixture.root, Unit: unit})
	}
	return sources
}

func (fixture packageBenchmarkFixture) checkGraph(b *testing.B, graph packagegraph.Graph, mode packagegraph.DependencySource) {
	b.Helper()
	packages := fixture.types / packageBenchmarkTypesPerPackage
	if len(graph.Packages) != packages || len(graph.Dependencies) != packages || len(graph.Diagnostics) != 0 {
		b.Fatalf("packages=%d edges=%d diagnostics=%d, want %d/%d/0", len(graph.Packages), len(graph.Dependencies), len(graph.Diagnostics), packages, packages)
	}
	var imports, relations, files int
	for _, pkg := range graph.Packages {
		files += len(pkg.Files)
	}
	for _, edge := range graph.Dependencies {
		imports += len(edge.Imports)
		relations += len(edge.TypeRelations)
	}
	wantImports, wantRelations := packages, fixture.types
	if mode == packagegraph.DeclaredTypes {
		wantImports = 0
	}
	if mode == packagegraph.Imports {
		wantRelations = 0
	}
	if files != len(fixture.files) || imports != wantImports || relations != wantRelations {
		b.Fatalf("files=%d imports=%d type relations=%d, want %d/%d/%d", files, imports, relations, len(fixture.files), wantImports, wantRelations)
	}
}

func (fixture packageBenchmarkFixture) checkView(b *testing.B, view packagegraph.View, opts packagegraph.ViewOptions) {
	b.Helper()
	packages := len(fixture.graph.Packages)
	visible := packages
	edges := packages
	if opts.Focus != nil {
		visible = 5
		edges = 4
	}
	cycles := 1
	if opts.Cycles == packagegraph.CyclesOff {
		cycles = 0
	}
	if len(view.Graph.Packages) != visible || len(view.Graph.Dependencies) != edges || len(view.Cycles) != cycles || view.Scope.PackageCount != packages || view.Scope.DependencyCount != packages {
		b.Fatalf("unexpected package projection: visible=%d/%d cycles=%d scope=%d/%d", len(view.Graph.Packages), len(view.Graph.Dependencies), len(view.Cycles), view.Scope.PackageCount, view.Scope.DependencyCount)
	}
	if cycles != 0 && len(view.Cycles[0].Packages) != packages {
		b.Fatalf("SCC members=%d, want %d", len(view.Cycles[0].Packages), packages)
	}
}

func (fixture packageBenchmarkFixture) report(b *testing.B) {
	b.Helper()
	b.ReportMetric(float64(len(fixture.files)), "files/op")
	b.ReportMetric(float64(fixture.types), "types/op")
	b.ReportMetric(float64(len(fixture.graph.Packages)), "packages/op")
	b.ReportMetric(float64(len(fixture.graph.Dependencies)), "edges/op")
}

func reportPackageBenchmarkView(b *testing.B, view packagegraph.View) {
	b.Helper()
	b.ReportMetric(float64(len(view.Graph.Packages)), "visible-packages/op")
	b.ReportMetric(float64(len(view.Graph.Dependencies)), "visible-edges/op")
	b.ReportMetric(float64(len(view.Cycles)), "sccs/op")
}
