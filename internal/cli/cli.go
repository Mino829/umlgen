package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/focus"
	"github.com/Mino829/umlgen/internal/gitdiff"
	"github.com/Mino829/umlgen/internal/java"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/relations"
	"github.com/Mino829/umlgen/internal/scanner"
)

var Version = "0.3.0-dev"

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
	configPath string
	verbose    bool
	quiet      bool
}

func Run(args []string, stdout, stderr io.Writer) (int, error) {
	if len(args) == 0 {
		printRootHelp(stdout)
		return exitOK, nil
	}
	commandAt := -1
	for i, arg := range args {
		switch arg {
		case "class", "diff", "init", "version", "help":
			commandAt = i
		}
		if commandAt >= 0 {
			break
		}
	}
	if commandAt < 0 {
		if has(args, "-h") || has(args, "--help") {
			printRootHelp(stdout)
			return exitOK, nil
		}
		return exitArgs, fmt.Errorf("unknown command: %s", args[0])
	}
	common, err := parseCommon(args[:commandAt])
	if err != nil {
		return exitArgs, err
	}
	command, rest := args[commandAt], args[commandAt+1:]
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
		case "diff":
			printDiffHelp(stdout)
			return exitOK, nil
		case "init":
			fmt.Fprintln(stdout, "Usage:\n  umlgen init\n\nCreate a .umlgen.yaml configuration file.")
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
	case "class":
		return runClass(rest, common, stdout, stderr)
	case "diff":
		return runDiff(rest, common, stdout, stderr)
	default:
		return exitError, errors.New("unreachable command")
	}
}

