package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/explain"
	"github.com/Mino829/umlgen/internal/language"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/renderer"
)

type packageOptions struct {
	commonOptions
	output, format, language, include, title string
	dependencySource, external, cycles       string
	focus, direction, relationKinds          string
	depth                                    int
	excludes, skinParams                     stringList
	noCache, explain                         bool
	layoutDirection, rendererType, serverURL string
	renderTimeout                            time.Duration
}

func runPackage(args []string, inherited commonOptions, stdout, stderr io.Writer) (int, error) {
	o := packageOptions{commonOptions: inherited}
	fs := flag.NewFlagSet("package", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.output, "output", "", "")
	fs.StringVar(&o.output, "o", "", "")
	fs.StringVar(&o.format, "format", "", "")
	fs.StringVar(&o.format, "f", "", "")
	fs.StringVar(&o.language, "language", "", "")
	fs.StringVar(&o.include, "include", "", "")
	fs.StringVar(&o.title, "title", "", "")
	fs.StringVar(&o.dependencySource, "dependency-source", "", "")
	fs.StringVar(&o.external, "external", "", "")
	fs.StringVar(&o.cycles, "cycles", "", "")
	fs.StringVar(&o.focus, "focus", "", "")
	fs.StringVar(&o.direction, "direction", "both", "")
	fs.StringVar(&o.relationKinds, "relations", "", "")
	fs.IntVar(&o.depth, "depth", 1, "")
	fs.Var(&o.excludes, "exclude", "")
	fs.Var(&o.skinParams, "skinparam", "")
	fs.BoolVar(&o.noCache, "no-cache", false, "")
	fs.BoolVar(&o.explain, "explain", false, "")
	fs.StringVar(&o.layoutDirection, "layout-direction", "", "")
	fs.StringVar(&o.rendererType, "renderer", "", "")
	fs.StringVar(&o.serverURL, "server-url", "", "")
	fs.DurationVar(&o.renderTimeout, "render-timeout", 0, "")
	fs.StringVar(&o.configPath, "config", o.configPath, "")
	fs.BoolVar(&o.verbose, "verbose", o.verbose, "")
	fs.BoolVar(&o.verbose, "v", o.verbose, "")
	fs.BoolVar(&o.quiet, "quiet", o.quiet, "")
	fs.BoolVar(&o.quiet, "q", o.quiet, "")
	normalized, err := normalizeDiagramArgs(args, fs)
	if err != nil {
		return exitArgs, err
	}
	if err := fs.Parse(normalized); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printPackageHelp(stdout)
			return exitOK, nil
		}
		return exitArgs, cleanFlagError(err)
	}
	set := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if o.verbose && o.quiet {
		return exitArgs, errors.New("--verbose and --quiet cannot be used together")
	}
	if fs.NArg() > 1 {
		return exitArgs, errors.New("package accepts at most one target")
	}
	if o.depth < 0 {
		return exitArgs, errors.New("--depth must be zero or greater")
	}
	if set["depth"] && o.focus == "" {
		return exitArgs, errors.New("--depth requires --focus")
	}
	if set["focus"] && o.focus == "" {
		return exitArgs, errors.New("--focus must not be empty")
	}
	if o.direction != "in" && o.direction != "out" && o.direction != "both" {
		return exitArgs, fmt.Errorf("unsupported package direction %q", o.direction)
	}
	if o.direction != "both" && o.focus == "" {
		return exitArgs, errors.New("--direction requires --focus")
	}
	cfg, loadedPath, err := config.Load(o.configPath, o.configPath != "")
	if err != nil {
		return exitArgs, err
	}
	if !cfg.Output.FileSet {
		cfg.Output.File = "package-diagram.puml"
	}
	config.ResolvePaths(&cfg, loadedPath)
	if set["language"] {
		cfg.Language = strings.ToLower(strings.TrimSpace(o.language))
	}
	if set["output"] || set["o"] {
		cfg.Output.File = o.output
	}
	if set["format"] || set["f"] {
		cfg.Output.Format = strings.ToLower(strings.TrimSpace(o.format))
	}
	if cfg.Output.File == "" {
		return exitArgs, errors.New("output path must not be empty")
	}
	if cfg.Output.Format != "plantuml" && cfg.Output.Format != "svg" && cfg.Output.Format != "png" {
		return exitArgs, fmt.Errorf("unsupported format %q: expected plantuml, svg or png", cfg.Output.Format)
	}
	if set["exclude"] {
		cfg.Exclude = append([]string(nil), o.excludes...)
	}
	if set["dependency-source"] {
		cfg.Diagrams.Package.DependencySource = o.dependencySource
	}
	if set["external"] {
		cfg.Diagrams.Package.External = o.external
	}
	if set["cycles"] {
		cfg.Diagrams.Package.Cycles = o.cycles
	}
	mode := packagegraph.DependencySource(strings.ToLower(strings.TrimSpace(cfg.Diagrams.Package.DependencySource)))
	if mode != packagegraph.DeclaredTypes && mode != packagegraph.Imports && mode != packagegraph.Both {
		return exitArgs, fmt.Errorf("unsupported dependency source %q", mode)
	}
	external := strings.ToLower(strings.TrimSpace(cfg.Diagrams.Package.External))
	if external != "hide" && external != "show" {
		return exitArgs, fmt.Errorf("unsupported external visibility %q: expected hide or show", external)
	}
	cycles := packagegraph.CycleMode(strings.ToLower(strings.TrimSpace(cfg.Diagrams.Package.Cycles)))
	if cycles != packagegraph.CyclesOff && cycles != packagegraph.CyclesMark && cycles != packagegraph.CyclesOnly {
		return exitArgs, fmt.Errorf("unsupported cycle mode %q", cycles)
	}
	viewOpts := packagegraph.ViewOptions{Depth: o.depth, Direction: packagegraph.Direction(o.direction), ShowExternal: external == "show", Cycles: cycles}
	if set["relations"] {
		if mode == packagegraph.Imports {
			return exitArgs, errors.New("--relations cannot be combined with imports-only dependencies")
		}
		kinds, err := parseRelationKinds(o.relationKinds)
		if err != nil {
			return exitArgs, err
		}
		for kind := range kinds {
			viewOpts.Relations = append(viewOpts.Relations, kind)
		}
		slices.Sort(viewOpts.Relations)
	}
	if set["layout-direction"] {
		cfg.Layout.Direction = o.layoutDirection
	}
	style := make(map[string]string, len(cfg.Layout.SkinParams)+len(o.skinParams))
	for key, value := range cfg.Layout.SkinParams {
		if key != "classAttributeIconSize" {
			style[key] = value
		}
	}
	for _, setting := range o.skinParams {
		key, value, ok := strings.Cut(setting, "=")
		if !ok || strings.ContainsAny(setting, "\r\n") {
			return exitArgs, errors.New("--skinparam requires a single-line key=value")
		}
		style[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	plantOpts := plantuml.PackageOptions{Title: o.title, LayoutDirection: cfg.Layout.Direction, SkinParams: style}
	if err := plantOpts.Validate(); err != nil {
		return exitArgs, err
	}
	if set["renderer"] {
		cfg.Renderer.Type = strings.ToLower(o.rendererType)
	}
	if set["server-url"] {
		cfg.Renderer.ServerURL = o.serverURL
	}
	if set["render-timeout"] {
		cfg.Renderer.Timeout = o.renderTimeout
	}
	renderOpts := renderer.Config{Type: renderer.Type(cfg.Renderer.Type), ServerURL: cfg.Renderer.ServerURL, Timeout: cfg.Renderer.Timeout}
	if err := renderOpts.Validate(); err != nil {
		return exitArgs, err
	}
	targets := cfg.Source
	if fs.NArg() == 1 {
		targets = []string{fs.Arg(0)}
	}
	if len(targets) == 0 {
		targets = []string{"."}
	}
	backend, err := language.Select(cfg.Language, targets, cfg.Exclude, nil)
	if err != nil {
		return exitArgs, err
	}
	report := explain.PackageReport{Scope: explain.Scope{Targets: targets, Excludes: cfg.Exclude, Include: o.include, Language: backend.DisplayName()}}
	if o.explain {
		report.Sources = make(map[string]explain.Source)
	}
	if o.verbose {
		if loadedPath != "" {
			fmt.Fprintf(stdout, "Config file: %s\n", loadedPath)
		}
		for _, target := range targets {
			fmt.Fprintf(stdout, "Target: %s\n", target)
		}
		fmt.Fprintf(stdout, "Language: %s\nScanning %s files...\n", backend.DisplayName(), backend.DisplayName())
	}
	files, err := scanSourceFiles(targets, cfg.Exclude, backend)
	if err != nil {
		if o.explain {
			report.Failures = []explain.Failure{{Source: strings.Join(targets, ", "), Message: err.Error(), Origin: "working tree"}}
			report.View = emptyPackageView(mode, viewOpts)
			fmt.Fprint(stdout, explain.FormatPackage(report))
		}
		return exitError, err
	}
	report.Scope.Files = len(files)
	if len(files) == 0 {
		if o.explain {
			report.View = emptyPackageView(mode, viewOpts)
			fmt.Fprint(stdout, explain.FormatPackage(report))
		}
		return exitError, fmt.Errorf("no %s files found in: %s", backend.DisplayName(), strings.Join(targets, ", "))
	}
	roots, err := packageSourceRoots(targets)
	if err != nil {
		return exitError, err
	}
	if o.verbose {
		for _, file := range files {
			fmt.Fprintf(stdout, "Found: %s\n", file)
		}
	}
	analysis := newSourceAnalysis(sourceAnalysisOptions{Config: cfg, Parser: backend, NoCache: o.noCache, CaptureSource: o.explain, Verbose: o.verbose}, stdout, stderr)
	var sources []packagegraph.Source
	var readFailures []packagegraph.Diagnostic
	warnings, succeeded := 0, 0
	for _, file := range files {
		result := analysis.parseFile(file)
		if o.explain && result.Captured {
			report.Sources[file] = explain.Source{Content: result.Content, Origin: "working tree"}
		}
		context := packageSourceContext(file, roots)
		if result.Err != nil {
			warnings++
			fmt.Fprintf(stderr, "Warning: failed to parse %s: %v\n", file, result.Err)
			if o.explain {
				report.Failures = append(report.Failures, explain.Failure{Source: file, Message: result.Err.Error(), Origin: "working tree"})
			}
		} else {
			succeeded++
			if backend.Name() == "go" {
				context = result.Unit.PackageContext
			}
		}
		if result.Unit != nil {
			sources = append(sources, packagegraph.Source{Context: context, Unit: result.Unit})
		} else {
			readFailures = append(readFailures, packagegraph.Diagnostic{From: packagegraph.PackageID{Language: backend.Name(), Context: context}, Status: packagegraph.Unresolved, Code: "source-read-error", File: packagegraph.File{Path: file}})
		}
	}
	analysis.reportCache()
	if backend.Name() == "go" {
		packageImplementations(sources, backend, o.explain)
	}
	known, err := packagegraph.Build(sources, mode)
	if err != nil {
		return exitError, err
	}
	known.Diagnostics = append(known.Diagnostics, readFailures...)
	report.Known = known
	selected := selectPackageScope(known, o.include, cfg.Exclude)
	if succeeded == 0 {
		report.View, err = packagegraph.Analyze(selected, viewOpts)
		if err != nil {
			return exitError, err
		}
		if o.explain {
			fmt.Fprint(stdout, explain.FormatPackage(report))
		}
		return exitParse, fmt.Errorf("failed to parse all %s files", backend.DisplayName())
	}
	if o.focus != "" {
		id, err := resolvePackageFocus(selected, o.focus)
		if err != nil {
			return exitArgs, err
		}
		viewOpts.Focus = &id
	}
	view, err := packagegraph.Analyze(selected, viewOpts)
	if err != nil {
		return exitArgs, err
	}
	report.View = view
	content, err := plantuml.GeneratePackage(view, plantOpts)
	if err != nil {
		return exitError, err
	}
	path := diagramOutputPath(cfg.Output.File)
	if err := writeDiagram(path, content); err != nil {
		return exitOutput, err
	}
	if !o.quiet {
		fmt.Fprintf(stdout, "Found %d %s files\n", len(files), backend.DisplayName())
		fmt.Fprintf(stdout, "Selected %d packages and %d dependencies; displayed %d packages and %d dependencies\n", view.Scope.PackageCount, view.Scope.DependencyCount, len(view.Graph.Packages), len(view.Graph.Dependencies))
		fmt.Fprintf(stdout, "Generated %s\n", path)
		if warnings != 0 {
			fmt.Fprintf(stdout, "%d warning(s)\n", warnings)
		}
	}
	if o.explain {
		fmt.Fprint(stdout, explain.FormatPackage(report))
	}
	return renderDiagram(cfg.Output.Format, path, renderOpts, o.quiet, stdout, stderr)
}

func emptyPackageView(mode packagegraph.DependencySource, opts packagegraph.ViewOptions) packagegraph.View {
	return packagegraph.View{Graph: packagegraph.Graph{Source: mode}, Scope: packagegraph.AnalysisScope{Source: mode, Relations: opts.Relations, Depth: opts.Depth, Direction: opts.Direction, ShowExternal: opts.ShowExternal, Cycles: opts.Cycles}}
}

func packageSourceRoots(targets []string) ([]string, error) {
	var roots []string
	for _, target := range targets {
		absolute, err := filepath.Abs(target)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			absolute = filepath.Dir(absolute)
		}
		roots = append(roots, filepath.Clean(absolute))
	}
	slices.SortFunc(roots, func(a, b string) int {
		if len(a) != len(b) {
			return len(a) - len(b)
		}
		return strings.Compare(a, b)
	})
	return slices.Compact(roots), nil
}

