package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/explain"
	"github.com/Mino829/umlgen/internal/gitsnapshot"
	"github.com/Mino829/umlgen/internal/language"
	"github.com/Mino829/umlgen/internal/packagediff"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/renderer"
)

// Use the registered flags to distinguish a diagram selector from a value such
// as --title=--diagram=package. The selected command still validates its flags.
func diffDiagramArgs(args []string) (string, []string, error) {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	var class classOptions
	var packages packageOptions
	registerClassFlags(fs, &class)
	packageFlags := flag.NewFlagSet("package", flag.ContinueOnError)
	registerPackageFlags(packageFlags, &packages)
	packageFlags.VisitAll(func(f *flag.Flag) {
		if fs.Lookup(f.Name) == nil {
			fs.Var(f.Value, f.Name, f.Usage)
		}
	})
	var diagram string
	fs.StringVar(&diagram, "diagram", "class", "")
	normalized, err := normalizeDiagramArgs(args, fs)
	if err != nil {
		return "", nil, err
	}
	remaining := make([]string, 0, len(normalized))
	for n := 0; n < len(normalized); n++ {
		arg := normalized[n]
		if arg == "--" {
			remaining = append(remaining, normalized[n:]...)
			break
		}
		name, value, inline := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		definition := fs.Lookup(name)
		if !strings.HasPrefix(arg, "-") || definition == nil {
			remaining = append(remaining, arg)
			continue
		}
		boolean, isBoolean := definition.Value.(interface{ IsBoolFlag() bool })
		if !inline && (!isBoolean || !boolean.IsBoolFlag()) {
			n++ // normalizeDiagramArgs already checked the value exists.
			value = normalized[n]
		}
		if name == "diagram" {
			diagram = value
			continue
		}
		remaining = append(remaining, arg)
		if !inline && (!isBoolean || !boolean.IsBoolFlag()) {
			remaining = append(remaining, value)
		}
	}
	if diagram != "class" && diagram != "package" {
		return "", nil, fmt.Errorf("unsupported diagram %q: expected class or package", diagram)
	}
	return diagram, remaining, nil
}

type packageSnapshotAnalysis struct {
	snapshot packagediff.Snapshot
	report   explain.PackageReport
	files    int
	parsed   int
	failed   int
}

func generatePackageDiff(rangeSpec string, targets []string, o packageOptions, cfg config.Config, mode packagegraph.DependencySource, viewOpts packagegraph.ViewOptions, plantOpts plantuml.PackageOptions, renderOpts renderer.Config, stdout, stderr io.Writer) (int, error) {
	pair, err := gitsnapshot.Load(rangeSpec, targets, cfg.Exclude)
	if err != nil {
		return exitError, err
	}
	backend, err := snapshotLanguage(cfg.Language, pair)
	if err != nil {
		return exitArgs, err
	}
	if o.verbose {
		fmt.Fprintf(stdout, "Language: %s\nBefore commit: %s\nAfter commit: %s\n", backend.DisplayName(), pair.Before.Revision, pair.After.Revision)
	}
	analysis := newSourceAnalysis(sourceAnalysisOptions{Config: cfg, Parser: backend, NoCache: o.noCache, CaptureSource: o.explain, Verbose: o.verbose}, stdout, stderr)
	// Compare complete selected graphs, before hiding external packages or
	// cycle-only projection. Display filters cannot turn retained edges into removals.
	fullOpts := viewOpts
	fullOpts.ShowExternal = true
	if fullOpts.Cycles == packagegraph.CyclesOnly {
		fullOpts.Cycles = packagegraph.CyclesMark
	}
	before, err := analyzePackageSnapshot(pair.Before, targets, o, cfg, mode, fullOpts, viewOpts, analysis, stderr)
	if err != nil {
		return exitError, err
	}
	after, err := analyzePackageSnapshot(pair.After, targets, o, cfg, mode, fullOpts, viewOpts, analysis, stderr)
	if err != nil {
		return exitError, err
	}
	analysis.reportCache()
	result := packagediff.Compare(before.snapshot, after.snapshot)
	result = packagediff.Project(result, viewOpts.ShowExternal, viewOpts.Cycles == packagegraph.CyclesOnly)
	report := explain.PackageDiffReport{Diff: result, Before: before.report, After: after.report}
	if before.files > 0 && before.parsed == 0 || after.files > 0 && after.parsed == 0 {
		if o.explain {
			fmt.Fprint(stdout, explain.FormatPackageDiff(report))
		}
		return exitParse, errors.New("package diff could not parse any selected source in at least one snapshot; previous output retained")
	}
	content, err := plantuml.GeneratePackageDiff(result, plantOpts)
	if err != nil {
		return exitError, err
	}
	path := diagramOutputPath(cfg.Output.File)
	if err := writeDiagram(path, content); err != nil {
		return exitOutput, err
	}
	if !o.quiet {
		fmt.Fprintf(stdout, "Compared Git commits %s..%s\n", pair.Before.Revision, pair.After.Revision)
		fmt.Fprintln(stdout, "Working-tree, staged and untracked changes are excluded; invocation filters apply to both commits.")
		fmt.Fprintf(stdout, "Before: %d files; after: %d files\nGenerated %s\n", before.files, after.files, path)
	}
	if o.explain {
		fmt.Fprint(stdout, explain.FormatPackageDiff(report))
	}
	if code, err := renderDiagram(cfg.Output.Format, path, renderOpts, o.quiet, stdout, stderr); code != exitOK || err != nil {
		return code, err
	}
	if before.failed+after.failed != 0 {
		return exitParse, fmt.Errorf("package diff is incomplete: %d source parse failure(s); uncertain differences must not establish dependency or cycle removal", before.failed+after.failed)
	}
	return exitOK, nil
}

