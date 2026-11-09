package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/gitdiff"
	"github.com/Mino829/umlgen/internal/javacache"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

var Version = "0.6.0-dev"

const (
	exitOK     = 0
	exitError  = 1
	exitArgs   = 2
	exitParse  = 3
	exitOutput = 4
	exitRender = 5
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type commonOptions struct {
	ctx                         context.Context
	strictParse                 bool
	watchTargets, watchExcludes []string
	configPath                  string
	verbose                     bool
	quiet                       bool
}

func Run(args []string, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 {
		printRootHelp(stdout)
		return exitOK, nil
	}
	common, commandArgs, err := parseCommon(args)
	if errors.Is(err, flag.ErrHelp) {
		printRootHelp(stdout)
		return exitOK, nil
	}
	if err != nil {
		return exitArgs, err
	}
	if len(commandArgs) == 0 {
		return exitArgs, errors.New("a command is required")
	}
	command, rest := commandArgs[0], commandArgs[1:]
	switch command {
	case "help":
		if len(rest) == 0 {
			printRootHelp(stdout)
			return exitOK, nil
		}
		switch rest[0] {
		case "class":
			printClassHelp(stdout)
			return exitOK, nil
		case "package":
			printPackageHelp(stdout)
			return exitOK, nil
		case "sequence":
			printSequenceHelp(stdout)
			return exitOK, nil
		case "activity":
			printActivityHelp(stdout)
			return exitOK, nil
		case "state":
			printStateHelp(stdout)
			return exitOK, nil
		case "component":
			printComponentHelp(stdout)
			return exitOK, nil
		case "deployment":
			printDeploymentHelp(stdout)
			return exitOK, nil
		case "watch":
			printWatchHelp(stdout)
			return exitOK, nil
		case "diff":
			printDiffHelp(stdout)
			return exitOK, nil
		case "init":
			fmt.Fprintln(stdout, "Usage:\n  umlgen init\n\nCreate a .umlgen.yaml configuration file.")
			return exitOK, nil
		case "cache":
			printCacheHelp(stdout)
			return exitOK, nil
		case "version":
			fmt.Fprintln(stdout, "Usage:\n  umlgen version")
			return exitOK, nil
		default:
			return exitArgs, fmt.Errorf("unknown help topic: %s", rest[0])
		}
	case "version":
		if len(rest) > 0 && !has(rest, "-h") && !has(rest, "--help") {
			return exitArgs, fmt.Errorf("version accepts no arguments")
		}
		fmt.Fprintf(stdout, "umlgen version %s\n", Version)
		return exitOK, nil
	case "init":
		return runInit(rest, common, stdout)
	case "cache":
		return runCache(rest, common, stdout)
	case "class":
		return runClass(rest, common, stdout, stderr)
	case "package":
		return runPackage(rest, common, stdout, stderr)
	case "sequence":
		return runSequence(rest, common, stdout, stderr)
	case "activity":
		return runActivity(rest, common, stdout, stderr)
	case "state":
		return runState(rest, common, stdout, stderr)
	case "component":
		return runComponent(rest, common, stdout, stderr)
	case "deployment":
		return runDeployment(rest, common, stdout, stderr)
	case "watch":
		return runWatch(rest, common, stdout, stderr)
	case "diff":
		return runDiff(rest, common, stdout, stderr)
	default:
		return exitArgs, fmt.Errorf("unknown command: %s", command)
	}
}

func parseCommon(args []string) (commonOptions, []string, error) {
	var c commonOptions
	fs := flag.NewFlagSet("umlgen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.configPath, "config", "", "")
	fs.BoolVar(&c.verbose, "verbose", false, "")
	fs.BoolVar(&c.verbose, "v", false, "")
	fs.BoolVar(&c.quiet, "quiet", false, "")
	fs.BoolVar(&c.quiet, "q", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return c, nil, err
		}
		return c, nil, cleanFlagError(err)
	}
	if c.verbose && c.quiet {
		return c, nil, errors.New("--verbose and --quiet cannot be used together")
	}
	return c, fs.Args(), nil
}

func runInit(args []string, common commonOptions, stdout io.Writer) (int, error) {
	if has(args, "-h") || has(args, "--help") {
		fmt.Fprintln(stdout, "Usage:\n  umlgen init\n\nCreate a .umlgen.yaml configuration file.")
		return exitOK, nil
	}
	if len(args) > 0 {
		return exitArgs, fmt.Errorf("init accepts no arguments")
	}
	path := config.DefaultFile
	if common.configPath != "" {
		path = common.configPath
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return exitArgs, fmt.Errorf("%s already exists", path)
		}
		return exitOutput, fmt.Errorf("failed to create configuration: %s", path)
	}
	if _, err = io.WriteString(file, config.Template); err != nil {
		file.Close()
		return exitOutput, fmt.Errorf("failed to write configuration: %s", path)
	}
	if err = file.Close(); err != nil {
		return exitOutput, fmt.Errorf("failed to write configuration: %s", path)
	}
	if !common.quiet {
		fmt.Fprintf(stdout, "Created %s\n", path)
	}
	return exitOK, nil
}

