package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Mino829/umlgen/internal/activity"
	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/explain"
	"github.com/Mino829/umlgen/internal/language"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/renderer"
	"github.com/Mino829/umlgen/internal/sequence"
	"github.com/Mino829/umlgen/internal/sequencego"
	"github.com/Mino829/umlgen/internal/sequencejava"
)

type activityOptions struct {
	commonOptions
	function, output, format, language, title string
	maxNodes, javaRelease                     int
	analysisTimeout, renderTimeout            time.Duration
	rendererType, serverURL                   string
	excludes                                  stringList
	explain                                   bool
}

func runActivity(args []string, inherited commonOptions, stdout, stderr io.Writer) (int, error) {
	o := activityOptions{commonOptions: inherited}
	fs := flag.NewFlagSet("activity", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.function, "function", "", "")
	fs.IntVar(&o.maxNodes, "max-nodes", 0, "")
	fs.IntVar(&o.javaRelease, "java-release", 0, "")
	fs.DurationVar(&o.analysisTimeout, "analysis-timeout", 0, "")
	fs.StringVar(&o.output, "output", "", "")
	fs.StringVar(&o.output, "o", "", "")
	fs.StringVar(&o.format, "format", "", "")
	fs.StringVar(&o.format, "f", "", "")
	fs.StringVar(&o.language, "language", "", "")
	fs.StringVar(&o.title, "title", "", "")
	fs.StringVar(&o.rendererType, "renderer", "", "")
	fs.StringVar(&o.serverURL, "server-url", "", "")
	fs.DurationVar(&o.renderTimeout, "render-timeout", 0, "")
	fs.Var(&o.excludes, "exclude", "")
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
	if err := fs.Parse(normalized); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printActivityHelp(stdout)
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
		return exitArgs, errors.New("activity accepts at most one target")
	}
	cfg, loadedPath, err := config.Load(o.configPath, o.configPath != "")
	if err != nil {
		return exitArgs, err
	}
	if !cfg.Output.FileSet {
		cfg.Output.File = "activity-diagram.puml"
	}
	config.ResolvePaths(&cfg, loadedPath)
	settings := cfg.Diagrams.Activity
	if set["function"] {
		settings.Function = o.function
	}
	if set["max-nodes"] {
		settings.MaxNodes = o.maxNodes
	}
	if set["java-release"] {
		settings.JavaRelease = o.javaRelease
	}
	if set["analysis-timeout"] {
		settings.AnalysisTimeout = o.analysisTimeout
	}
	if strings.TrimSpace(settings.Function) == "" {
		return exitArgs, errors.New("activity requires a non-empty --function or diagrams.activity.function")
	}
	if settings.MaxNodes < 1 {
		return exitArgs, errors.New("activity max-nodes must be positive")
	}
	if settings.JavaRelease < 17 || settings.AnalysisTimeout <= 0 {
		return exitArgs, errors.New("activity java-release must be at least 17 and analysis-timeout must be positive")
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
	if backend.Name() != "java" && set["java-release"] {
		return exitArgs, errors.New("--java-release is only supported for Java activity analysis")
	}
	files, err := scanSourceFiles(targets, cfg.Exclude, backend)
	if err != nil {
		return exitError, err
	}
	if len(files) == 0 {
		return exitError, fmt.Errorf("no %s files found in: %s", backend.DisplayName(), strings.Join(targets, ", "))
	}
	roots, err := packageSourceRoots(targets)
	if err != nil {
		return exitError, err
	}
	sources := make([]sequence.Source, 0, len(files))
	var readFailures []sequence.Diagnostic
	var captured map[string]explain.Source
	if o.explain {
		captured = make(map[string]explain.Source, len(files))
	}
	for _, path := range files {
		if o.verbose {
			fmt.Fprintf(stdout, "Reading activity source: %s\n", path)
		}
		info, readErr := os.Lstat(path)
		if readErr == nil && !info.Mode().IsRegular() {
			readErr = errors.New("activity sources must be regular files; source symlinks and special files are not supported")
		}
		var content []byte
		if readErr == nil {
			content, readErr = os.ReadFile(path)
		}
		if readErr != nil {
			readFailures = append(readFailures, sequence.Diagnostic{Code: "source-read-error", File: path, Message: readErr.Error(), Error: true})
			continue
		}
		sources = append(sources, sequence.Source{Path: path, Root: packageSourceContext(path, roots), Content: content})
		if o.explain {
			captured[path] = explain.Source{Content: content, Origin: "working tree"}
		}
	}
	graph := activity.Graph{Language: backend.Name()}
	if len(sources) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), settings.AnalysisTimeout)
		defer cancel()
		if backend.Name() == "go" {
			graph, err = sequencego.AnalyzeActivity(ctx, sources, settings.Function, settings.MaxNodes)
		} else {
			graph, err = sequencejava.AnalyzeActivity(ctx, sources, settings.Function, settings.MaxNodes, sequencejava.Options{Release: settings.JavaRelease})
		}
	} else {
		err = fmt.Errorf("%w: no readable selected sources", activity.ErrSelection)
	}
	graph.Diagnostics = append(graph.Diagnostics, readFailures...)
	for _, failure := range readFailures {
		graph.Files = append(graph.Files, sequence.File{Path: failure.File})
	}
	failed := false
	for _, diagnostic := range graph.Diagnostics {
		failed = failed || diagnostic.Error
		fmt.Fprintf(stderr, "Warning: %s:%d:%d: %s: %s\n", diagnostic.File, diagnostic.Span.Start.Line, diagnostic.Span.Start.Column, diagnostic.Code, diagnostic.Message)
	}
	if err != nil {
		if o.explain {
			fmt.Fprint(stdout, explain.FormatActivity(explain.ActivityReport{Graph: graph, MaxNodes: settings.MaxNodes, Sources: captured}))
		}
		if errors.Is(err, activity.ErrSelection) {
			if failed && !errors.Is(err, sequence.ErrAmbiguousEntry) {
				return exitParse, fmt.Errorf("activity analysis is incomplete and the selected function could not be analyzed: %w", err)
			}
			return exitArgs, err
		}
		if errors.Is(err, activity.ErrLimit) {
			return exitArgs, err
		}
		return exitError, err
	}
	content, err := plantuml.GenerateActivity(graph, plantuml.ActivityOptions{Title: o.title, MaxNodes: settings.MaxNodes})
	if err != nil {
		return exitError, err
	}
	path := diagramOutputPath(cfg.Output.File)
	if err := writeDiagram(path, content); err != nil {
		return exitOutput, err
	}
	if !o.quiet {
		fmt.Fprintf(stdout, "Activity backend: %s\nFunction: %s\nSelected files: %d; nodes: %d; edges: %d\nGenerated %s\n", graph.Backend, graph.Function.Selector, len(files), len(graph.Nodes), len(graph.Edges), path)
	}
	if o.explain {
		fmt.Fprint(stdout, explain.FormatActivity(explain.ActivityReport{Graph: graph, MaxNodes: settings.MaxNodes, Sources: captured}))
	}
	if code, err := renderDiagram(cfg.Output.Format, path, renderOpts, o.quiet, stdout, stderr); code != exitOK || err != nil {
		return code, err
	}
	if failed {
		return exitParse, errors.New("activity analysis is incomplete; diagram and diagnostics were preserved")
	}
	return exitOK, nil
}