// Auto language selection uses only the pinned source sets, never the current
// checkout or its project markers. Empty comparisons need an explicit language.
func snapshotLanguage(configured string, pair gitsnapshot.Pair) (language.Parser, error) {
	if configured != "" && configured != "auto" {
		return language.ForName(configured)
	}
	javaFound, goFound := false, false
	for _, snapshot := range []gitsnapshot.Snapshot{pair.Before, pair.After} {
		for _, file := range snapshot.Files {
			lower := strings.ToLower(file.Path)
			if strings.HasSuffix(lower, ".java") {
				javaFound = true
			} else if strings.HasSuffix(lower, ".go") && !strings.HasSuffix(lower, "_test.go") {
				goFound = true
			}
		}
	}
	if javaFound == goFound {
		return nil, errors.New("package diff snapshots contain mixed or no source languages; select --language java or --language go")
	}
	if javaFound {
		return language.ForName("java")
	}
	return language.ForName("go")
}

func analyzePackageSnapshot(snapshot gitsnapshot.Snapshot, targets []string, o packageOptions, cfg config.Config, mode packagegraph.DependencySource, fullOpts, displayOpts packagegraph.ViewOptions, analysis *sourceAnalysis, stderr io.Writer) (packageSnapshotAnalysis, error) {
	result := packageSnapshotAnalysis{snapshot: packagediff.Snapshot{Revision: snapshot.Revision, Complete: true}}
	result.report.Scope = explain.Scope{Targets: targets, Excludes: cfg.Exclude, Include: o.include, Language: analysis.parser.DisplayName()}
	if o.explain {
		result.report.Sources = make(map[string]explain.Source)
	}
	var sources []packagegraph.Source
	for _, file := range snapshot.Files {
		if !analysis.parser.MatchesPath(file.Path) {
			continue
		}
		result.files++
		parsed := analysis.parseSnapshotSource(file.Path, file.Content)
		origin := "Git " + snapshot.Revision
		if o.explain {
			result.report.Sources[file.Path] = explain.Source{Content: file.Content, Origin: origin}
		}
		if parsed.Err != nil {
			result.failed++
			result.snapshot.Complete = false
			fmt.Fprintf(stderr, "Warning: failed to parse %s (%s): %v\n", file.Path, origin, parsed.Err)
			if o.explain {
				result.report.Failures = append(result.report.Failures, explain.Failure{Source: file.Path, Message: parsed.Err.Error(), Origin: origin})
			}
		} else {
			result.parsed++
		}
		if parsed.Unit != nil {
			snapshot.BindContext(parsed.Unit)
			context := file.Context
			if analysis.parser.Name() == "go" {
				context = parsed.Unit.PackageContext
			}
			sources = append(sources, packagegraph.Source{Context: context, Unit: parsed.Unit})
		}
	}
	result.report.Scope.Files = result.files
	if analysis.parser.Name() == "go" {
		packageImplementations(sources, analysis.parser, o.explain)
	}
	known, err := packagegraph.Build(sources, mode)
	if err != nil {
		return result, err
	}
	result.report.Known = known
	selected := selectPackageScope(known, o.include, cfg.Exclude)
	result.snapshot.View, err = packagegraph.Analyze(selected, fullOpts)
	if err != nil {
		return result, err
	}
	result.report.View = result.snapshot.View
	result.report.View.Scope.ShowExternal = displayOpts.ShowExternal
	result.report.View.Scope.Cycles = displayOpts.Cycles
	return result, err
}

// Snapshot parsing deliberately does not invoke filesystem BindContext. The
// caller binds each cached syntax unit to its own immutable Git module tree.
func (a *sourceAnalysis) parseSnapshotSource(path string, content []byte) sourceFileResult {
	var parsed sourceFileResult
	if a.verbose {
		fmt.Fprintf(a.stdout, "Parsing Git source: %s\n", path)
	}
	if a.cache == nil {
		parsed.Unit, parsed.Err = a.parser.ParseSource(path, content)
		return parsed
	}
	cached, err := a.cache.ParseSource(path, content)
	parsed.Unit, parsed.Err = cached.Unit, err
	if cached.Hit {
		a.hits++
	} else {
		a.misses++
	}
	if cached.CacheError != nil {
		fmt.Fprintf(a.stderr, "Warning: cache entry was refreshed for %s: %v\n", path, cached.CacheError)
	}
	return parsed
}
