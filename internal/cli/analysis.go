package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/javacache"
	"github.com/Mino829/umlgen/internal/language"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/scanner"
)

type sourceAnalysisOptions struct {
	Config                          config.Config
	Parser                          language.Parser
	NoCache, CaptureSource, Verbose bool
}

type sourceFileResult struct {
	Unit     *model.SourceUnit
	Content  []byte
	Captured bool
	Err      error
}

type sourceAnalysis struct {
	parser           language.Parser
	cache            *javacache.Cache
	capture, verbose bool
	stdout, stderr   io.Writer
	hits, misses     int
}

func scanSourceFiles(targets, excludes []string, parser language.Parser) ([]string, error) {
	files, err := scanner.SourceFiles(targets, excludes, parser.Extensions()...)
	if err != nil {
		return nil, err
	}
	return matchingFiles(files, parser.MatchesPath), nil
}

func newSourceAnalysis(opts sourceAnalysisOptions, stdout, stderr io.Writer) *sourceAnalysis {
	a := &sourceAnalysis{parser: opts.Parser, capture: opts.CaptureSource, verbose: opts.Verbose, stdout: stdout, stderr: stderr}
	if opts.NoCache {
		if opts.Verbose {
			fmt.Fprintln(stdout, "Cache: disabled")
		}
		return a
	}
	settings := opts.Config
	// Preserve the existing syntax-cache contract while removing every display
	// setting, including command-specific settings and YAML presence metadata.
	settings.Output.File = ""
	settings.Output.Format = ""
	settings.Output.FileSet = false
	settings.Visibility.Public = false
	settings.Visibility.Protected = false
	settings.Visibility.Private = false
	settings.Visibility.PackagePrivate = false
	settings.Members.Fields = false
	settings.Members.Methods = false
	settings.Relations.Inheritance = false
	settings.Relations.Implementation = false
	settings.Relations.FieldDependency = false
	settings.Relations.ParameterDependency = false
	settings.Relations.ReturnDependency = false
	settings.Renderer.Type = ""
	settings.Renderer.ServerURL = ""
	settings.Renderer.Timeout = 0
	settings.Layout.PackageGrouping = ""
	settings.Layout.Direction = ""
	settings.Layout.SkinParams = nil
	settings.Diagrams = config.DiagramsConfig{}
	cache, err := javacache.OpenFor(Version, opts.Parser.Name(), opts.Parser.CacheSchema(), settings, opts.Parser.ParseSource)
	if err != nil {
		fmt.Fprintf(stderr, "Warning: cache disabled: %v\n", err)
	} else {
		a.cache = cache
	}
	return a
}

func (a *sourceAnalysis) parseFile(path string) sourceFileResult {
	if a.verbose {
		fmt.Fprintf(a.stdout, "Parsing: %s\n", path)
	}
	var parsed sourceFileResult
	if a.capture {
		parsed.Content, parsed.Err = os.ReadFile(path)
		if parsed.Err != nil {
			return parsed
		}
		parsed.Captured = true
	}
	if a.cache == nil {
		if a.capture {
			parsed.Unit, parsed.Err = a.parser.ParseSource(path, parsed.Content)
		} else {
			parsed.Unit, parsed.Err = a.parser.ParseFile(path)
		}
	} else {
		var result javacache.Result
		if a.capture {
			result, parsed.Err = a.cache.ParseSource(path, parsed.Content)
		} else {
			result, parsed.Err = a.cache.ParseFile(path)
		}
		parsed.Unit = result.Unit
		if result.Hit {
			a.hits++
		} else {
			a.misses++
		}
		if result.CacheError != nil {
			fmt.Fprintf(a.stderr, "Warning: cache entry was refreshed for %s: %v\n", path, result.CacheError)
		}
	}
	if parsed.Err == nil {
		a.parser.BindContext(parsed.Unit)
	}
	return parsed
}

func (a *sourceAnalysis) reportCache() {
	if a.verbose && a.cache != nil {
		fmt.Fprintf(a.stdout, "Cache hits: %d, misses: %d\n", a.hits, a.misses)
	}
}
