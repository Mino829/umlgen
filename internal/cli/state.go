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
	"github.com/Mino829/umlgen/internal/state"
)

type stateOptions struct {
	commonOptions
	definition, targetType, field, output, format, language, title string
	maxNodes, javaRelease                                          int
	analysisTimeout, renderTimeout                                 time.Duration
	rendererType, serverURL                                        string
	excludes                                                       stringList
	explain                                                        bool
}

func runState(args []string, inherited commonOptions, stdout, stderr io.Writer) (int, error) {
	o := stateOptions{commonOptions: inherited}
	fs := flag.NewFlagSet("state", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.definition, "state-config", "", "")
	fs.StringVar(&o.targetType, "type", "", "")
	fs.StringVar(&o.field, "state-field", "", "")
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
			printStateHelp(stdout)
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
		return exitArgs, errors.New("state accepts at most one target")
	}
	cfg, loadedPath, err := config.Load(o.configPath, o.configPath != "")
	if err != nil {
		return exitArgs, err
	}
	if !cfg.Output.FileSet {
		cfg.Output.File = "state-diagram.puml"
	}
	config.ResolvePaths(&cfg, loadedPath)
	settings := cfg.Diagrams.State
	if set["state-config"] {
		settings.StateConfig = o.definition
	}
	if set["type"] {
		settings.Type = o.targetType
	}
	if set["state-field"] {
		settings.Field = o.field
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
	if settings.StateConfig == "" {
		return exitArgs, errors.New("state requires --state-config or diagrams.state.state_config; a machine is never inferred from an enum")
	}
	if settings.MaxNodes < 1 || settings.JavaRelease < 17 || settings.AnalysisTimeout <= 0 {
		return exitArgs, errors.New("state requires positive max-nodes/analysis-timeout and java-release >=17")
	}
	definition, err := state.LoadDefinition(settings.StateConfig)
	if err != nil {
		return exitArgs, err
	}
	if settings.Type != "" || set["type"] {
		definition.Type = settings.Type
	}
	if settings.Field != "" || set["state-field"] {
		definition.Field = settings.Field
	}
	if strings.TrimSpace(definition.Type) == "" || strings.TrimSpace(definition.Field) == "" {
		return exitArgs, errors.New("state requires an explicit type and field in its definition, diagrams.state, or --type/--state-field")
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
		return exitArgs, errors.New("--java-release is only supported for Java state analysis")
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
			fmt.Fprintf(stdout, "Reading state source: %s\n", path)
		}
		info, readErr := os.Lstat(path)
		if readErr == nil && !info.Mode().IsRegular() {
			readErr = errors.New("state sources must be regular files; source symlinks and special files are not supported")
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
	analysis := state.Analysis{Language: backend.Name()}
	if len(sources) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), settings.AnalysisTimeout)
		defer cancel()
		if backend.Name() == "go" {
			analysis, err = sequencego.AnalyzeState(ctx, sources, definition.Type, definition.Field, settings.MaxNodes)
		} else {
			analysis, err = sequencejava.AnalyzeState(ctx, sources, definition.Type, definition.Field, settings.MaxNodes, sequencejava.Options{Release: settings.JavaRelease})
		}
	} else {
		err = fmt.Errorf("%w: no readable selected sources", state.ErrSelection)
	}
	analysis.Diagnostics = append(analysis.Diagnostics, readFailures...)
	for _, failure := range readFailures {
		analysis.Files = append(analysis.Files, sequence.File{Path: failure.File})
	}
	failed := false
	for _, d := range analysis.Diagnostics {
		failed = failed || d.Error
		fmt.Fprintf(stderr, "Warning: %s:%d:%d: %s: %s\n", d.File, d.Span.Start.Line, d.Span.Start.Column, d.Code, d.Message)
	}
	if err != nil {
		if o.explain {
			fmt.Fprint(stdout, explain.FormatState(explain.StateReport{View: state.View{Definition: definition, Analysis: analysis, MaxNodes: settings.MaxNodes}, Sources: captured}))
		}
		if errors.Is(err, state.ErrSelection) {
			if failed && !errors.Is(err, state.ErrAmbiguous) {
				return exitParse, err
			}
			return exitArgs, err
		}
		if errors.Is(err, activity.ErrLimit) {
			return exitArgs, err
		}
		return exitError, err
	}
	view, err := state.Bind(definition, analysis, settings.MaxNodes)
	if err != nil {
		if o.explain {
			fmt.Fprint(stdout, explain.FormatState(explain.StateReport{View: view, Sources: captured}))
		}
		return exitArgs, err
	}
	content, err := plantuml.GenerateState(view, plantuml.StateOptions{Title: o.title})
	if err != nil {
		return exitError, err
	}
	path := diagramOutputPath(cfg.Output.File)
	if err := writeDiagram(path, content); err != nil {
		return exitOutput, err
	}
	if !o.quiet {
		fmt.Fprintf(stdout, "State backend: %s\nTarget: %s.%s\nConfigured states: %d; transitions: %d; source assignment candidates: %d\nGenerated %s\n", analysis.Backend, analysis.Target.Qualified, analysis.Target.Field, len(definition.States), len(definition.Transitions), len(view.Candidates), path)
	}
	if o.explain {
		fmt.Fprint(stdout, explain.FormatState(explain.StateReport{View: view, Sources: captured}))
	}
	if code, err := renderDiagram(cfg.Output.Format, path, renderOpts, o.quiet, stdout, stderr); code != exitOK || err != nil {
		return code, err
	}
	if failed {
		return exitParse, errors.New("state source analysis is incomplete; configured diagram and source diagnostics were preserved")
	}
	return exitOK, nil
}

func printStateHelp(w io.Writer) {
	fmt.Fprint(w, `Draw an explicitly defined state machine and separate source assignment candidates.

Usage:
  umlgen state [target] --state-config <machine.yaml> [flags]

Flags:
      --state-config string       strict version: 1 machine YAML; never executed
      --type string               override unique type name, qualified name or context::name
      --state-field string        override explicitly selected direct state field
      --language string           auto, java, or go
      --max-nodes int             hard CFG cap per candidate-containing function (default 500)
      --java-release int          Java release >=17 (default 17); Java only
      --analysis-timeout duration analysis deadline (default 30s; cooperative for Go)
      --exclude string            replace configured excludes (repeatable)
      --explain                   configured provenance, source writes, guards, CFG coverage
  -o, --output string             output file (default state-diagram.puml)
  -f, --format string             plantuml, svg, or png
      --title string              safe diagram title
      --renderer string           auto, local, jar, or server
      --server-url string         URL only for explicitly selected server renderer
      --render-timeout duration   rendering timeout (default 30s; 0 disables timeout)
      --config string             shared YAML configuration file
  -q, --quiet                     suppress progress, not explanations
  -v, --verbose                   verbose source progress

Configured initial state/transitions are declarations, not source-proven behavior.
Source assignments never prove a previous state, instance identity, event or transition:
only dashed unknown-previous-state candidates may be shown; guards remain source text.
Optional state value selectors bind compiler constants/enum members or observed literals.
No enum-only machine inference, runtime evaluation, target build, execution or downloads.
Unknown/ambiguous bindings, invalid machines and hard limits preserve previous output.
Partial source errors preserve an available diagram with exit 3. Analysis is always fresh.
`)
}
