package cli

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/focus"
	"github.com/Mino829/umlgen/internal/fsidentity"
	"github.com/Mino829/umlgen/internal/language"
	"github.com/Mino829/umlgen/internal/packagegraph"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/renderer"
	"github.com/Mino829/umlgen/internal/scanner"
	"github.com/Mino829/umlgen/internal/watch"
)

// The child flag registrations remain the authority for names, arity and defaults.
// The common projection below contains only options used to resolve watch scope.
type watchParsedOptions struct {
	class              classOptions
	pkg                packageOptions
	fs                 *flag.FlagSet
	set                map[string]bool
	interval, debounce time.Duration
}

func parseWatchOptions(diagram string, args []string, inherited commonOptions) (watchParsedOptions, []string, error) {
	o := watchParsedOptions{interval: 500 * time.Millisecond, debounce: 250 * time.Millisecond}
	o.fs = flag.NewFlagSet("watch "+diagram, flag.ContinueOnError)
	o.fs.SetOutput(io.Discard)
	if diagram == "package" {
		o.pkg.commonOptions = inherited
		registerPackageFlags(o.fs, &o.pkg)
	} else if diagram == "class" {
		o.class.commonOptions = inherited
		registerClassFlags(o.fs, &o.class)
	} else {
		return o, nil, fmt.Errorf("watch supports only class and package diagrams, not %q", diagram)
	}
	o.fs.DurationVar(&o.interval, "interval", o.interval, "")
	o.fs.DurationVar(&o.debounce, "debounce", o.debounce, "")
	normalized, err := normalizeDiagramArgs(args, o.fs)
	if err != nil {
		return o, nil, err
	}
	if err := o.fs.Parse(normalized); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return o, nil, err
		}
		return o, nil, cleanFlagError(err)
	}
	o.set = make(map[string]bool)
	o.fs.Visit(func(f *flag.Flag) { o.set[f.Name] = true })
	if diagram == "package" {
		p := o.pkg
		o.class = classOptions{
			commonOptions: p.commonOptions, output: p.output, format: p.format,
			language: p.language, title: p.title, focus: p.focus, direction: p.direction,
			relationKinds: p.relationKinds, depth: p.depth, excludes: p.excludes,
			layoutDirection: p.layoutDirection, skinParams: p.skinParams,
			rendererType: p.rendererType, serverURL: p.serverURL, renderTimeout: p.renderTimeout,
		}
	}
	return o, normalized, nil
}

func parseWatchRequest(args []string, inherited commonOptions) (watchRequest, bool, error) {
	diagram := "class"
	if len(args) > 0 && (args[0] == "class" || args[0] == "package") {
		diagram, args = args[0], args[1:]
	}
	o, normalized, err := parseWatchOptions(diagram, args, inherited)
	if errors.Is(err, flag.ErrHelp) {
		return watchRequest{}, true, nil
	}
	if err != nil {
		return watchRequest{}, false, &watchAttemptError{code: exitArgs, err: err}
	}
	if err := validateWatchOptions(diagram, o); err != nil {
		return watchRequest{}, false, &watchAttemptError{code: exitArgs, err: err}
	}
	child := watchWithoutFlags(normalized, o.fs, "interval", "debounce")
	return watchRequest{diagram: diagram, args: child, common: o.class.commonOptions, interval: o.interval, debounce: o.debounce}, false, nil
}

// Consume values using registered arity, not their spelling: --title --output
// sets a title, and -- ends option processing even for flag-looking targets.
func watchWithoutFlags(args []string, definitions *flag.FlagSet, names ...string) []string {
	child := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			child = append(child, args[i:]...)
			break
		}
		name, _, inline := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		definition := definitions.Lookup(name)
		isFlag := strings.HasPrefix(arg, "-") && arg != "-" && definition != nil
		remove := isFlag && has(names, name)
		if !remove {
			child = append(child, arg)
		}
		if isFlag && !inline {
			boolean, ok := definition.Value.(interface{ IsBoolFlag() bool })
			if !ok || !boolean.IsBoolFlag() {
				i++
				if !remove {
					child = append(child, args[i])
				}
			}
		}
	}
	return child
}

