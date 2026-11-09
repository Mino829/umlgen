package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/focus"
	"github.com/Mino829/umlgen/internal/plantuml"
	"github.com/Mino829/umlgen/internal/relations"
)

type classOptions struct {
	commonOptions
	output, format, include, title       string
	focus                                string
	direction                            string
	relationKinds                        string
	language                             string
	depth                                int
	excludes                             stringList
	hideFields, hideMethods, hidePrivate bool
	noRelations, showRelationLabels      bool
	noCache                              bool
	explain                              bool
	packageGrouping, layoutDirection     string
	skinParams                           stringList
	rendererType                         string
	serverURL                            string
	renderTimeout                        time.Duration
	outputSet, formatSet, excludesSet    bool
	hideFieldsSet, hideMethodsSet        bool
	depthSet                             bool
	rendererTypeSet, serverURLSet        bool
	renderTimeoutSet                     bool
}

func registerClassFlags(fs *flag.FlagSet, o *classOptions) {
	fs.StringVar(&o.output, "output", "", "")
	fs.StringVar(&o.output, "o", "", "")
	fs.StringVar(&o.format, "format", "", "")
	fs.StringVar(&o.format, "f", "", "")
	fs.StringVar(&o.include, "include", "", "")
	fs.StringVar(&o.focus, "focus", "", "")
	fs.StringVar(&o.direction, "direction", "both", "")
	fs.IntVar(&o.depth, "depth", 1, "")
	fs.StringVar(&o.relationKinds, "relations", "", "")
	fs.StringVar(&o.language, "language", "", "")
	fs.Var(&o.excludes, "exclude", "")
	fs.BoolVar(&o.hideFields, "hide-fields", false, "")
	fs.BoolVar(&o.hideMethods, "hide-methods", false, "")
	fs.BoolVar(&o.hidePrivate, "hide-private", false, "")
	fs.BoolVar(&o.noRelations, "no-relations", false, "")
	fs.BoolVar(&o.showRelationLabels, "show-relation-labels", false, "")
	fs.BoolVar(&o.noCache, "no-cache", false, "")
	fs.BoolVar(&o.explain, "explain", false, "")
	fs.StringVar(&o.packageGrouping, "package-grouping", "", "")
	fs.StringVar(&o.layoutDirection, "layout-direction", "", "")
	fs.Var(&o.skinParams, "skinparam", "")
	fs.StringVar(&o.title, "title", "", "")
	fs.StringVar(&o.rendererType, "renderer", "", "")
	fs.StringVar(&o.serverURL, "server-url", "", "")
	fs.DurationVar(&o.renderTimeout, "render-timeout", 0, "")
	fs.StringVar(&o.configPath, "config", o.configPath, "")
	fs.BoolVar(&o.verbose, "verbose", o.verbose, "")
	fs.BoolVar(&o.verbose, "v", o.verbose, "")
	fs.BoolVar(&o.quiet, "quiet", o.quiet, "")
	fs.BoolVar(&o.quiet, "q", o.quiet, "")
}

func parseClassRequest(args []string, inherited commonOptions, diff bool) (classOptions, []string, focus.Direction, error) {
	var o classOptions
	o.commonOptions = inherited
	fs := flag.NewFlagSet("class", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	registerClassFlags(fs, &o)
	normalized, err := normalizeDiagramArgs(args, fs)
	if err != nil {
		return o, nil, "", err
	}
	if err := fs.Parse(normalized); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return o, nil, "", err
		}
		return o, nil, "", cleanFlagError(err)
	}
	if o.verbose && o.quiet {
		return o, nil, "", errors.New("--verbose and --quiet cannot be used together")
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
		case "renderer":
			o.rendererTypeSet = true
		case "server-url":
			o.serverURLSet = true
		case "render-timeout":
			o.renderTimeoutSet = true
		}
	})
	if fs.NArg() > 1 {
		return o, nil, "", errors.New("class accepts at most one target")
	}
	if o.depth < 0 {
		return o, nil, "", errors.New("--depth must be zero or greater")
	}
	if o.depthSet && o.focus == "" && !diff {
		return o, nil, "", errors.New("--depth requires --focus")
	}
	direction, err := focus.ParseDirection(o.direction)
	if err != nil {
		return o, nil, "", err
	}
	if o.direction != "both" && o.focus == "" && !diff {
		return o, nil, "", errors.New("--direction requires --focus")
	}
	if diff && o.focus != "" {
		return o, nil, "", errors.New("--focus cannot be combined with the diff command")
	}
	if o.noRelations && o.relationKinds != "" {
		return o, nil, "", errors.New("--relations and --no-relations cannot be used together")
	}
	return o, fs.Args(), direction, nil
}

