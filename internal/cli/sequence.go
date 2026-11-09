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

	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/explain"
	"github.com/Mino829/umlgen/internal/language"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/renderer"
	"github.com/Mino829/umlgen/internal/sequence"
	"github.com/Mino829/umlgen/internal/sequencego"
	"github.com/Mino829/umlgen/internal/sequencejava"
)

type sequenceOptions struct {
	commonOptions
	entry, output, format, language, title                  string
	maxCallDepth, maxParticipants, maxMessages, javaRelease int
	analysisTimeout, renderTimeout                          time.Duration
	rendererType, serverURL                                 string
	excludes                                                stringList
	explain                                                 bool
}

func runSequence(args []string, inherited commonOptions, stdout, stderr io.Writer) (int, error) {
	o := sequenceOptions{commonOptions: inherited}
	fs := flag.NewFlagSet("sequence", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.entry, "entry", "", "")
	fs.IntVar(&o.maxCallDepth, "max-call-depth", 0, "")
	fs.IntVar(&o.maxParticipants, "max-participants", 0, "")
	fs.IntVar(&o.maxMessages, "max-messages", 0, "")
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
			printSequenceHelp(stdout)
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
		return exitArgs, errors.New("sequence accepts at most one target")
	}
	cfg, loadedPath, err := config.Load(o.configPath, o.configPath != "")
	if err != nil {
		return exitArgs, err
	}
	if !cfg.Output.FileSet {
		cfg.Output.File = "sequence-diagram.puml"
	}
	config.ResolvePaths(&cfg, loadedPath)
	settings := cfg.Diagrams.Sequence
	if set["entry"] {
		settings.Entry = o.entry
	}
	if set["max-call-depth"] {
		settings.MaxCallDepth = o.maxCallDepth
	}
	if set["max-participants"] {
		settings.MaxParticipants = o.maxParticipants
	}
	if set["max-messages"] {
		settings.MaxMessages = o.maxMessages
	}
	if set["java-release"] {
		settings.JavaRelease = o.javaRelease
	}
	if set["analysis-timeout"] {
		settings.AnalysisTimeout = o.analysisTimeout
	}
	if strings.TrimSpace(settings.Entry) == "" {
		return exitArgs, errors.New("sequence requires a non-empty --entry or diagrams.sequence.entry")
	}
	if settings.MaxCallDepth < 0 || settings.MaxParticipants < 1 || settings.MaxMessages < 1 {
		return exitArgs, errors.New("sequence max-call-depth must be nonnegative and max-participants/max-messages must be positive")
	}
	if settings.JavaRelease < 17 || settings.AnalysisTimeout <= 0 {
		return exitArgs, errors.New("sequence java-release must be at least 17 and analysis-timeout must be positive")
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
		return exitArgs, errors.New("--java-release is only supported for Java sequence analysis")
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
			fmt.Fprintf(stdout, "Reading sequence source: %s\n", path)
		}
		info, readErr := os.Lstat(path)
		if readErr == nil && !info.Mode().IsRegular() {
			readErr = errors.New("sequence sources must be regular files; source symlinks and special files are not supported")
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
	limits := sequence.Limits{MaxCallDepth: settings.MaxCallDepth, MaxParticipants: settings.MaxParticipants, MaxMessages: settings.MaxMessages}
	program := sequence.Program{Language: backend.Name()}
	if len(sources) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), settings.AnalysisTimeout)
		defer cancel()
		if backend.Name() == "go" {
			program, err = sequencego.Analyze(ctx, sources)
		} else {
			program, err = sequencejava.Analyze(ctx, sources, sequencejava.Options{Release: settings.JavaRelease})
		}
		if err != nil {
			return exitError, err
		}
	}
	program.Diagnostics = append(program.Diagnostics, readFailures...)
	for _, failure := range readFailures {
		program.Files = append(program.Files, sequence.File{Path: failure.File})
	}
	failed := false
	for _, diagnostic := range program.Diagnostics {
		if diagnostic.Error {
			failed = true
		}
		fmt.Fprintf(stderr, "Warning: %s:%d:%d: %s: %s\n", diagnostic.File, diagnostic.Span.Start.Line, diagnostic.Span.Start.Column, diagnostic.Code, diagnostic.Message)
	}
	entry, err := sequence.Select(program, settings.Entry)
	if err != nil {
		if o.explain {
			fmt.Fprint(stdout, explain.FormatSequence(explain.SequenceReport{View: sequence.View{Program: program, Limits: limits}, Sources: captured}))
		}
		if failed && !errors.Is(err, sequence.ErrAmbiguousEntry) {
			return exitParse, fmt.Errorf("sequence analysis is incomplete and the selected entry could not be analyzed: %w", err)
		}
		return exitArgs, err
	}
	view, err := sequence.Project(program, entry, limits)
	if err != nil {
		return exitArgs, err
	}
	content, err := plantuml.GenerateSequence(view, plantuml.SequenceOptions{Title: o.title})
	if err != nil {
		return exitError, err
	}
	path := diagramOutputPath(cfg.Output.File)
	if err := writeDiagram(path, content); err != nil {
		return exitOutput, err
	}
	if !o.quiet {
		fmt.Fprintf(stdout, "Sequence backend: %s\nEntry: %s\nSelected files: %d; callables: %d; messages: %d; omissions: %d\nGenerated %s\n", program.Backend, entry.Selector, len(files), len(program.Callables), view.Messages, view.Omissions, path)
	}
	if o.explain {
		fmt.Fprint(stdout, explain.FormatSequence(explain.SequenceReport{View: view, Sources: captured}))
	}
	if code, err := renderDiagram(cfg.Output.Format, path, renderOpts, o.quiet, stdout, stderr); code != exitOK || err != nil {
		return code, err
	}
	if failed {
		return exitParse, errors.New("sequence analysis is incomplete; diagram and diagnostics were preserved")
	}
	return exitOK, nil
}