func packageSourceContext(path string, roots []string) string {
	for _, root := range roots {
		relative, err := filepath.Rel(root, path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return root
		}
	}
	return filepath.Dir(path)
}

// Preserve each file's imports/members while attaching derived implementation
// records only to its actual declaration anchor. Never merge equal names across
// independent module contexts, and collect matched methods only for --explain.
func packageImplementations(sources []packagegraph.Source, backend language.Parser, withEvidence bool) {
	groups := make(map[string][]*model.SourceUnit)
	for _, source := range sources {
		if source.Unit.PackagePath != "" {
			groups[source.Context] = append(groups[source.Context], source.Unit)
		}
	}
	for _, units := range groups {
		var types []model.Type
		for _, unit := range units {
			types = append(types, unit.Types...)
		}
		derived := make(map[string]model.Type)
		for _, typ := range backend.Finalize(types, withEvidence) {
			derived[typ.QualifiedName()] = typ
		}
		for _, unit := range units {
			for n, typ := range unit.Types {
				final := derived[typ.QualifiedName()]
				if final.Source == unit.Path {
					unit.Types[n].Implements = final.Implements
					unit.Types[n].ImplementationMethods = final.ImplementationMethods
				}
			}
		}
	}
}

func selectPackageScope(known packagegraph.Graph, include string, excludes []string) packagegraph.Graph {
	if include == "" && len(excludes) == 0 {
		return known
	}
	selected := packagegraph.Graph{Source: known.Source}
	keep := make(map[packagegraph.PackageID]bool, len(known.Packages))
	for _, pkg := range known.Packages {
		if includePackage(pkg.ID.Name, include) && !excludePackage(pkg.ID.Name, excludes) {
			keep[pkg.ID] = true
			selected.Packages = append(selected.Packages, pkg)
		}
	}
	for _, edge := range known.Dependencies {
		if keep[edge.From] && keep[edge.To] {
			selected.Dependencies = append(selected.Dependencies, edge)
		}
	}
	for _, diagnostic := range known.Diagnostics {
		if keep[diagnostic.From] || diagnostic.Code == "syntax-error" || diagnostic.Code == "source-read-error" || diagnostic.Code == "unsupported-module-declaration" {
			selected.Diagnostics = append(selected.Diagnostics, diagnostic)
		}
	}
	return selected
}

