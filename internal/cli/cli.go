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

	"github.com/umlgen/umlgen/internal/config"
	"github.com/umlgen/umlgen/internal/java"
	"github.com/umlgen/umlgen/internal/model"
	"github.com/umlgen/umlgen/internal/plantuml"
	"github.com/umlgen/umlgen/internal/scanner"
)

const Version = "0.1.0"

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
		case "class", "init", "version", "help":
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
	excludes                             stringList
	hideFields, hideMethods, hidePrivate bool
	noRelations                          bool
	outputSet, formatSet, excludesSet    bool
	hideFieldsSet, hideMethodsSet        bool
}

func runClass(args []string, inherited commonOptions, stdout, stderr io.Writer) (int, error) {
	var o classOptions
	o.commonOptions = inherited
	fs := flag.NewFlagSet("class", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.output, "output", "", "")
	fs.StringVar(&o.output, "o", "", "")
	fs.StringVar(&o.format, "format", "", "")
	fs.StringVar(&o.format, "f", "", "")
	fs.StringVar(&o.include, "include", "", "")
	fs.Var(&o.excludes, "exclude", "")
	fs.BoolVar(&o.hideFields, "hide-fields", false, "")
	fs.BoolVar(&o.hideMethods, "hide-methods", false, "")
	fs.BoolVar(&o.hidePrivate, "hide-private", false, "")
	fs.BoolVar(&o.noRelations, "no-relations", false, "")
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
		}
	})
	if fs.NArg() > 1 {
		return exitArgs, errors.New("class accepts at most one target")
	}

	cfg, loadedPath, err := config.Load(o.configPath, o.configPath != "")
	if err != nil {
		return exitArgs, err
	}
	config.ResolvePaths(&cfg, loadedPath)
	if o.outputSet {
		cfg.Output.File = o.output
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
	if cfg.Output.Format != "plantuml" && cfg.Output.Format != "svg" {
		return exitArgs, fmt.Errorf("unsupported format: %s\nSupported formats: plantuml, svg", cfg.Output.Format)
	}
	targets := cfg.Source
	if fs.NArg() == 1 {
		targets = []string{fs.Arg(0)}
	}
	if len(targets) == 0 {
		targets = []string{"."}
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
	if len(files) == 0 {
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
	if len(types) == 0 && warnings == len(files) {
		return exitParse, errors.New("failed to parse all Java files")
	}
	java.SortTypes(types)
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
      --no-relations        hide relationships
  -o, --output string       output file path
      --title string        diagram title
      --config string       configuration file path
  -q, --quiet               suppress normal output
  -v, --verbose             enable verbose output
`)
}