func printActivityHelp(w io.Writer) {
	fmt.Fprint(w, `Draw source-backed control-flow possibilities inside one selected function.

Usage:
  umlgen activity [target] --function <callable-selector> [flags]

Flags:
      --function string           unique name, qualified name/signature, or context::selector
      --language string           auto, java, or go
      --max-nodes int             hard CFG node cap including entry/exit (default 500)
      --java-release int          Java source release >=17 (default 17); Java only
      --analysis-timeout duration analysis deadline (default 30s; cooperative for Go)
      --exclude string            replace configured excludes (repeatable)
      --explain                   nodes, edges, captured source snippets and limitations
  -o, --output string             output file (default activity-diagram.puml)
  -f, --format string             plantuml, svg, or png
      --title string              safe diagram title
      --renderer string           auto, local, jar, or server
      --server-url string         URL only for explicitly selected server renderer
      --render-timeout duration   rendering timeout (default 30s; 0 disables timeout)
      --config string             YAML configuration file
  -q, --quiet                     suppress progress, not requested explanations
  -v, --verbose                   verbose source progress

Explicit nodes and labeled CFG edges retain short-circuit branches, loop targets
and early exits; graph layout does not imply execution order or reachability.
Unsupported transfers are located terminal analysis barriers, not normal exits.
Calls and implicit exceptions/panics are not expanded. No target code is executed.
Go uses only captured local sources; Java requires JDK>=17 with project classpath,
processors, implicit source loading and code generation disabled. Analysis is fresh.
Class/package/sequence-only controls are rejected. Node cap/selection errors preserve
prior output. Partial parse/type failures preserve an available graph with exit 3.
`)
}
