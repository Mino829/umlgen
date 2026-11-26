package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Mino829/umlgen/internal/config"
	"github.com/Mino829/umlgen/internal/gitsnapshot"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

func runRelationshipDiff(rev string, args []string, inherited commonOptions, stdout, stderr io.Writer) (int, error) {
	flags := flag.NewFlagSet("relationships", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var excludes stringList
	var configured, include, kinds string
	var details, noCache, excludesSet bool
	flags.StringVar(&configured, "language", "", "")
	flags.StringVar(&include, "include", "", "")
	flags.StringVar(&kinds, "relations", "", "")
	flags.StringVar(&inherited.configPath, "config", inherited.configPath, "")
	flags.Var(&excludes, "exclude", "")
	flags.BoolVar(&details, "explain", false, "")
	flags.BoolVar(&noCache, "no-cache", false, "")
	flags.BoolVar(&inherited.quiet, "quiet", inherited.quiet, "")
	flags.BoolVar(&inherited.quiet, "q", inherited.quiet, "")
	flags.BoolVar(&inherited.verbose, "verbose", inherited.verbose, "")
	flags.BoolVar(&inherited.verbose, "v", inherited.verbose, "")
	normalized, err := normalizeDiagramArgs(args, flags)
	if err != nil {
		return exitArgs, err
	}
	if err := flags.Parse(normalized); err != nil {
		return exitArgs, cleanFlagError(err)
	}
	if flags.NArg() > 1 || inherited.quiet && inherited.verbose {
		return exitArgs, errors.New("relationship report accepts one target and mutually exclusive --quiet/--verbose")
	}
	cfg, loaded, err := config.Load(inherited.configPath, inherited.configPath != "")
	if err != nil {
		return exitArgs, err
	}
	config.ResolvePaths(&cfg, loaded)
	flags.Visit(func(f *flag.Flag) { excludesSet = excludesSet || f.Name == "exclude" })
	if excludesSet {
		cfg.Exclude = excludes
	}
	if configured != "" {
		cfg.Language = strings.ToLower(strings.TrimSpace(configured))
	}
	enabled := map[relations.Kind]bool{
		relations.Inheritance: cfg.Relations.Inheritance, relations.Implementation: cfg.Relations.Implementation,
		relations.Field: cfg.Relations.FieldDependency, relations.Parameter: cfg.Relations.ParameterDependency,
		relations.Return: cfg.Relations.ReturnDependency,
	}
	if kinds != "" {
		enabled, err = parseRelationKinds(kinds)
		if err != nil {
			return exitArgs, err
		}
	}
	targets := cfg.Source
	if flags.NArg() != 0 {
		targets = flags.Args()
	}
	pair, err := gitsnapshot.Load(rev, targets, cfg.Exclude)
	if err != nil {
		return exitError, err
	}
	backend, err := snapshotLanguage(cfg.Language, pair, "relationship report")
	if err != nil {
		return exitArgs, err
	}
	analysis := newSourceAnalysis(sourceAnalysisOptions{Config: cfg, Parser: backend, NoCache: noCache, Verbose: inherited.verbose}, stderr, stderr)
	before, err := relationshipSnapshot(pair.Before, analysis, include, cfg.Exclude, enabled, details, stderr)
	if err != nil {
		return exitParse, err
	}
	after, err := relationshipSnapshot(pair.After, analysis, include, cfg.Exclude, enabled, details, stderr)
	if err != nil {
		return exitParse, err
	}
	analysis.reportCache()
	if err := writeRelationshipDiff(stdout, pair, backend.DisplayName(), before, after, details); err != nil {
		return exitOutput, err
	}
	if before.failed+after.failed != 0 {
		return exitParse, errors.New("relationship report is incomplete because selected sources failed to parse")
	}
	return exitOK, nil
}

type relationshipEdge struct {
	from, to string
	kind     relations.Kind
}

type relationshipOwner struct {
	name string
	kind relations.Kind
}

type relationshipProof struct {
	evidence            relations.Evidence
	label, multiplicity string
}

type relationshipSide struct {
	edges      map[relationshipEdge][]relationshipProof
	unresolved map[relationshipOwner]bool
	sources    map[string][]byte
	parsed     int
	failed     int
}

func relationshipSnapshot(snapshot gitsnapshot.Snapshot, analysis *sourceAnalysis, include string, excludes []string, enabled map[relations.Kind]bool, details bool, stderr io.Writer) (relationshipSide, error) {
	result := relationshipSide{edges: make(map[relationshipEdge][]relationshipProof), unresolved: make(map[relationshipOwner]bool)}
	if details {
		result.sources = make(map[string][]byte)
	}
	var known, selected []model.Type
	identities := make(map[string]string)
	goPackages := make(map[string]string)
	for _, file := range snapshot.Files {
		if !analysis.parser.MatchesPath(file.Path) {
			continue
		}
		parsed := analysis.parseSnapshotSource(file.Path, file.Content)
		if parsed.Err != nil {
			result.failed++
			fmt.Fprintf(stderr, "Warning: failed to parse %q (Git %s): %v\n", file.Path, snapshot.Revision, parsed.Err)
			continue
		}
		result.parsed++
		unit := parsed.Unit
		snapshot.BindContext(unit)
		if details {
			result.sources[file.Path] = file.Content
		}
		if !includePackage(unit.PackagePath, include) || excludePackage(unit.PackagePath, excludes) {
			known = append(known, unit.Types...)
			continue
		}
		if unit.Language == "go" {
			if prior, exists := goPackages[unit.PackagePath]; exists && prior != unit.PackageContext {
				return result, fmt.Errorf("Git %s has ambiguous Go package %q in %q and %q", snapshot.Revision, unit.PackagePath, prior, unit.PackageContext)
			}
			goPackages[unit.PackagePath] = unit.PackageContext
		}
		for _, declaration := range unit.Types {
			name := declaration.QualifiedName()
			if prior, exists := identities[name]; exists {
				return result, fmt.Errorf("Git %s has ambiguous type %q in %q and %q", snapshot.Revision, name, prior, file.Path)
			}
			identities[name] = file.Path
		}
		selected = append(selected, unit.Types...)
	}
	selected = analysis.parser.Finalize(selected, true)
	if len(known) > 0 {
		known = append(known, selected...)
	}
	graph := relations.Analyze(relations.Input{Types: selected, Known: known}, true)
	for _, edge := range graph.Relations {
		if enabled[edge.Kind] {
			key := relationshipEdge{edge.From, edge.To, edge.Kind}
			for _, evidence := range edge.Evidence {
				result.edges[key] = append(result.edges[key], relationshipProof{evidence, edge.Label, edge.Multiplicity})
			}
		}
	}
	for _, ref := range graph.Diagnostics {
		if enabled[ref.Kind] && ref.Status == relations.StatusUnresolved {
			result.unresolved[relationshipOwner{ref.From, ref.Kind}] = true
		}
	}
	return result, nil
}

func sameRelationshipProofs(before, after []relationshipProof) bool {
	if len(before) != len(after) {
		return false
	}
	if len(before) == 1 {
		a, b := before[0], after[0]
		a.evidence.Line, b.evidence.Line = 0, 0
		return a == b
	}
	counts := make(map[relationshipProof]int, len(before))
	for _, proof := range before {
		proof.evidence.Line = 0
		counts[proof]++
	}
	for _, proof := range after {
		proof.evidence.Line = 0
		if counts[proof] == 0 {
			return false
		}
		counts[proof]--
	}
	return true
}

func writeRelationshipDiff(out io.Writer, pair gitsnapshot.Pair, language string, before, after relationshipSide, details bool) error {
	var report strings.Builder
	fmt.Fprintf(&report, "Class relationship changes (%s)\nBefore: %s (%d parsed, %d failed)\nAfter: %s (%d parsed, %d failed)\n", language,
		pair.Before.Revision, before.parsed, before.failed, pair.After.Revision, after.parsed, after.failed)
	incomplete := before.failed+after.failed > 0
	if incomplete {
		fmt.Fprintln(&report, "Incomplete analysis: candidate changes are uncertain.")
	}
	if len(before.unresolved)+len(after.unresolved) > 0 {
		fmt.Fprintln(&report, "Unresolved references: changes from affected types of the same relationship kind are uncertain.")
	}
	if before.failed > 0 && before.parsed == 0 || after.failed > 0 && after.parsed == 0 {
		fmt.Fprintln(&report, "No comparison: at least one snapshot could not parse any selected source.")
	} else {
		keys := make([]relationshipEdge, 0, len(before.edges)+len(after.edges))
		for key := range before.edges {
			keys = append(keys, key)
		}
		for key := range after.edges {
			if _, exists := before.edges[key]; !exists {
				keys = append(keys, key)
			}
		}
		slices.SortFunc(keys, func(a, b relationshipEdge) int {
			if a.from != b.from {
				return strings.Compare(a.from, b.from)
			}
			if a.to != b.to {
				return strings.Compare(a.to, b.to)
			}
			return strings.Compare(string(a.kind), string(b.kind))
		})
		changed := false
		for _, key := range keys {
			old, current := before.edges[key], after.edges[key]
			if sameRelationshipProofs(old, current) {
				continue
			}
			status := "evidence-changed"
			if len(old) == 0 {
				status = "added"
			} else if len(current) == 0 {
				status = "removed"
			}
			owner := relationshipOwner{key.from, key.kind}
			if incomplete || before.unresolved[owner] || after.unresolved[owner] {
				status = "uncertain"
			}
			changed = true
			fmt.Fprintf(&report, "%s %s -> %s [%s] (evidence: %d -> %d)\n", status, key.from, key.to, key.kind, len(old), len(current))
			if details {
				writeRelationshipProofs(&report, "before", pair.Root, old, before.sources)
				writeRelationshipProofs(&report, "after", pair.Root, current, after.sources)
			}
		}
		if !changed {
			fmt.Fprintln(&report, "No relationship changes detected in the analyzed scope.")
		}
	}
	_, err := io.WriteString(out, report.String())
	return err
}

func writeRelationshipProofs(out *strings.Builder, side, root string, proofs []relationshipProof, sources map[string][]byte) {
	for _, proof := range proofs {
		evidence := proof.evidence
		path, err := filepath.Rel(root, evidence.Source)
		if err != nil {
			path = evidence.Source
		}
		fmt.Fprintf(out, "  %s %q:%d member=%q type=%q inferred=%t\n", side, filepath.ToSlash(path), evidence.Line, evidence.Member, evidence.Type, evidence.Inferred)
		if evidence.Line < 1 {
			continue
		}
		n := 0
		for line := range bytes.SplitSeq(bytes.TrimSuffix(sources[evidence.Source], []byte{'\n'}), []byte{'\n'}) {
			n++
			if n >= evidence.Line+3 {
				break
			}
			if n >= evidence.Line {
				fmt.Fprintf(out, "    %d | %s\n", n, bytes.TrimSuffix(line, []byte{'\r'}))
			}
		}
	}
}