func resolvePackageFocus(graph packagegraph.Graph, selector string) (packagegraph.PackageID, error) {
	name, context := selector, ""
	if at := strings.LastIndex(selector, "::"); at >= 0 {
		context = selector[:at]
		name = selector[at+2:]
		absolute, err := filepath.Abs(context)
		if err != nil {
			return packagegraph.PackageID{}, err
		}
		context = filepath.Clean(absolute)
	}
	if name == "(unnamed package)" {
		name = ""
	}
	var matches []packagegraph.PackageID
	for _, pkg := range graph.Packages {
		if pkg.ID.Name == name && (context == "" || pkg.ID.Context == context) {
			matches = append(matches, pkg.ID)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return packagegraph.PackageID{}, fmt.Errorf("focus package not found: %s", selector)
	}
	var candidates []string
	for _, id := range matches {
		display := id.Name
		if display == "" {
			display = "(unnamed package)"
		}
		candidates = append(candidates, id.Context+"::"+display)
	}
	slices.Sort(candidates)
	return packagegraph.PackageID{}, fmt.Errorf("focus package %q is ambiguous; use one of: %s", selector, strings.Join(candidates, ", "))
}

func printPackageHelp(w io.Writer) {
	fmt.Fprint(w, `Generate a package dependency diagram from Java or Go source code.

Usage:
  umlgen package [target] [flags]

Flags:
      --dependency-source string  declared-types, imports, or both (default declared-types)
      --external string           hide or show external import paths (default hide)
      --cycles string             off, mark, or only (default mark)
      --focus string              canonical package name or context::name
      --depth int                 package hop distance from focus (default 1)
      --direction string          in, out, or both (default both)
      --relations string          comma-separated type relation kinds (not with imports-only)
      --include string            include package prefix
      --exclude string            replace configured excludes (repeatable)
      --language string           auto, java, or go
      --no-cache                  bypass parse cache reads and writes
      --explain                   explain package evidence, snapshots and analysis limits
  -o, --output string             output path (default package-diagram.puml)
  -f, --format string             plantuml, svg, or png
      --title string              diagram title
      --layout-direction string   top-to-bottom or left-to-right
      --skinparam key=value       linetype or shadowing (repeatable)
      --renderer string           auto, local, jar, or server
      --server-url string         server URL, only for explicit server renderer
      --render-timeout duration   rendering timeout (default 30s)
      --config string             configuration file path
  -q, --quiet                     suppress normal output (not requested explanations)
  -v, --verbose                   enable verbose output

Cycles describe the selected source model, not compiler/build validity.
Class/future-diagram-only flags are rejected. Package differences are not supported.
`)
}