func validateWatchOptions(diagram string, o watchParsedOptions) error {
	c := o.class
	if o.interval <= 0 {
		return errors.New("--interval must be greater than zero")
	}
	if o.debounce < 0 {
		return errors.New("--debounce must be zero or greater")
	}
	if c.verbose && c.quiet {
		return errors.New("--verbose and --quiet cannot be used together")
	}
	if o.fs.NArg() > 1 {
		return fmt.Errorf("%s accepts at most one target", diagram)
	}
	if c.depth < 0 {
		return errors.New("--depth must be zero or greater")
	}
	if o.set["depth"] && c.focus == "" {
		return errors.New("--depth requires --focus")
	}
	if diagram == "package" && o.set["focus"] && c.focus == "" {
		return errors.New("--focus must not be empty")
	}
	if _, err := focus.ParseDirection(c.direction); err != nil {
		return err
	}
	if c.direction != "both" && c.focus == "" {
		return errors.New("--direction requires --focus")
	}
	if c.noRelations && c.relationKinds != "" {
		return errors.New("--relations and --no-relations cannot be used together")
	}
	if c.relationKinds != "" || diagram == "package" && o.set["relations"] {
		if _, err := parseRelationKinds(c.relationKinds); err != nil {
			return err
		}
	}
	return nil
}

func resolveWatchPlan(req watchRequest) (watchPlan, error) {
	plan, err := resolveWatchPlanInputs(req)
	if err == nil {
		return plan, nil
	}
	var attempt *watchAttemptError
	if errors.As(err, &attempt) {
		return watchPlan{}, err
	}
	return watchPlan{}, &watchAttemptError{code: exitArgs, err: err}
}

