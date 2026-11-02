package packagegraph

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

type edgeKey struct{ from, to PackageID }
type relationIdentity struct {
	from, to            string
	kind                relations.Kind
	label, multiplicity string
}
type importIdentity struct {
	file File
	span model.SourceSpan
}
type diagnosticIdentity struct {
	from            PackageID
	status          Resolution
	code, reference string
	file            File
	span            model.SourceSpan
	line            int
	kind            relations.Kind
	member          string
	owner           string
}
type dependencyBuilder struct {
	Dependency
	types   map[relationIdentity]int
	imports map[importIdentity]struct{}
}

type graphBuilder struct {
	graph       Graph
	packages    map[PackageID]int
	edges       map[edgeKey]*dependencyBuilder
	diagnostics map[diagnosticIdentity]struct{}
}

// Build constructs only dependencies represented by the selected input syntax.
// Empty mode selects declared-types. Source contexts must identify their actual
// analysis root/module; the graph never guesses them from a package name.
// Inferred implementation records, when supplied in unit Types, retain their
// Evidence.Inferred flag. Build does not run a compiler or infer new matches.
func Build(sources []Source, mode DependencySource) (Graph, error) {
	if mode == "" {
		mode = DeclaredTypes
	}
	if mode != DeclaredTypes && mode != Imports && mode != Both {
		return Graph{}, fmt.Errorf("unsupported dependency source %q", mode)
	}
	ordered, err := prepareSources(sources)
	if err != nil {
		return Graph{}, err
	}
	builder := graphBuilder{
		graph: Graph{Source: mode}, packages: make(map[PackageID]int),
		edges: make(map[edgeKey]*dependencyBuilder), diagnostics: make(map[diagnosticIdentity]struct{}),
	}
	index := newResolverIndex(ordered)
	for _, source := range ordered {
		id := packageID(source)
		file := sourceFile(source)
		for _, diagnostic := range source.Unit.Diagnostics {
			builder.addDiagnostic(Diagnostic{
				From: id, Status: Unresolved, Code: diagnostic.Code, File: file, Span: diagnostic.Span,
			})
		}
		if !hasPackage(source.Unit) {
			continue
		}
		position := builder.addPackage(id, InternalPackage)
		builder.graph.Packages[position].Files = append(builder.graph.Packages[position].Files, file)
	}
	for _, source := range ordered {
		if !hasPackage(source.Unit) {
			continue
		}
		if mode == Imports || mode == Both {
			builder.addImports(index, source)
		}
		if mode == DeclaredTypes || mode == Both {
			builder.addTypes(index, source)
		}
	}
	return builder.finish(), nil
}

func prepareSources(sources []Source) ([]Source, error) {
	type fileID struct{ language, context, path string }
	seen := make(map[fileID]string, len(sources))
	ordered := make([]Source, 0, len(sources))
	var language string
	for _, source := range sources {
		u := source.Unit
		if u == nil || source.Context == "" || u.Path == "" || u.ContentHash == "" {
			return nil, fmt.Errorf("package graph requires a source context, path and content hash")
		}
		if u.Language != "java" && u.Language != "go" {
			return nil, fmt.Errorf("unsupported source language %q", u.Language)
		}
		if language != "" && language != u.Language {
			return nil, fmt.Errorf("package graph requires a single source language")
		}
		language = u.Language
		if u.Language == "go" && hasPackage(u) && u.PackagePath == "" {
			return nil, fmt.Errorf("Go source %s has no bound package path", u.Path)
		}
		id := fileID{u.Language, source.Context, u.Path}
		if hash, exists := seen[id]; exists {
			if hash != u.ContentHash {
				return nil, fmt.Errorf("conflicting source snapshots for %s", u.Path)
			}
			continue
		}
		seen[id] = u.ContentHash
		ordered = append(ordered, source)
	}
	slices.SortFunc(ordered, func(a, b Source) int {
		if order := cmp.Compare(a.Context, b.Context); order != 0 {
			return order
		}
		return cmp.Compare(a.Unit.Path, b.Unit.Path)
	})
	return ordered, nil
}

func sourceFile(source Source) File {
	return File{Path: source.Unit.Path, ContentHash: source.Unit.ContentHash}
}

func (b *graphBuilder) addPackage(id PackageID, kind PackageKind) int {
	if position, exists := b.packages[id]; exists {
		return position
	}
	position := len(b.graph.Packages)
	b.packages[id] = position
	b.graph.Packages = append(b.graph.Packages, Package{ID: id, Kind: kind})
	return position
}

func (b *graphBuilder) dependency(from PackageID, target Target, status Resolution) *dependencyBuilder {
	if from == target.Package {
		return nil
	}
	kind := InternalPackage
	if status == External {
		kind = ExternalPackage
	}
	b.addPackage(target.Package, kind)
	key := edgeKey{from, target.Package}
	edge := b.edges[key]
	if edge == nil {
		edge = &dependencyBuilder{
			Dependency: Dependency{From: from, To: target.Package},
			types:      make(map[relationIdentity]int), imports: make(map[importIdentity]struct{}),
		}
		b.edges[key] = edge
	}
	return edge
}

func (b *graphBuilder) addImports(index *resolverIndex, source Source) {
	from, file := packageID(source), sourceFile(source)
	for _, imported := range source.Unit.Imports {
		var result resolutionResult
		if source.Unit.Language == "go" {
			result = index.resolveGoImport(source, imported)
		} else {
			result = index.resolveJavaImport(source, imported)
		}
		if result.Status != Resolved && result.Status != External {
			b.addDiagnostic(Diagnostic{
				From: from, Status: result.Status, Code: string(result.Status) + "-import",
				Reference: imported.Name, File: file, Span: imported.Span, Candidates: result.Candidates,
			})
			continue
		}
		edge := b.dependency(from, result.Target, result.Status)
		if edge == nil {
			continue
		}
		key := importIdentity{file, imported.Span}
		if _, exists := edge.imports[key]; !exists {
			edge.imports[key] = struct{}{}
			edge.Imports = append(edge.Imports, ImportEvidence{File: file, Import: imported})
		}
	}
}