func runCache(args []string, common commonOptions, stdout io.Writer) (int, error) {
	if has(args, "-h") || has(args, "--help") {
		printCacheHelp(stdout)
		return exitOK, nil
	}
	if len(args) != 1 || args[0] != "clean" {
		return exitArgs, errors.New("cache requires the clean subcommand")
	}
	path, err := javacache.Clean()
	if err != nil {
		return exitError, err
	}
	if !common.quiet {
		fmt.Fprintf(stdout, "Cleared cache: %s\n", path)
	}
	return exitOK, nil
}

func runDiff(args []string, inherited commonOptions, stdout, stderr io.Writer) (int, error) {
	if has(args, "-h") || has(args, "--help") {
		printDiffHelp(stdout)
		return exitOK, nil
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return exitArgs, errors.New("diff requires a Git revision or range")
	}
	diagram, remaining, err := diffDiagramArgs(args[1:])
	if err != nil {
		return exitArgs, err
	}
	if diagram == "package" {
		return runPackageMode(remaining, inherited, stdout, stderr, args[0])
	}
	selection, err := gitdiff.Analyze(args[0])
	if err != nil {
		return exitError, err
	}
	return runClassMode(remaining, inherited, stdout, stderr, &selection)
}

func normalizeDiagramArgs(args []string, fs *flag.FlagSet) ([]string, error) {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			flags = append(flags, "--")
			positional = append(positional, args[i+1:]...)
			return append(flags, positional...), nil
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flags = append(flags, arg)
			name, _, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			if definition := fs.Lookup(name); definition != nil && !hasValue {
				boolean, isBoolean := definition.Value.(interface{ IsBoolFlag() bool })
				if !isBoolean || !boolean.IsBoolFlag() {
					if i+1 >= len(args) {
						return nil, fmt.Errorf("flag needs an argument: %s", arg)
					}
					i++
					flags = append(flags, args[i])
				}
			}
		} else {
			positional = append(positional, arg)
		}
	}
	return append(flags, positional...), nil
}

func matchingFiles(files []string, matches func(string) bool) []string {
	result := files[:0]
	for _, file := range files {
		if matches(file) {
			result = append(result, file)
		}
	}
	return result
}

func parseRelationKinds(value string) (map[relations.Kind]bool, error) {
	all := []relations.Kind{
		relations.Inheritance, relations.Implementation, relations.Field, relations.Parameter, relations.Return,
	}
	result := map[relations.Kind]bool{}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "all" {
			for _, kind := range all {
				result[kind] = true
			}
			continue
		}
		kind := relations.Kind(item)
		valid := false
		for _, candidate := range all {
			if kind == candidate {
				valid = true
				break
			}
		}
		if !valid {
			return nil, fmt.Errorf(
				"unsupported relation kind: %s\nSupported kinds: inheritance, implementation, field, parameter, return",
				item,
			)
		}
		result[kind] = true
	}
	return result, nil
}

func includePackage(pkg, prefix string) bool {
	return prefix == "" || pkg == prefix || strings.HasPrefix(pkg, prefix+".") || strings.HasPrefix(pkg, prefix+"/")
}

func excludePackage(pkg string, excludes []string) bool {
	for _, ex := range excludes {
		ex = strings.TrimSpace(ex)
		if ex == "" {
			continue
		}
		// Match as package prefix or path suffix
		if pkg == ex || strings.HasPrefix(pkg, ex+".") || strings.HasPrefix(pkg, ex+"/") || strings.HasSuffix(pkg, "/"+ex) || strings.HasSuffix(pkg, "\\"+ex) {
			return true
		}
	}
	return false
}

