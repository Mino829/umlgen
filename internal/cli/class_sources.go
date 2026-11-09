package cli

import (
	"fmt"
	"io"

	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/explain"
	"github.com/Mino829/umlgen/internal/gitdiff"
	"github.com/Mino829/umlgen/internal/language"
	"github.com/Mino829/umlgen/internal/model"
)

type classSourceResult struct {
	types, knownTypes              []model.Type
	warnings, changedParseFailures int
}

func collectClassSources(files []string, o classOptions, cfg config.Config, sourceParser language.Parser, diffSelection *gitdiff.Result, explanation *explain.Report, stdout, stderr io.Writer) (classSourceResult, error) {
	analysis := newSourceAnalysis(sourceAnalysisOptions{
		Config: cfg, Parser: sourceParser, NoCache: o.noCache,
		CaptureSource: o.explain, Verbose: o.verbose,
	}, stdout, stderr)
	var result classSourceResult
	for _, file := range files {
		if o.ctx != nil && o.ctx.Err() != nil {
			return result, o.ctx.Err()
		}
		changedFile := false
		if diffSelection != nil {
			_, changedFile = diffSelection.ChangeFor(file)
		}
		parsed := analysis.parseFile(file)
		unit, parseErr := parsed.Unit, parsed.Err
		if o.explain && parsed.Captured {
			explanation.Sources[file] = explain.Source{Content: parsed.Content, Origin: "working tree"}
		}
		if parseErr != nil {
			result.warnings++
			if changedFile {
				result.changedParseFailures++
			}
			fmt.Fprintf(stderr, "Warning: failed to parse %s: %v\n", file, parseErr)
			if o.explain {
				explanation.Failures = append(explanation.Failures, explain.Failure{
					Source: file, Message: parseErr.Error(), Origin: "working tree",
				})
			}
			continue
		}
		found := unit.Types
		if o.explain {
			result.knownTypes = append(result.knownTypes, found...)
		}
		for _, t := range found {
			if includePackage(t.Package, o.include) && !excludePackage(t.Package, cfg.Exclude) {
				if diffSelection != nil {
					if change, ok := diffSelection.ChangeFor(t.Source); ok {
						t.Change = change
					}
				}
				result.types = append(result.types, t)
				if o.verbose {
					fmt.Fprintf(stdout, "Detected %s: %s\n", t.Kind, t.QualifiedName())
				}
			}
		}
	}
	analysis.reportCache()
	if diffSelection == nil {
		return result, nil
	}
	for _, deleted := range diffSelection.Deleted {
		if !sourceParser.MatchesPath(deleted.Path) {
			continue
		}
		if o.explain {
			explanation.Scope.Files++
			explanation.Sources[deleted.Path] = explain.Source{Content: deleted.Content, Origin: "Git base"}
		}
		unit, parseErr := sourceParser.ParseSource(deleted.Path, deleted.Content)
		if parseErr != nil {
			result.warnings++
			result.changedParseFailures++
			fmt.Fprintf(stderr, "Warning: failed to parse deleted file %s: %v\n", deleted.Path, parseErr)
			if o.explain {
				explanation.Failures = append(explanation.Failures, explain.Failure{
					Source: deleted.Path, Message: parseErr.Error(), Origin: "Git base",
				})
			}
			continue
		}
		sourceParser.BindContext(unit)
		found := unit.Types
		if o.explain {
			result.knownTypes = append(result.knownTypes, found...)
		}
		for _, t := range found {
			if includePackage(t.Package, o.include) && !excludePackage(t.Package, cfg.Exclude) {
				t.Change = model.Deleted
				result.types = append(result.types, t)
			}
		}
	}
	return result, nil
}