func (b *graphBuilder) addTypes(index *resolverIndex, source Source) {
	type lookup struct {
		owner, reference string
		inferred         bool
	}
	resolved := make(map[lookup]resolutionResult)
	// Relations use string endpoints; private tokens preserve contextual target
	// identities even when two roots contain the same fully qualified type name.
	tokens := make(map[Target]string)
	targets := make(map[string]resolutionResult)
	resolve := func(owner model.Type, reference string, inferred bool) (string, bool) {
		reference = relations.Base(reference)
		key := lookup{owner.QualifiedName(), reference, inferred}
		result, exists := resolved[key]
		if !exists {
			if source.Unit.Language == "go" {
				result = index.resolveGoType(source, reference, inferred)
			} else {
				result = index.resolveJavaType(source, owner, reference)
			}
			resolved[key] = result
		}
		if result.Status != Resolved && result.Status != External {
			return "", false
		}
		token, exists := tokens[result.Target]
		if !exists {
			token = "@" + strconv.Itoa(len(tokens))
			tokens[result.Target] = token
			targets[token] = result
		}
		return token, true
	}
	analysis := relations.AnalyzeWithResolver(source.Unit.Types, resolve)
	from, file := packageID(source), sourceFile(source)
	for _, diagnostic := range analysis.Diagnostics {
		result := resolved[lookup{diagnostic.From, diagnostic.Reference, diagnostic.Evidence.Inferred}]
		b.addDiagnostic(Diagnostic{
			From: from, Status: result.Status, Code: string(result.Status) + "-type", Reference: diagnostic.Reference,
			File: file, Line: diagnostic.Evidence.Line, Kind: diagnostic.Kind,
			Member: diagnostic.Evidence.Member, Owner: diagnostic.From, Candidates: result.Candidates,
		})
	}
	for _, relation := range analysis.Relations {
		result := targets[relation.To]
		edge := b.dependency(from, result.Target, result.Status)
		if edge == nil {
			continue
		}
		relation.To = result.Target.Type
		key := relationIdentity{relation.From, relation.To, relation.Kind, relation.Label, relation.Multiplicity}
		if position, exists := edge.types[key]; exists {
			stored := &edge.TypeRelations[position]
			for _, evidence := range relation.Evidence {
				if !slices.Contains(stored.Evidence, evidence) {
					stored.Evidence = append(stored.Evidence, evidence)
				}
			}
		} else {
			edge.types[key] = len(edge.TypeRelations)
			edge.TypeRelations = append(edge.TypeRelations, relation)
		}
	}
}

func (b *graphBuilder) addDiagnostic(diagnostic Diagnostic) {
	key := diagnosticIdentity{
		diagnostic.From, diagnostic.Status, diagnostic.Code, diagnostic.Reference, diagnostic.File,
		diagnostic.Span, diagnostic.Line, diagnostic.Kind, diagnostic.Member, diagnostic.Owner,
	}
	if _, exists := b.diagnostics[key]; !exists {
		b.diagnostics[key] = struct{}{}
		b.graph.Diagnostics = append(b.graph.Diagnostics, diagnostic)
	}
}

func (b *graphBuilder) finish() Graph {
	for _, edge := range b.edges {
		slices.SortFunc(edge.TypeRelations, compareRelation)
		for i := range edge.TypeRelations {
			slices.SortFunc(edge.TypeRelations[i].Evidence, compareEvidence)
		}
		slices.SortFunc(edge.Imports, func(a, b ImportEvidence) int {
			if order := cmp.Compare(a.File.Path, b.File.Path); order != 0 {
				return order
			}
			return cmp.Compare(a.Import.Span.Start.Offset, b.Import.Span.Start.Offset)
		})
		b.graph.Dependencies = append(b.graph.Dependencies, edge.Dependency)
	}
	slices.SortFunc(b.graph.Packages, func(a, b Package) int { return comparePackageID(a.ID, b.ID) })
	slices.SortFunc(b.graph.Dependencies, func(a, b Dependency) int {
		if order := comparePackageID(a.From, b.From); order != 0 {
			return order
		}
		return comparePackageID(a.To, b.To)
	})
	slices.SortFunc(b.graph.Diagnostics, compareDiagnostic)
	return b.graph
}

func compareRelation(a, b relations.Relation) int {
	if order := cmp.Compare(a.From, b.From); order != 0 {
		return order
	}
	if order := cmp.Compare(a.To, b.To); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Kind, b.Kind); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Label, b.Label); order != 0 {
		return order
	}
	return cmp.Compare(a.Multiplicity, b.Multiplicity)
}

func compareEvidence(a, b relations.Evidence) int {
	if order := cmp.Compare(a.Source, b.Source); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Line, b.Line); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Member, b.Member); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Type, b.Type); order != 0 {
		return order
	}
	if a.Inferred != b.Inferred {
		if a.Inferred {
			return 1
		}
		return -1
	}
	return 0
}

func compareDiagnostic(a, b Diagnostic) int {
	if order := comparePackageID(a.From, b.From); order != 0 {
		return order
	}
	if order := cmp.Compare(a.File.Path, b.File.Path); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Span.Start.Offset, b.Span.Start.Offset); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Line, b.Line); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Owner, b.Owner); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Kind, b.Kind); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Member, b.Member); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Reference, b.Reference); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Status, b.Status); order != 0 {
		return order
	}
	return cmp.Compare(a.Code, b.Code)
}