func loadClassConfiguration(o classOptions, diff bool) (config.Config, plantuml.Options, string, error) {
	cfg, loadedPath, err := config.Load(o.configPath, o.configPath != "")
	var opts plantuml.Options
	if err != nil {
		return cfg, opts, loadedPath, err
	}
	config.ResolvePaths(&cfg, loadedPath)
	if o.language != "" {
		cfg.Language = strings.ToLower(o.language)
	}
	if o.outputSet {
		cfg.Output.File = o.output
	} else if diff && cfg.Output.File == "class-diagram.puml" {
		cfg.Output.File = "change-diagram.puml"
	}
	if o.formatSet {
		cfg.Output.Format = strings.ToLower(o.format)
	}
	if o.packageGrouping != "" {
		cfg.Layout.PackageGrouping = o.packageGrouping
	}
	if o.layoutDirection != "" {
		cfg.Layout.Direction = o.layoutDirection
	}
	for _, setting := range o.skinParams {
		key, value, ok := strings.Cut(setting, "=")
		if !ok || strings.ContainsAny(setting, "\r\n") {
			return cfg, opts, loadedPath, errors.New("--skinparam requires a single-line key=value")
		}
		if cfg.Layout.SkinParams == nil {
			cfg.Layout.SkinParams = make(map[string]string)
		}
		cfg.Layout.SkinParams[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if o.rendererTypeSet {
		cfg.Renderer.Type = strings.ToLower(o.rendererType)
	}
	if o.serverURLSet {
		cfg.Renderer.ServerURL = o.serverURL
	}
	if o.renderTimeoutSet {
		cfg.Renderer.Timeout = o.renderTimeout
	}
	if o.excludesSet {
		cfg.Exclude = append([]string(nil), o.excludes...)
	}
	if o.watchExcludes != nil {
		cfg.Exclude = o.watchExcludes
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
		enabled, err := parseRelationKinds(o.relationKinds)
		if err != nil {
			return cfg, opts, loadedPath, err
		}
		cfg.Relations.Inheritance = enabled[relations.Inheritance]
		cfg.Relations.Implementation = enabled[relations.Implementation]
		cfg.Relations.FieldDependency = enabled[relations.Field]
		cfg.Relations.ParameterDependency = enabled[relations.Parameter]
		cfg.Relations.ReturnDependency = enabled[relations.Return]
	}
	if cfg.Renderer.Timeout < 0 {
		return cfg, opts, loadedPath, errors.New("--render-timeout must be zero or greater")
	}
	if cfg.Output.Format != "plantuml" && cfg.Output.Format != "svg" && cfg.Output.Format != "png" {
		return cfg, opts, loadedPath, fmt.Errorf("unsupported format: %s\nSupported formats: plantuml, svg, png", cfg.Output.Format)
	}
	opts = plantuml.Options{
		Title: o.title, ShowFields: cfg.Members.Fields, ShowMethods: cfg.Members.Methods,
		ShowPrivate: cfg.Visibility.Private, ShowPublic: cfg.Visibility.Public,
		ShowProtected: cfg.Visibility.Protected, ShowPackage: cfg.Visibility.PackagePrivate,
		ShowRelations: !o.noRelations, Inheritance: cfg.Relations.Inheritance,
		Implementation: cfg.Relations.Implementation, FieldDependency: cfg.Relations.FieldDependency,
		ParamDependency: cfg.Relations.ParameterDependency, ReturnDependency: cfg.Relations.ReturnDependency,
		ShowRelationLabels: o.showRelationLabels,
		PackageGrouping:    cfg.Layout.PackageGrouping, LayoutDirection: cfg.Layout.Direction,
		SkinParams: cfg.Layout.SkinParams,
	}
	return cfg, opts, loadedPath, opts.ValidateLayout()
}