func parseCommon(args []string) (commonOptions, error) {
	var c commonOptions
	fs := flag.NewFlagSet("umlgen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&c.configPath, "config", "", "")
	fs.BoolVar(&c.verbose, "verbose", false, "")
	fs.BoolVar(&c.verbose, "v", false, "")
	fs.BoolVar(&c.quiet, "quiet", false, "")
	fs.BoolVar(&c.quiet, "q", false, "")
	if err := fs.Parse(args); err != nil {
		return c, cleanFlagError(err)
	}
	if fs.NArg() > 0 {
		return c, fmt.Errorf("unexpected argument before command: %s", fs.Arg(0))
	}
	if c.verbose && c.quiet {
		return c, errors.New("--verbose and --quiet cannot be used together")
	}
	return c, nil
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

type classOptions struct {
	commonOptions
	output, format, include, title       string
	focus                                string
	direction                            string
	relationKinds                        string
	depth                                int
	excludes                             stringList
	hideFields, hideMethods, hidePrivate bool
	noRelations, showRelationLabels      bool
	outputSet, formatSet, excludesSet    bool
	hideFieldsSet, hideMethodsSet        bool
	depthSet                             bool
}

func runClass(args []string, inherited commonOptions, stdout, stderr io.Writer) (int, error) {
	return runClassMode(args, inherited, stdout, stderr, nil)
}

func runDiff(args []string, inherited commonOptions, stdout, stderr io.Writer) (int, error) {
	if has(args, "-h") || has(args, "--help") {
		printDiffHelp(stdout)
		return exitOK, nil
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return exitArgs, errors.New("diff requires a Git revision or range")
	}
	selection, err := gitdiff.Analyze(args[0])
	if err != nil {
		return exitError, err
	}
	return runClassMode(args[1:], inherited, stdout, stderr, &selection)
}

func runClassMode(
	args []string,
	inherited commonOptions,
	stdout, stderr io.Writer,
	diffSelection *gitdiff.Result,
) (int, error) {
	var o classOptions
	o.commonOptions = inherited
	fs := flag.NewFlagSet("class", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.output, "output", "", "")
	fs.StringVar(&o.output, "o", "", "")
	fs.StringVar(&o.format, "format", "", "")
	fs.StringVar(&o.format, "f", "", "")
	fs.StringVar(&o.include, "include", "", "")
	fs.StringVar(&o.focus, "focus", "", "")
	fs.StringVar(&o.direction, "direction", "both", "")
	fs.IntVar(&o.depth, "depth", 1, "")
	fs.StringVar(&o.relationKinds, "relations", "", "")
	fs.Var(&o.excludes, "exclude", "")
	fs.BoolVar(&o.hideFields, "hide-fields", false, "")
	fs.BoolVar(&o.hideMethods, "hide-methods", false, "")
	fs.BoolVar(&o.hidePrivate, "hide-private", false, "")
	fs.BoolVar(&o.noRelations, "no-relations", false, "")
	fs.BoolVar(&o.showRelationLabels, "show-relation-labels", false, "")
	fs.StringVar(&o.title, "title", "", "")
	fs.StringVar(&o.configPath, "config", o.configPath, "")
	fs.BoolVar(&o.verbose, "verbose", o.verbose, "")
	fs.BoolVar(&o.verbose, "v", o.verbose, "")
	fs.BoolVar(&o.quiet, "quiet", o.quiet, "")
	fs.BoolVar(&o.quiet, "q", o.quiet, "")
	normalized, normalizeErr := normalizeClassArgs(args)
	if normalizeErr != nil {
		return exitArgs, normalizeErr
	}
	if err := fs.Parse(normalized); err != nil {
		return exitArgs, cleanFlagError(err)
	}
	if has(args, "-h") || has(args, "--help") {
		printClassHelp(stdout)
		return exitOK, nil
	}
	if o.verbose && o.quiet {
		return exitArgs, errors.New("--verbose and --quiet cannot be used together")
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "output", "o":
			o.outputSet = true
		case "format", "f":
			o.formatSet = true
		case "exclude":
			o.excludesSet = true
		case "hide-fields":
			o.hideFieldsSet = true
		case "hide-methods":
			o.hideMethodsSet = true
		case "depth":
			o.depthSet = true
		}
	})
	if fs.NArg() > 1 {
		return exitArgs, errors.New("class accepts at most one target")
	}
	if o.depth < 0 {
		return exitArgs, errors.New("--depth must be zero or greater")
	}
	if o.depthSet && o.focus == "" && diffSelection == nil {
		return exitArgs, errors.New("--depth requires --focus")
	}
	direction, err := focus.ParseDirection(o.direction)
	if err != nil {
		return exitArgs, err
	}
	if o.direction != "both" && o.focus == "" && diffSelection == nil {
		return exitArgs, errors.New("--direction requires --focus")
	}
	if diffSelection != nil && o.focus != "" {
		return exitArgs, errors.New("--focus cannot be combined with the diff command")
	}
	if o.noRelations && o.relationKinds != "" {
		return exitArgs, errors.New("--relations and --no-relations cannot be used together")
	}

	cfg, loadedPath, err := config.Load(o.configPath, o.configPath != "")
	if err != nil {
		return exitArgs, err
	}
	config.ResolvePaths(&cfg, loadedPath)
	if o.outputSet {
		cfg.Output.File = o.output
	} else if diffSelection != nil && cfg.Output.File == "class-diagram.puml" {
		cfg.Output.File = "change-diagram.puml"
	}
	if o.formatSet {
		cfg.Output.Format = strings.ToLower(o.format)
	}
	if o.excludesSet {
		cfg.Exclude = append([]string(nil), o.excludes...)
	}
	if o.hideFieldsSet && o.hideFields {
		cfg.Members.Fields = false
	}
	if o.hideMethodsSet && o.hideMethods {
		cfg.Members.Methods = false
	}
	if o.hidePrivate {
		cfg.Visibility.Private = false
	}
	if o.relationKinds != "" {
		enabled, parseErr := parseRelationKinds(o.relationKinds)
		if parseErr != nil {
			return exitArgs, parseErr
		}
		cfg.Relations.Inheritance = enabled[relations.Inheritance]
		cfg.Relations.Implementation = enabled[relations.Implementation]
		cfg.Relations.FieldDependency = enabled[relations.Field]
		cfg.Relations.ParameterDependency = enabled[relations.Parameter]
		cfg.Relations.ReturnDependency = enabled[relations.Return]
	}
	if cfg.Output.Format != "plantuml" && cfg.Output.Format != "svg" {
		return exitArgs, fmt.Errorf("unsupported format: %s\nSupported formats: plantuml, svg", cfg.Output.Format)
	}
	targets := cfg.Source
	if fs.NArg() == 1 {
		targets = []string{fs.Arg(0)}
	}
	if len(targets) == 0 {
		if diffSelection != nil {
			targets = []string{diffSelection.Root}
		} else {
			targets = []string{"."}
		}
	}
	if o.verbose {
		if loadedPath != "" {
			fmt.Fprintf(stdout, "Config file: %s\n", loadedPath)
		}
		for _, target := range targets {
			fmt.Fprintf(stdout, "Target: %s\n", target)
		}
		fmt.Fprintln(stdout, "Scanning Java files...")
	}
	files, err := scanner.JavaFiles(targets, cfg.Exclude)
	if err != nil {
		return exitError, err
	}
	if len(files) == 0 && (diffSelection == nil || len(diffSelection.Deleted) == 0) {
		return exitError, fmt.Errorf("no Java files found in: %s", strings.Join(targets, ", "))
	}
	if o.verbose {
		for _, file := range files {
			fmt.Fprintf(stdout, "Found: %s\n", file)
		}
	}

	var types []model.Type
	warnings := 0
	for _, file := range files {
		if o.verbose {
			fmt.Fprintf(stdout, "Parsing: %s\n", file)
		}
		found, parseErr := java.ParseFile(file)
		if parseErr != nil {
			warnings++
			fmt.Fprintf(stderr, "Warning: failed to parse %s: %v\n", file, parseErr)
			continue
		}
		for _, t := range found {
			if includePackage(t.Package, o.include) && !excludePackage(t.Package, cfg.Exclude) {
				types = append(types, t)
				if o.verbose {
					fmt.Fprintf(stdout, "Detected %s: %s\n", t.Kind, t.QualifiedName())
				}
			}
		}
	}
	if diffSelection != nil {
		for _, deleted := range diffSelection.Deleted {
			found, parseErr := java.ParseSource(deleted.Path, deleted.Content)
			if parseErr != nil {
				warnings++
				fmt.Fprintf(stderr, "Warning: failed to parse deleted file %s: %v\n", deleted.Path, parseErr)
				continue
			}
			for _, t := range found {
				if includePackage(t.Package, o.include) && !excludePackage(t.Package, cfg.Exclude) {
					t.Change = model.Deleted
					types = append(types, t)
				}
			}
		}
	}
	if len(types) == 0 && warnings == len(files) {
		return exitParse, errors.New("failed to parse all Java files")
	}
	java.SortTypes(types)
	if diffSelection != nil {
		var changed []string
		for i := range types {
			if types[i].Change == model.Deleted {
				changed = append(changed, types[i].QualifiedName())
				continue
			}
			if change, ok := diffSelection.ChangeFor(types[i].Source); ok {
				types[i].Change = change
				changed = append(changed, types[i].QualifiedName())
			}
		}
		if len(changed) == 0 {
			return exitError, errors.New("changed Java files did not contain types in the selected target")
		}
		types, err = focus.ApplyMany(types, changed, o.depth, direction)
		if err != nil {
			return exitArgs, err
		}
		if o.verbose {
			fmt.Fprintf(stdout, "Focused on %d changed types with depth %d (%d types)\n", len(changed), o.depth, len(types))
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
	pumlPath := cfg.Output.File
	if strings.EqualFold(filepath.Ext(pumlPath), ".svg") {
		pumlPath = strings.TrimSuffix(pumlPath, filepath.Ext(pumlPath)) + ".puml"
	}
	if filepath.Ext(pumlPath) == "" {
		pumlPath += ".puml"
	}
	content := plantuml.Generate(model.Project{Types: types}, plantuml.Options{
		Title: o.title, ShowFields: cfg.Members.Fields, ShowMethods: cfg.Members.Methods,
		ShowPrivate: cfg.Visibility.Private, ShowPublic: cfg.Visibility.Public,
		ShowProtected: cfg.Visibility.Protected, ShowPackage: cfg.Visibility.PackagePrivate,
		ShowRelations: !o.noRelations, Inheritance: cfg.Relations.Inheritance,
		Implementation: cfg.Relations.Implementation, FieldDependency: cfg.Relations.FieldDependency,
		ParamDependency: cfg.Relations.ParameterDependency, ReturnDependency: cfg.Relations.ReturnDependency,
		ShowRelationLabels: o.showRelationLabels,
	})
	if err := os.MkdirAll(filepath.Dir(pumlPath), 0o755); err != nil {
		return exitOutput, fmt.Errorf("failed to create output directory: %s", filepath.Dir(pumlPath))
	}
	if err := os.WriteFile(pumlPath, []byte(content), 0o644); err != nil {
		return exitOutput, fmt.Errorf("failed to write output file: %s", pumlPath)
	}
	classes, interfaces := countKinds(types)
	if !o.quiet {
		fmt.Fprintf(stdout, "Found %d Java files\n", len(files))
		fmt.Fprintf(stdout, "Detected %d classes and %d interfaces\n", classes, interfaces)
		fmt.Fprintf(stdout, "Generated %s\n", pumlPath)
		if warnings > 0 {
			fmt.Fprintf(stdout, "%d warning(s)\n", warnings)
		}
	}
	if cfg.Output.Format == "svg" {
		svgPath, renderErr := renderSVG(pumlPath)
		if renderErr != nil {
			fmt.Fprintf(stderr, "Warning: PlantUML file was generated, but SVG rendering failed: %v\n", renderErr)
			return exitRender, nil
		}
		if !o.quiet {
			fmt.Fprintf(stdout, "Generated %s\n", svgPath)
		}
	}
	return exitOK, nil
}

func normalizeClassArgs(args []string) ([]string, error) {
	valueFlags := map[string]bool{
		"--output": true, "-o": true, "--format": true, "-f": true,
		"--include": true, "--exclude": true, "--title": true, "--config": true,
		"--focus": true, "--depth": true, "--direction": true, "--relations": true,
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			name := arg
			if at := strings.IndexByte(arg, '='); at >= 0 {
				name = arg[:at]
			}
			if valueFlags[name] && !strings.Contains(arg, "=") {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("flag needs an argument: %s", arg)
				}
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flags, positional...), nil
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

func renderSVG(pumlPath string) (string, error) {
	var command *exec.Cmd
	if binary, err := exec.LookPath("plantuml"); err == nil {
		command = exec.Command(binary, "-tsvg", pumlPath)
	} else if jar := os.Getenv("PLANTUML_JAR"); jar != "" {
		command = exec.Command("java", "-jar", jar, "-tsvg", pumlPath)
	} else {
		return "", errors.New("PlantUML was not found; install plantuml or set PLANTUML_JAR")
	}
	if output, err := command.CombinedOutput(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSuffix(pumlPath, filepath.Ext(pumlPath)) + ".svg", nil
}

func includePackage(pkg, prefix string) bool {
	return prefix == "" || pkg == prefix || strings.HasPrefix(pkg, prefix+".")
}

func excludePackage(pkg string, excludes []string) bool {
	for _, ex := range excludes {
		ex = strings.TrimSpace(ex)
		if strings.Contains(ex, ".") && (pkg == ex || strings.HasPrefix(pkg, ex+".")) {
			return true
		}
	}
	return false
}

func countKinds(types []model.Type) (int, int) {
	var classes, interfaces int
	for _, t := range types {
		if t.Kind == model.Interface {
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

func printDiffHelp(w io.Writer) {
	fmt.Fprint(w, `Generate a class diagram for changed Java types and their surroundings.

Usage:
  umlgen diff <revision-or-range> [target] [flags]

Examples:
  umlgen diff HEAD~1
  umlgen diff main...HEAD --depth 2 --show-relation-labels

Changed types are colored green (added), yellow (modified), or red (deleted).
Class command flags such as --depth, --direction, --output, and --format are supported.
`)
}

func printClassHelp(w io.Writer) {
	fmt.Fprint(w, `Generate a PlantUML class diagram from Java source code.

Usage:
  umlgen class [target] [flags]

Flags:
      --exclude string      exclude paths or packages (repeatable)
  -f, --format string       output format: plantuml or svg
      --hide-fields         hide class fields
      --hide-methods        hide class methods
      --hide-private        hide private members
      --include string      include package prefix
      --focus string        include a type and its related types
      --depth int           relationship distance from --focus (default 1)
      --direction string    relation direction: in, out, or both (default both)
      --relations string    relation kinds to show (comma-separated)
      --show-relation-labels
                            label field, parameter, and return relations
      --no-relations        hide relationships
  -o, --output string       output file path
      --title string        diagram title
      --config string       configuration file path
  -q, --quiet               suppress normal output
  -v, --verbose             enable verbose output
`)
}