func resolveWatchPlanInputs(req watchRequest) (watchPlan, error) {
	o, _, err := parseWatchOptions(req.diagram, req.args, req.common)
	if err != nil {
		return watchPlan{}, err
	}
	if err := validateWatchOptions(req.diagram, o); err != nil {
		return watchPlan{}, err
	}
	c := o.class
	configPath := c.configPath
	if configPath == "" {
		configPath = config.DefaultFile
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return watchPlan{}, err
	}
	configSnapshot := make(watch.Snapshot)
	if err := watchSnapshotFile(configSnapshot, configPath, true); err != nil {
		return watchPlan{}, err
	}
	cfg, loadedPath, err := config.Load(c.configPath, c.configPath != "")
	if err != nil {
		return watchPlan{}, err
	}
	if req.diagram == "package" && !cfg.Output.FileSet {
		cfg.Output.File = "package-diagram.puml"
	}
	config.ResolvePaths(&cfg, loadedPath)
	if c.language != "" || req.diagram == "package" && o.set["language"] {
		cfg.Language = strings.ToLower(c.language)
		if req.diagram == "package" {
			cfg.Language = strings.TrimSpace(cfg.Language)
		}
	}
	if o.set["output"] || o.set["o"] {
		cfg.Output.File = c.output
	}
	if o.set["format"] || o.set["f"] {
		cfg.Output.Format = strings.ToLower(c.format)
		if req.diagram == "package" {
			cfg.Output.Format = strings.TrimSpace(cfg.Output.Format)
		}
	}
	if cfg.Output.File == "" {
		return watchPlan{}, errors.New("output path must not be empty")
	}
	if cfg.Output.Format != "plantuml" && cfg.Output.Format != "svg" && cfg.Output.Format != "png" {
		return watchPlan{}, fmt.Errorf("unsupported format %q: expected plantuml, svg or png", cfg.Output.Format)
	}
	if o.set["exclude"] {
		cfg.Exclude = append([]string(nil), c.excludes...)
	}
	if err := validateWatchConfig(req.diagram, o, &cfg); err != nil {
		return watchPlan{}, err
	}
	configured := strings.ToLower(strings.TrimSpace(cfg.Language))
	// Execution-ready arguments omit original output flags so the runner can
	// prepend its staged --output safely, including before a user's -- delimiter.
	plan := watchPlan{diagram: req.diagram, args: watchWithoutFlags(req.args, o.fs, "output", "o"), common: c.commonOptions,
		output: diagramOutputPath(cfg.Output.File), format: cfg.Output.Format, title: c.title, snapshot: make(watch.Snapshot)}
	plan.explain = o.fs.Lookup("explain").Value.String() == "true"
	plan.renderer = renderer.Config{Type: renderer.Type(cfg.Renderer.Type), ServerURL: cfg.Renderer.ServerURL, Timeout: cfg.Renderer.Timeout, Context: c.ctx}
	outputs := []string{plan.output}
	if plan.format != "plantuml" {
		outputs = append(outputs, strings.TrimSuffix(plan.output, filepath.Ext(plan.output))+"."+plan.format)
	}
	for i := range outputs {
		if filepath.Base(outputs[i]) == "go.mod" {
			return watchPlan{}, fmt.Errorf("watch output must not overwrite Go module metadata: %s", outputs[i])
		}
		if !filepath.IsAbs(outputs[i]) {
			cwd, err := os.Getwd()
			if err != nil {
				return watchPlan{}, err
			}
			// Preserve link/.. until physical identity has been resolved.
			outputs[i] = cwd + string(os.PathSeparator) + outputs[i]
		}
		info, statErr := os.Lstat(outputs[i])
		if statErr != nil && !os.IsNotExist(statErr) {
			return watchPlan{}, statErr
		}
		if statErr == nil && !info.Mode().IsRegular() {
			return watchPlan{}, fmt.Errorf("watch output must be a regular file, not a symlink or directory: %s", outputs[i])
		}
		physical, err := fsidentity.Resolve(outputs[i])
		if err != nil {
			return watchPlan{}, err
		}
		if i == 0 && physical != filepath.Clean(outputs[i]) {
			// Staging uses Dir(output), which cleans parent traversal. Publish
			// through the resolved path whenever that would change its meaning.
			plan.output = physical
		}
		outputs[i] = physical
	}
	if err := watchSnapshotFile(plan.snapshot, configPath, true); err != nil {
		return watchPlan{}, err
	}
	if plan.snapshot[configPath] != configSnapshot[configPath] {
		return watchPlan{}, errors.New("configuration changed while resolving watch inputs")
	}
	targets := cfg.Source
	if o.fs.NArg() == 1 {
		targets = []string{o.fs.Arg(0)}
	}
	if len(targets) == 0 {
		targets = []string{"."}
	}
	var liveTargets []string
	directories := make(map[string]bool)
	var markerDirectories map[string]bool
	if configured == "" || configured == "auto" {
		markerDirectories = make(map[string]bool)
	}
	watchExcludes := cfg.Exclude
	var candidates []string
	for _, target := range targets {
		absolute, err := filepath.Abs(target)
		if err != nil {
			return watchPlan{}, err
		}
		if watchStagePath(absolute) {
			continue
		}
		// Walk and execute using the same absolute path spelling as generated
		// directory exclusions; relative walks cannot match absolute excludes.
		target = absolute
		info, err := os.Stat(target)
		if err != nil {
			if !os.IsNotExist(err) {
				return watchPlan{}, &watchAttemptError{code: exitError, err: err}
			}
			plan.missing = true
			plan.snapshot[absolute] = [32]byte{}
			watchAncestorDirectories(directories, filepath.Dir(absolute))
			continue
		}
		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(target))
			if ext != ".java" && ext != ".go" {
				return watchPlan{}, &watchAttemptError{code: exitError, err: fmt.Errorf("target is not a supported source file or directory: %s", target)}
			}
			watchAncestorDirectories(directories, filepath.Dir(absolute))
			if markerDirectories != nil {
				watchAncestorDirectories(markerDirectories, filepath.Dir(absolute))
			}
		} else {
			watchAncestorDirectories(directories, absolute)
			if markerDirectories != nil {
				watchAncestorDirectories(markerDirectories, absolute)
			}
			for _, output := range outputs {
				collides, err := watchOutputInSourceScope(output, absolute, configured, cfg.Exclude)
				if err != nil {
					return watchPlan{}, err
				}
				if collides {
					return watchPlan{}, fmt.Errorf("watch output %s collides with selected source scope %s", output, target)
				}
			}
		}
		liveTargets = append(liveTargets, target)
		err = filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if strings.HasPrefix(entry.Name(), ".umlgen-watch-") || path != target && scanner.ExcludedDirectory(path, cfg.Exclude) {
					if strings.HasPrefix(entry.Name(), ".umlgen-watch-") {
						absolute, err := filepath.Abs(path)
						if err != nil {
							return err
						}
						watchExcludes = append(watchExcludes, absolute)
					}
					return filepath.SkipDir
				}
				return nil
			}
			if path != target && scanner.ExcludedPath(path, cfg.Exclude) {
				return nil
			}
			if entry.Name() == "go.mod" && configured != "java" {
				absolute, err := filepath.Abs(path)
				if err != nil {
					return err
				}
				if err := watchSnapshotFile(plan.snapshot, absolute, false); err != nil {
					return err
				}
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".java" || ext == ".go" && !strings.HasSuffix(strings.ToLower(path), "_test.go") {
				absolute, err := filepath.Abs(path)
				if err != nil {
					return err
				}
				candidates = append(candidates, absolute)
			}
			return nil
		})
		if err != nil {
			return watchPlan{}, &watchAttemptError{code: exitError, err: err}
		}
	}
	plan.targets = liveTargets
	watchExcludes = append(watchExcludes, outputs...)
	plan.excludes = watchExcludes
	var selected []string
	if configured != "" && configured != "auto" || len(candidates) > 0 {
		backend, err := language.Select(cfg.Language, liveTargets, watchExcludes, nil)
		if err != nil {
			return watchPlan{}, err
		}
		// Inspect original candidates too: an existing source must never be
		// reclassified as disposable generated content by the output exclusion.
		for _, file := range candidates {
			if backend.MatchesPath(file) {
				for _, output := range outputs {
					collides, err := watchSameFile(file, output)
					if err != nil {
						return watchPlan{}, err
					}
					if collides {
						return watchPlan{}, fmt.Errorf("watch output %s collides with source input %s", output, file)
					}
				}
			}
		}
		selected, err = scanSourceFiles(liveTargets, watchExcludes, backend)
		if err != nil {
			return watchPlan{}, &watchAttemptError{code: exitError, err: err}
		}
	}
	for _, file := range selected {
		if watchStagePath(file) {
			continue
		}
		if err := watchSnapshotFile(plan.snapshot, file, false); err != nil {
			return watchPlan{}, &watchAttemptError{code: exitError, err: err}
		}
		watchAncestorDirectories(directories, filepath.Dir(file))
	}
	plan.empty = true
	for _, file := range selected {
		if !watchStagePath(file) {
			plan.empty = false
			break
		}
	}
	// Absent ancestor metadata matters too: adding a module can change source
	// package/cache identity without changing any source bytes.
	for directory := range directories {
		if configured != "java" {
			if err := watchSnapshotFile(plan.snapshot, filepath.Join(directory, "go.mod"), true); err != nil {
				return watchPlan{}, err
			}
		}
	}
	if configured == "" || configured == "auto" {
		// Auto detection consults markers only at requested roots/ancestors,
		// unlike Go package binding, which also consults source ancestors.
		for directory := range markerDirectories {
			for _, name := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
				if err := watchSnapshotFile(plan.snapshot, filepath.Join(directory, name), true); err != nil {
					return watchPlan{}, err
				}
			}
		}
	}
	for _, output := range outputs {
		for input := range plan.snapshot {
			collides, err := watchSameFile(input, output)
			if err != nil {
				return watchPlan{}, err
			}
			if collides {
				return watchPlan{}, fmt.Errorf("watch output %s collides with input %s", output, input)
			}
		}
	}
	return plan, nil
}