func countKinds(types []model.Type) (int, int) {
	var classes, interfaces int
	for _, t := range types {
		if t.Kind == model.Interface || t.Kind == model.Annotation {
			interfaces++
		} else {
			classes++
		}
	}
	return classes, interfaces
}

func cleanFlagError(err error) error {
	text := err.Error()
	if strings.HasPrefix(text, "flag provided but not defined:") {
		text = strings.Replace(text, "flag provided but not defined:", "unknown flag:", 1)
	}
	return errors.New(text)
}

func has(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func printRootHelp(w io.Writer) {
	fmt.Fprint(w, `Generate UML diagrams from source code.

Usage:
  umlgen [global options] <command> [arguments] [options]

Available Commands:
  class       Generate a class diagram
  package     Generate a package dependency diagram
  sequence    Project static calls and control regions from a selected entry
  activity    Trace explicit control-flow edges inside one selected function
  state       Compare an explicit state machine with source assignment candidates
  component   Aggregate package dependencies into explicit component boundaries
  deployment  Draw declared Compose or Kubernetes topology without runtime inspection
  watch       Regenerate class/package diagrams safely after source changes
  cache       Manage the local parse cache
  diff        Generate a diagram for Git changes
  init        Create a configuration file
  version     Print version information
  help        Help about any command

Flags:
      --config string   configuration file path
  -h, --help            help for umlgen
  -q, --quiet           suppress normal output
  -v, --verbose         enable verbose output
`)
}

func printCacheHelp(w io.Writer) {
	fmt.Fprint(w, `Manage the local source parse cache.

Usage:
  umlgen cache clean

The clean subcommand removes all cached source parse results for umlgen.
`)
}

func printDiffHelp(w io.Writer) {
	fmt.Fprint(w, `Generate a class or package dependency diff from Java or Go source code.

Usage:
  umlgen diff <revision-or-range> [target] [flags]

Examples:
  umlgen diff HEAD~1
  umlgen diff main...HEAD --depth 2 --show-relation-labels
  umlgen diff main...HEAD --diagram package --dependency-source both --explain

--diagram class (default) preserves the existing changed-type/working-tree behavior.
Class flags such as --depth, --direction, --output, --format, and --explain are supported.

--diagram package compares complete immutable Git trees, not changed files alone.
A single revision means revision..HEAD; A..B uses both commits; A...B uses merge-base(A,B)..B.
Dirty, staged and untracked content is excluded, including go.mod changes.
Package flags: --dependency-source declared-types|imports|both, --external hide|show,
--cycles off|mark|only, --relations, --include, --exclude, --language, --no-cache,
--explain, --layout-direction, --skinparam, --output, --format and renderer controls.
--focus, --depth, --direction and class-only flags are not supported for package diff.
Default package diff output: package-change-diagram.puml.
Partial parse failure emits an uncertain diff and exits 3; total failure preserves output.
Added/removed edges are distinct from modified evidence on an existing edge.
`)
}

func printClassHelp(w io.Writer) {
	fmt.Fprint(w, `Generate a PlantUML class diagram from Java or Go source code.

Usage:
  umlgen class [target] [flags]

Flags:
      --exclude string      exclude paths or packages (repeatable)
  -f, --format string       output format: plantuml, svg, or png
      --language string     source language: auto, java, or go
      --hide-fields         hide class fields
      --hide-methods        hide class methods
      --hide-private        hide private members
      --include string      include package prefix
      --focus string        include a type and its related types
      --depth int           relationship distance from --focus (default 1)
      --direction string    relation direction: in, out, or both (default both)
      --package-grouping string  package wrappers: flat, hierarchical, or none (default flat)
      --layout-direction string  top-to-bottom or left-to-right (default top-to-bottom)
      --skinparam key=value  safe PlantUML styling (repeatable)
      --relations string    relation kinds to show (comma-separated)
      --show-relation-labels
                            label field, parameter, and return relations
      --no-relations        hide relationships
      --no-cache            parse source without reading or writing the cache
      --explain             explain reasons, source snippets, and analysis limitations
  -o, --output string       output file path
      --renderer string     PlantUML renderer: auto, local, jar, or server
      --render-timeout duration  rendering timeout (default 30s)
      --server-url string   PlantUML server URL (used with --renderer server)
      --title string        diagram title
      --config string       configuration file path
  -q, --quiet               suppress normal output
  -v, --verbose             enable verbose output
`)
}
