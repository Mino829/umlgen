package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/deployment"
	"github.com/Mino829/umlgen/internal/explain"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/renderer"
)

func runDeployment(args []string, common commonOptions, stdout, stderr io.Writer) (int, error) {
	var manifest, manifestFormat, boundary, mapping, output, format, title, rendererType, serverURL string
	var timeout time.Duration
	var showExplain bool
	fs := flag.NewFlagSet("deployment", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&manifest, "manifest", "", "")
	fs.StringVar(&manifestFormat, "manifest-format", "", "")
	fs.StringVar(&boundary, "boundary-config", "", "")
	fs.StringVar(&mapping, "mapping-config", "", "")
	fs.StringVar(&output, "output", "", "")
	fs.StringVar(&output, "o", "", "")
	fs.StringVar(&format, "format", "", "")
	fs.StringVar(&format, "f", "", "")
	fs.StringVar(&title, "title", "", "")
	fs.StringVar(&rendererType, "renderer", "", "")
	fs.StringVar(&serverURL, "server-url", "", "")
	fs.DurationVar(&timeout, "render-timeout", 0, "")
	fs.BoolVar(&showExplain, "explain", false, "")
	fs.StringVar(&common.configPath, "config", common.configPath, "")
	fs.BoolVar(&common.quiet, "quiet", common.quiet, "")
	fs.BoolVar(&common.quiet, "q", common.quiet, "")
	fs.BoolVar(&common.verbose, "verbose", common.verbose, "")
	fs.BoolVar(&common.verbose, "v", common.verbose, "")
	normalized, err := normalizeDiagramArgs(args, fs)
	if err != nil {
		return exitArgs, err
	}
	if err = fs.Parse(normalized); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printDeploymentHelp(stdout)
			return exitOK, nil
		}
		return exitArgs, cleanFlagError(err)
	}
	if fs.NArg() != 0 {
		return exitArgs, errors.New("deployment accepts no positional targets; use --manifest")
	}
	if common.quiet && common.verbose {
		return exitArgs, errors.New("--verbose and --quiet cannot be used together")
	}
	set := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	cfg, loadedPath, err := config.Load(common.configPath, common.configPath != "")
	if err != nil {
		return exitArgs, err
	}
	if !cfg.Output.FileSet {
		cfg.Output.File = "deployment-diagram.puml"
	}
	config.ResolvePaths(&cfg, loadedPath)
	settings := cfg.Diagrams.Deployment
	if set["manifest"] {
		settings.Manifest = manifest
	}
	if set["manifest-format"] {
		settings.ManifestFormat = manifestFormat
	}
	if set["boundary-config"] {
		settings.BoundaryConfig = boundary
	}
	if set["mapping-config"] {
		settings.MappingConfig = mapping
	}
	if settings.Manifest == "" {
		return exitArgs, errors.New("deployment requires --manifest or diagrams.deployment.manifest")
	}
	settings.ManifestFormat = strings.ToLower(strings.TrimSpace(settings.ManifestFormat))
	switch settings.ManifestFormat {
	case "auto", "compose", "kubernetes":
	default:
		return exitArgs, errors.New("manifest format must be auto, compose or kubernetes")
	}
	if settings.MappingConfig != "" && settings.BoundaryConfig == "" {
		return exitArgs, errors.New("deployment mapping requires a boundary configuration")
	}
	if set["output"] || set["o"] {
		cfg.Output.File = output
	}
	if set["format"] || set["f"] {
		cfg.Output.Format = strings.ToLower(strings.TrimSpace(format))
	}
	if set["renderer"] {
		cfg.Renderer.Type = strings.ToLower(strings.TrimSpace(rendererType))
	}
	if set["server-url"] {
		cfg.Renderer.ServerURL = serverURL
	}
	if set["render-timeout"] {
		cfg.Renderer.Timeout = timeout
	}
	if cfg.Output.File == "" {
		return exitArgs, errors.New("output path must not be empty")
	}
	switch cfg.Output.Format {
	case "plantuml", "svg", "png":
	default:
		return exitArgs, errors.New("format must be plantuml, svg or png")
	}
	renderOpts := renderer.Config{Type: renderer.Type(cfg.Renderer.Type), ServerURL: cfg.Renderer.ServerURL, Timeout: cfg.Renderer.Timeout}
	if err = renderOpts.Validate(); err != nil {
		return exitArgs, err
	}
	graph, err := deployment.LoadManifest(settings.Manifest, settings.ManifestFormat)
	if err != nil {
		return deploymentError(err)
	}
	if len(graph.Nodes) == 0 {
		return exitArgs, errors.New("manifest has no supported deployment nodes; previous diagram preserved")
	}
	graph, err = deployment.BindComponents(graph, settings.BoundaryConfig, settings.MappingConfig)
	if err != nil {
		return deploymentError(err)
	}
	for _, d := range graph.Diagnostics {
		fmt.Fprintf(stderr, "Warning: %s:%d: %s: %s\n", d.Origin.File, d.Origin.Line, d.Code, d.Message)
	}
	content, err := plantuml.GenerateDeployment(graph, plantuml.DeploymentOptions{Title: title})
	if err != nil {
		return exitError, err
	}
	path := diagramOutputPath(cfg.Output.File)
	if err = writeDiagram(path, content); err != nil {
		return exitOutput, err
	}
	if !common.quiet {
		fmt.Fprintf(stdout, "Declared deployment nodes: %d; declared relationships: %d\nGenerated %s\n", len(graph.Nodes), len(graph.Edges), path)
	}
	if showExplain {
		fmt.Fprint(stdout, explain.FormatDeployment(graph))
	}
	if code, err := renderDiagram(cfg.Output.Format, path, renderOpts, common.quiet, stdout, stderr); code != exitOK || err != nil {
		return code, err
	}
	if graph.Incomplete {
		return exitParse, errors.New("declared deployment topology is incomplete; diagram and diagnostics were preserved")
	}
	return exitOK, nil
}

func deploymentError(err error) (int, error) {
	if errors.Is(err, deployment.ErrInput) {
		return exitArgs, err
	}
	return exitError, err
}

func printDeploymentHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  umlgen deployment --manifest FILE [options]

Draw declared Compose or Kubernetes topology, never observed runtime state.
No source scan, environment expansion, build or runtime tools are used.

Options:
  --manifest FILE                 Explicit manifest (required unless configured)
  --manifest-format FORMAT        auto, compose or kubernetes
  --boundary-config FILE          Explicit component boundaries
  --mapping-config FILE           Explicit bindings (requires boundaries)
  --output, -o FILE               Output (default deployment-diagram.puml)
  --format, -f FORMAT             plantuml, svg or png
  --title TEXT                   Diagram title
  --explain                      Explain declared evidence and limitations
  --renderer TYPE                auto, local or server
  --server-url URL               PlantUML server
  --render-timeout DURATION      Rendering timeout (default 30s; 0 disables timeout)
  --config FILE                  Shared configuration
  --quiet, -q                    Suppress progress
  --verbose, -v                  Verbose output

Incomplete supported topology writes a diagram and exits 3.
Invalid input exits 2 and preserves the previous diagram.`)
}
