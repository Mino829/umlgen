package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mino829/umlgen/internal/component"
	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/explain"
	"github.com/Mino829/umlgen/internal/golang"
	"github.com/Mino829/umlgen/internal/language"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/renderer"
)

type componentOptions struct {
	commonOptions
	boundary, output, format, language, title, mode, external string
	rendererType, serverURL                                   string
	renderTimeout                                             time.Duration
	excludes                                                  stringList
	noCache, explain                                          bool
}

func runComponent(args []string, inherited commonOptions, stdout, stderr io.Writer) (int, error) {
	o := componentOptions{commonOptions: inherited}
	fs := flag.NewFlagSet("component", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.boundary, "boundary-config", "", "")
	fs.StringVar(&o.mode, "dependency-source", "", "")
	fs.StringVar(&o.external, "external", "", "")
	fs.StringVar(&o.language, "language", "", "")
	fs.StringVar(&o.output, "output", "", "")
	fs.StringVar(&o.output, "o", "", "")
	fs.StringVar(&o.format, "format", "", "")
	fs.StringVar(&o.format, "f", "", "")
	fs.StringVar(&o.title, "title", "", "")
	fs.StringVar(&o.rendererType, "renderer", "", "")
	fs.StringVar(&o.serverURL, "server-url", "", "")
	fs.DurationVar(&o.renderTimeout, "render-timeout", 0, "")
	fs.Var(&o.excludes, "exclude", "")
	fs.BoolVar(&o.noCache, "no-cache", false, "")
	fs.BoolVar(&o.explain, "explain", false, "")
	fs.StringVar(&o.configPath, "config", o.configPath, "")
	fs.BoolVar(&o.quiet, "quiet", o.quiet, "")
	fs.BoolVar(&o.quiet, "q", o.quiet, "")
	fs.BoolVar(&o.verbose, "verbose", o.verbose, "")
	fs.BoolVar(&o.verbose, "v", o.verbose, "")
	normalized, err := normalizeDiagramArgs(args, fs)
	if err != nil {
		return exitArgs, err
	}
	if err = fs.Parse(normalized); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printComponentHelp(stdout)
			return exitOK, nil
		}
		return exitArgs, cleanFlagError(err)
	}
	set := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if o.quiet && o.verbose {
		return exitArgs, errors.New("--verbose and --quiet cannot be used together")
	}
	if fs.NArg() > 1 {
		return exitArgs, errors.New("component accepts at most one target")
	}
	cfg, loadedPath, err := config.Load(o.configPath, o.configPath != "")
	if err != nil {
		return exitArgs, err
	}
	if !cfg.Output.FileSet {
		cfg.Output.File = "component-diagram.puml"
	}
	config.ResolvePaths(&cfg, loadedPath)
	settings := cfg.Diagrams.Component
	if set["boundary-config"] {
		settings.BoundaryConfig = o.boundary
	}
	if set["dependency-source"] {
		settings.DependencySource = o.mode
	}
	if set["external"] {
		settings.External = o.external
	}
	if settings.BoundaryConfig == "" {
		return exitArgs, errors.New("component requires --boundary-config or diagrams.component.boundary_config; architecture ownership is never inferred")
	}
	definition, err := component.LoadDefinition(settings.BoundaryConfig)
	if err != nil {
		return exitArgs, err
	}
	mode := packagegraph.DependencySource(strings.ToLower(strings.TrimSpace(settings.DependencySource)))
	if mode != packagegraph.DeclaredTypes && mode != packagegraph.Imports && mode != packagegraph.Both {
		return exitArgs, fmt.Errorf("unsupported dependency source %q", mode)
	}
	external := strings.ToLower(strings.TrimSpace(settings.External))
	if external != "hide" && external != "show" {
		return exitArgs, fmt.Errorf("unsupported external visibility %q", external)
	}
	if set["language"] {
		cfg.Language = strings.ToLower(strings.TrimSpace(o.language))
	}
	if set["output"] || set["o"] {
		cfg.Output.File = o.output
	}
	if set["format"] || set["f"] {
		cfg.Output.Format = strings.ToLower(strings.TrimSpace(o.format))
	}
	if set["exclude"] {
		cfg.Exclude = append([]string(nil), o.excludes...)
	}
	if set["renderer"] {
		cfg.Renderer.Type = strings.ToLower(strings.TrimSpace(o.rendererType))
	}
	if set["server-url"] {
		cfg.Renderer.ServerURL = o.serverURL
	}
	if set["render-timeout"] {
		cfg.Renderer.Timeout = o.renderTimeout
	}
	if cfg.Output.File == "" {
		return exitArgs, errors.New("output path must not be empty")
	}
	if cfg.Output.Format != "plantuml" && cfg.Output.Format != "svg" && cfg.Output.Format != "png" {
		return exitArgs, fmt.Errorf("unsupported format %q: expected plantuml, svg or png", cfg.Output.Format)
	}
	renderOpts := renderer.Config{Type: renderer.Type(cfg.Renderer.Type), ServerURL: cfg.Renderer.ServerURL, Timeout: cfg.Renderer.Timeout}
	if err = renderOpts.Validate(); err != nil {
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
	report := explain.ComponentReport{Scope: explain.Scope{Targets: targets, Excludes: cfg.Exclude, Language: backend.DisplayName()}}
	files, err := scanSourceFiles(targets, cfg.Exclude, backend)
	if err != nil {
		return exitError, err
	}
	if len(files) == 0 {
		return exitError, fmt.Errorf("no %s files found in: %s", backend.DisplayName(), strings.Join(targets, ", "))
	}
	report.Scope.Files = len(files)
	if o.explain {
		report.Sources = make(map[string]explain.Source, len(files))
	}
	roots, err := packageSourceRoots(targets)
	if err != nil {
		return exitError, err
	}
	analysis := newSourceAnalysis(sourceAnalysisOptions{Config: cfg, Parser: backend, NoCache: o.noCache, CaptureSource: o.explain, Verbose: o.verbose}, stdout, stderr)
	sources := make([]packagegraph.Source, 0, len(files))
	var failures []packagegraph.Diagnostic
	contexts := make(map[string]string)
	for _, file := range files {
		result := analysis.parseFile(file)
		if o.explain && result.Captured {
			report.Sources[file] = explain.Source{Content: result.Content, Origin: "working tree"}
		}
		context := packageSourceContext(file, roots)
		if result.Err != nil {
			fmt.Fprintf(stderr, "Warning: failed to parse %s: %v\n", file, result.Err)
			report.Failures = append(report.Failures, explain.Failure{Source: file, Message: result.Err.Error(), Origin: "working tree"})
			failedFile := packagegraph.File{Path: file}
			if result.Unit != nil {
				failedFile.ContentHash = result.Unit.ContentHash
			}
			failures = append(failures, packagegraph.Diagnostic{From: packagegraph.PackageID{Language: backend.Name(), Context: context}, Status: packagegraph.Unresolved, Code: "source-parse-error", File: failedFile})
			continue // Recovered/invalid declarations cannot prove a boundary dependency.
		}
		if backend.Name() == "go" {
			context = result.Unit.PackageContext
		}
		// Definition roots use physical identity, not invocation-path aliases
		// such as /var versus /private/var or a symlinked source directory.
		physical, ok := contexts[context]
		if !ok {
			physical, err = filepath.EvalSymlinks(context)
			if err != nil {
				return exitError, fmt.Errorf("resolve component source context %q: %w", context, err)
			}
			contexts[context] = physical
		}
		context = physical
		sources = append(sources, packagegraph.Source{Context: context, Unit: result.Unit})
	}
	analysis.reportCache()
	if backend.Name() == "go" {
		packageImplementations(sources, backend, o.explain)
	}
	known, err := packagegraph.Build(sources, mode)
	if err != nil {
		return exitError, err
	}
	known.Diagnostics = append(known.Diagnostics, failures...)
	if len(sources) == 0 {
		report.Graph = component.Graph{Definition: definition, Source: known}
		if o.explain {
			fmt.Fprint(stdout, explain.FormatComponent(report))
		}
		return exitParse, fmt.Errorf("failed to parse all %s files; previous component diagram preserved", backend.DisplayName())
	}
	modules, err := captureComponentModules(known)
	if err != nil {
		return exitError, err
	}
	graph, err := component.Project(known, definition, modules, component.Options{ShowExternal: external == "show"})
	report.Graph = graph
	if err != nil {
		if o.explain {
			report.Graph.Definition = definition
			report.Graph.Source = known
			report.Graph.Modules = modules
			fmt.Fprint(stdout, explain.FormatComponent(report))
		}
		if errors.Is(err, component.ErrBoundary) {
			return exitArgs, err
		}
		return exitError, err
	}
	for _, d := range graph.Diagnostics {
		fmt.Fprintf(stderr, "Warning: %s:%d: %s: %s\n", definition.Source, d.Line, d.Code, d.Message)
	}
	content, err := plantuml.GenerateComponent(graph, plantuml.ComponentOptions{Title: o.title})
	if err != nil {
		return exitError, err
	}
	path := diagramOutputPath(cfg.Output.File)
	if err = writeDiagram(path, content); err != nil {
		return exitOutput, err
	}
	if !o.quiet {
		fmt.Fprintf(stdout, "Configured components: %d; displayed nodes: %d; inter-boundary dependencies: %d\nGenerated %s\n", len(definition.Components), len(graph.Nodes), len(graph.Dependencies), path)
	}
	if o.explain {
		fmt.Fprint(stdout, explain.FormatComponent(report))
	}
	if code, err := renderDiagram(cfg.Output.Format, path, renderOpts, o.quiet, stdout, stderr); code != exitOK || err != nil {
		return code, err
	}
	if len(report.Failures) > 0 {
		return exitParse, errors.New("component source analysis is incomplete; diagram and diagnostics were preserved")
	}
	return exitOK, nil
}