func printSequenceHelp(w io.Writer) {
	fmt.Fprint(w, `Project source-backed static calls and control regions, not an execution trace.

Usage:
  umlgen sequence [target] --entry <callable-selector> [flags]

Flags:
      --entry string              unique simple name, qualified name/signature, or context::selector
      --language string           auto, java, or go
      --max-call-depth int        direct-call expansion depth; 0 keeps entry calls only (default 3)
      --max-participants int      participant cap including entry (default 30)
      --max-messages int          call/async/defer/channel message cap (default 200)
      --java-release int          Java source release >=17 (default 17); Java only
      --analysis-timeout duration  analysis deadline (default 30s; cooperative for Go)
      --exclude string            replace configured path excludes (repeatable)
      --explain                   source positions/snippets, candidates, diagnostics and omissions
  -o, --output string             output file (default sequence-diagram.puml)
  -f, --format string             plantuml, svg, or png
      --title string              safe diagram title
      --renderer string           auto, local, jar, or server
      --server-url string         URL only for explicitly selected server renderer
      --render-timeout duration   rendering timeout (default 30s; 0 disables timeout)
      --config string             YAML configuration file
  -q, --quiet                     suppress progress, not requested explanations
  -v, --verbose                   verbose source progress

Go uses go/types with selected local sources only: no go command, downloads, drivers or cgo.
Java requires JDK>=17 and runs only the trusted embedded javac analysis helper with
annotation processing, implicit source loading, project classpath and code generation disabled.
Dynamic calls are alternatives; go/defer are not synchronous execution. Values and loop
counts are unknown. Recursion/limits and unsupported regions remain visible.
Sequence semantic analysis is always fresh; class/package --no-cache and layout flags do not apply.
Partial analysis errors produce a diagram with exit 3; unavailable entry preserves prior output.
`)
}