func validateWatchConfig(diagram string, o watchParsedOptions, cfg *config.Config) error {
	c := o.class
	if c.packageGrouping != "" {
		cfg.Layout.PackageGrouping = c.packageGrouping
	}
	if c.layoutDirection != "" || diagram == "package" && o.set["layout-direction"] {
		cfg.Layout.Direction = c.layoutDirection
	}
	for _, setting := range c.skinParams {
		key, value, ok := strings.Cut(setting, "=")
		if !ok || strings.ContainsAny(setting, "\r\n") {
			return errors.New("--skinparam requires a single-line key=value")
		}
		if cfg.Layout.SkinParams == nil {
			cfg.Layout.SkinParams = make(map[string]string)
		}
		cfg.Layout.SkinParams[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if diagram == "package" {
		p := o.pkg
		if o.set["dependency-source"] {
			cfg.Diagrams.Package.DependencySource = p.dependencySource
		}
		if o.set["external"] {
			cfg.Diagrams.Package.External = p.external
		}
		if o.set["cycles"] {
			cfg.Diagrams.Package.Cycles = p.cycles
		}
		mode := packagegraph.DependencySource(strings.ToLower(strings.TrimSpace(cfg.Diagrams.Package.DependencySource)))
		if mode != packagegraph.DeclaredTypes && mode != packagegraph.Imports && mode != packagegraph.Both {
			return fmt.Errorf("unsupported dependency source %q", mode)
		}
		external := strings.ToLower(strings.TrimSpace(cfg.Diagrams.Package.External))
		if external != "hide" && external != "show" {
			return fmt.Errorf("unsupported external visibility %q: expected hide or show", external)
		}
		cycles := packagegraph.CycleMode(strings.ToLower(strings.TrimSpace(cfg.Diagrams.Package.Cycles)))
		if cycles != packagegraph.CyclesOff && cycles != packagegraph.CyclesMark && cycles != packagegraph.CyclesOnly {
			return fmt.Errorf("unsupported cycle mode %q", cycles)
		}
		if o.set["relations"] && mode == packagegraph.Imports {
			return errors.New("--relations cannot be combined with imports-only dependencies")
		}
		delete(cfg.Layout.SkinParams, "classAttributeIconSize")
		// Explicit package skinparams do not inherit the class-only exception.
		for _, setting := range c.skinParams {
			key, _, _ := strings.Cut(setting, "=")
			if strings.TrimSpace(key) == "classAttributeIconSize" {
				return errors.New("unsupported package skinparam classAttributeIconSize")
			}
		}
		if err := (plantuml.PackageOptions{Title: c.title, LayoutDirection: cfg.Layout.Direction, SkinParams: cfg.Layout.SkinParams}).Validate(); err != nil {
			return err
		}
	} else if err := (plantuml.Options{PackageGrouping: cfg.Layout.PackageGrouping, LayoutDirection: cfg.Layout.Direction, SkinParams: cfg.Layout.SkinParams}).ValidateLayout(); err != nil {
		return err
	}
	if o.set["renderer"] {
		cfg.Renderer.Type = strings.ToLower(c.rendererType)
	}
	if o.set["server-url"] {
		cfg.Renderer.ServerURL = c.serverURL
	}
	if o.set["render-timeout"] {
		cfg.Renderer.Timeout = c.renderTimeout
	}
	return (renderer.Config{Type: renderer.Type(cfg.Renderer.Type), ServerURL: cfg.Renderer.ServerURL, Timeout: cfg.Renderer.Timeout}).Validate()
}

func watchAncestorDirectories(directories map[string]bool, directory string) {
	for {
		directories[directory] = true
		parent := filepath.Dir(directory)
		if parent == directory {
			return
		}
		directory = parent
	}
}

func watchSnapshotFile(snapshot watch.Snapshot, path string, optional bool) error {
	info, err := os.Stat(path)
	if err != nil {
		if optional && os.IsNotExist(err) {
			snapshot[path] = [32]byte{}
			return nil
		}
		return fmt.Errorf("cannot inspect watch input %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("watch input is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if optional && os.IsNotExist(err) {
			snapshot[path] = [32]byte{}
			return nil
		}
		return fmt.Errorf("cannot read watch input %s: %w", path, err)
	}
	snapshot[path] = sha256.Sum256(data)
	return nil
}

func watchStagePath(path string) bool {
	slash := filepath.ToSlash(path)
	return strings.HasPrefix(slash, ".umlgen-watch-") || strings.Contains(slash, "/.umlgen-watch-")
}

func watchSameFile(a, b string) (bool, error) {
	leftPath, err := fsidentity.Resolve(a)
	if err != nil {
		return false, err
	}
	rightPath, err := fsidentity.Resolve(b)
	if err != nil {
		return false, err
	}
	if leftPath == rightPath {
		return true, nil
	}
	left, leftErr := os.Stat(a)
	right, rightErr := os.Stat(b)
	if leftErr != nil && !os.IsNotExist(leftErr) {
		return false, leftErr
	}
	if rightErr != nil && !os.IsNotExist(rightErr) {
		return false, rightErr
	}
	if leftErr == nil && rightErr == nil {
		return os.SameFile(left, right), nil
	}
	// An absent name has no inode to compare. Refuse case-only collisions
	// within the same physical parent, even on a case-sensitive filesystem.
	// This is output safety, not case-folding source or package identities.
	if !strings.EqualFold(filepath.Base(leftPath), filepath.Base(rightPath)) {
		return false, nil
	}
	leftParent, rightParent := filepath.Dir(leftPath), filepath.Dir(rightPath)
	if leftParent == rightParent {
		return true, nil
	}
	left, leftErr = os.Stat(leftParent)
	right, rightErr = os.Stat(rightParent)
	return leftErr == nil && rightErr == nil && os.SameFile(left, right), nil
}

func watchOutputInSourceScope(output, root, configured string, excludes []string) (bool, error) {
	ext := strings.ToLower(filepath.Ext(output))
	if ext != ".java" && ext != ".go" || configured == "java" && ext != ".java" || configured == "go" && ext != ".go" {
		return false, nil
	}
	if ext == ".go" && strings.HasSuffix(strings.ToLower(output), "_test.go") {
		return false, nil
	}
	physicalOutput, err := fsidentity.Resolve(output)
	if err != nil {
		return false, err
	}
	physicalRoot, err := fsidentity.Resolve(root)
	if err != nil {
		return false, err
	}
	rootInfo, err := os.Stat(physicalRoot)
	if err != nil {
		return false, err
	}
	// Compare ancestors physically too: EvalSymlinks does not canonicalize
	// path casing on case-insensitive filesystems.
	relative := filepath.Base(physicalOutput)
	for directory := filepath.Dir(physicalOutput); ; directory = filepath.Dir(directory) {
		info, statErr := os.Stat(directory)
		if statErr != nil && !os.IsNotExist(statErr) {
			return false, statErr
		}
		if directory == physicalRoot || statErr == nil && os.SameFile(rootInfo, info) {
			break
		}
		if directory == filepath.Dir(directory) {
			return false, nil
		}
		relative = filepath.Join(filepath.Base(directory), relative)
	}
	// Apply exclusions using the spelling seen by the source scanner, not
	// an output alias or a symlink's physical destination.
	scopedOutput := filepath.Join(root, relative)
	if scanner.ExcludedPath(scopedOutput, excludes) || watchStagePath(scopedOutput) {
		return false, nil
	}
	for directory := filepath.Dir(scopedOutput); directory != root; directory = filepath.Dir(directory) {
		if scanner.ExcludedDirectory(directory, excludes) {
			return false, nil
		}
		if directory == filepath.Dir(directory) {
			return false, nil
		}
	}
	return true, nil
}