func captureComponentModules(graph packagegraph.Graph) ([]component.Module, error) {
	seen := make(map[string]bool)
	var modules []component.Module
	for _, pkg := range graph.Packages {
		if pkg.Kind != packagegraph.InternalPackage || pkg.ID.Language != "go" || seen[pkg.ID.Context] {
			continue
		}
		seen[pkg.ID.Context] = true
		path := filepath.Join(pkg.ID.Context, "go.mod")
		content, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("capture Go module metadata %s: %w", path, err)
		}
		name := golang.ModuleName(content)
		if name == "" {
			return nil, fmt.Errorf("cannot determine lexical Go module name from %s", path)
		}
		hash := sha256.Sum256(content)
		modules = append(modules, component.Module{Context: pkg.ID.Context, Name: name, File: path, Hash: hex.EncodeToString(hash[:])})
	}
	return modules, nil
}

func printComponentHelp(w io.Writer) {
	fmt.Fprint(w, `Aggregate package dependencies into explicitly configured architecture boundaries.

Usage:
  umlgen component [target] --boundary-config <architecture.yaml> [flags]

Flags:
      --boundary-config string   strict version: 1 boundary YAML (required)
      --dependency-source string declared-types, imports, or both (default both)
      --external string          hide or show external package identities (default hide)
      --language string          auto, java, or go
      --exclude string           replace configured file/path excludes (repeatable)
      --no-cache                 disable syntax cache, not boundary validation
      --explain                  boundary ownership, original package/type/import evidence
  -o, --output string            output file (default component-diagram.puml)
  -f, --format string            plantuml, svg, or png
      --title string             safe diagram title
      --renderer string          auto, local, jar, or server
      --server-url string        URL only for explicitly selected server renderer
      --render-timeout duration  rendering timeout (default 30s; 0 disables timeout)
      --config string            shared YAML configuration file
  -q, --quiet                    suppress progress, not requested explanations
  -v, --verbose                  verbose source/cache progress

Boundary selectors explicitly combine language with package_prefix, source_root,
or go_module. Prefixes match package separators, not text prefixes or globs.
source_root is an exact physical analysis-context root; go_module is an exact
captured Go module path, not a Java/Maven/Gradle module or dependency declaration.
Multiple component owners are errors, never first-match or longest-prefix wins.
Unassigned internal packages remain distinct nodes; external packages never acquire
source ownership. Internal dependencies retain evidence but do not draw self-arrows.
Provides/requires are configured display contracts: same names do not auto-connect.
No runtime communication, deployment, target build, processors or scripts are inferred
or executed. Existing class/package syntax parsers are used; Java needs no JDK.
Invalid boundaries preserve prior output. Partial failed files are excluded with
source diagnostics and exit 3; an all-failed analysis preserves the prior diagram.
Class/package focus, include, cycles, relation and layout controls do not apply.
`)
}
