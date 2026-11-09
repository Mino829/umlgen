package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/Mino829/umlgen/internal/explain"
	"github.com/Mino829/umlgen/internal/focus"
	"github.com/Mino829/umlgen/internal/gitdiff"
	"github.com/Mino829/umlgen/internal/language"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/renderer"
)

func runClass(args []string, inherited commonOptions, stdout, stderr io.Writer) (int, error) {
	return runClassMode(args, inherited, stdout, stderr, nil)
}

func runClassMode(
	args []string,
	inherited commonOptions,
	stdout, stderr io.Writer,
	diffSelection *gitdiff.Result,
) (int, error) {
	o, positional, direction, err := parseClassRequest(args, inherited, diffSelection != nil)
	if errors.Is(err, flag.ErrHelp) {
		printClassHelp(stdout)
		return exitOK, nil
	}
	if err != nil {
		return exitArgs, err
	}
	cfg, opts, loadedPath, err := loadClassConfiguration(o, diffSelection != nil)
	if err != nil {
		return exitArgs, err
	}
	targets := cfg.Source
	if len(positional) == 1 {
		targets = positional
	}
	if len(targets) == 0 {
		if diffSelection != nil {
			targets = []string{diffSelection.Root}
		} else {
			targets = []string{"."}
		}
	}
	if o.watchTargets != nil {
		targets = o.watchTargets
	}
	var changedPaths []string
	if diffSelection != nil {
		for path := range diffSelection.Current {
			changedPaths = append(changedPaths, path)
		}
		for _, deleted := range diffSelection.Deleted {
			changedPaths = append(changedPaths, deleted.Path)
		}
	}
	sourceParser, err := language.Select(cfg.Language, targets, cfg.Exclude, changedPaths)
	if err != nil {
		return exitArgs, err
	}
	if o.verbose {
		if loadedPath != "" {
			fmt.Fprintf(stdout, "Config file: %s\n", loadedPath)
		}
		for _, target := range targets {
			fmt.Fprintf(stdout, "Target: %s\n", target)
		}
		fmt.Fprintf(stdout, "Language: %s\n", sourceParser.DisplayName())
		fmt.Fprintf(stdout, "Scanning %s files...\n", sourceParser.DisplayName())
	}
	var explanation explain.Report
	if o.explain {
		explanation.Scope = explain.Scope{
			Targets: targets, Excludes: cfg.Exclude, Include: o.include,
			Language: sourceParser.DisplayName(),
		}
		explanation.Sources = make(map[string]explain.Source)
	}
	files, err := scanSourceFiles(targets, cfg.Exclude, sourceParser)
	if err != nil {
		if o.explain {
			explanation.Failures = append(explanation.Failures, explain.Failure{
				Source: strings.Join(targets, ", "), Message: err.Error(), Origin: "working tree",
			})
			writeExplanation(stdout, explanation, nil, nil, opts)
		}
		return exitError, err
	}
	if o.explain {
		explanation.Scope.Files = len(files)
	}
	if len(files) == 0 && (diffSelection == nil || len(diffSelection.Deleted) == 0) {
		if o.explain {
			writeExplanation(stdout, explanation, nil, nil, opts)
		}
		return exitError, fmt.Errorf("no %s files found in: %s", sourceParser.DisplayName(), strings.Join(targets, ", "))
	}
	if o.verbose {
		for _, file := range files {
			fmt.Fprintf(stdout, "Found: %s\n", file)
		}
	}

	sources, err := collectClassSources(files, o, cfg, sourceParser, diffSelection, &explanation, stdout, stderr)
	if err != nil {
		return exitError, err
	}
	types, knownTypes := sources.types, sources.knownTypes
	warnings, changedParseFailures := sources.warnings, sources.changedParseFailures
	if diffSelection == nil && len(types) == 0 && warnings == len(files) {
		if o.explain {
			writeExplanation(stdout, explanation, nil, knownTypes, opts)
		}
		return exitParse, fmt.Errorf("failed to parse all %s files", sourceParser.DisplayName())
	}
	if o.strictParse && warnings > 0 {
		if o.explain {
			writeExplanation(stdout, explanation, types, knownTypes, opts)
		}
		return exitParse, fmt.Errorf("watch preserves the previous diagram because %d source file(s) failed to parse", warnings)
	}
	types = sourceParser.Finalize(types, o.explain)
	var noChanges bool
	if diffSelection != nil {
		var changed []string
		for i := range types {
			if types[i].Change != model.Unchanged {
				changed = append(changed, types[i].QualifiedName())
				continue
			}
			if change, ok := diffSelection.ChangeFor(types[i].Source); ok {
				types[i].Change = change
				changed = append(changed, types[i].QualifiedName())
			}
		}
		if len(changed) == 0 {
			if changedParseFailures > 0 {
				if o.explain {
					writeExplanation(stdout, explanation, nil, knownTypes, opts)
				}
				return exitParse, fmt.Errorf("failed to parse changed %s files; no changed types could be generated", sourceParser.DisplayName())
			}
			noChanges = true
		} else {
			types, err = focus.ApplyMany(types, changed, o.depth, direction)
			if err != nil {
				return exitArgs, err
			}
			if o.verbose {
				fmt.Fprintf(stdout, "Focused on %d changed types with depth %d (%d types)\n", len(changed), o.depth, len(types))
			}
		}
	} else if o.focus != "" {
		types, err = focus.Apply(types, o.focus, o.depth, direction)
		if err != nil {
			return exitArgs, err
		}
		if o.verbose {
			fmt.Fprintf(stdout, "Focused on %s with depth %d (%d types)\n", o.focus, o.depth, len(types))
		}
	}
	pumlPath := diagramOutputPath(cfg.Output.File)
	var content string
	if noChanges {
		content = plantuml.Placeholder(o.title, opts)
	} else {
		content = plantuml.Generate(model.Project{Types: types}, opts)
	}
	if err := writeDiagram(pumlPath, content); err != nil {
		return exitOutput, err
	}
	classes, interfaces := countKinds(types)
	if !o.quiet {
		fmt.Fprintf(stdout, "Found %d %s files\n", len(files), sourceParser.DisplayName())
		fmt.Fprintf(stdout, "Detected %d classes and %d interfaces\n", classes, interfaces)
		if noChanges {
			fmt.Fprintln(stdout, "No displayable changed types; generated placeholder diagram")
		}
		fmt.Fprintf(stdout, "Generated %s\n", pumlPath)
		if warnings > 0 {
			fmt.Fprintf(stdout, "%d warning(s)\n", warnings)
		}
	}
	if o.explain {
		explainedTypes := types
		if noChanges {
			explainedTypes = nil
		}
		writeExplanation(stdout, explanation, explainedTypes, knownTypes, opts)
	}
	return renderDiagram(cfg.Output.Format, pumlPath, renderer.Config{
		Type: renderer.Type(cfg.Renderer.Type), ServerURL: cfg.Renderer.ServerURL,
		Timeout: cfg.Renderer.Timeout, Context: o.ctx,
	}, o.quiet, stdout, stderr)
}
