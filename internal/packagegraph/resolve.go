package packagegraph

import (
	"cmp"
	"slices"

	"github.com/Mino829/umlgen/internal/model"
)

type qualifiedName struct {
	Language string
	Name     string
}

type resolverIndex struct {
	types        map[qualifiedName][]Target
	packages     map[qualifiedName][]PackageID
	packageNames map[PackageID][]string
	aliases      map[Target]struct{}
}

type resolutionResult struct {
	Status     Resolution
	Target     Target
	Candidates []Target
}

func packageID(source Source) PackageID {
	name := source.Unit.Package.Name
	if source.Unit.Language == "go" {
		name = source.Unit.PackagePath
	}
	return PackageID{Language: source.Unit.Language, Context: source.Context, Name: name}
}

func newResolverIndex(sources []Source) *resolverIndex {
	index := &resolverIndex{
		types: make(map[qualifiedName][]Target), packages: make(map[qualifiedName][]PackageID),
		packageNames: make(map[PackageID][]string),
	}
	for _, source := range sources {
		if !hasPackage(source.Unit) {
			continue
		}
		id := packageID(source)
		key := qualifiedName{Language: id.Language, Name: id.Name}
		if !slices.Contains(index.packages[key], id) {
			index.packages[key] = append(index.packages[key], id)
		}
		if !slices.Contains(index.packageNames[id], source.Unit.Package.Name) {
			index.packageNames[id] = append(index.packageNames[id], source.Unit.Package.Name)
		}
		for _, typ := range source.Unit.Types {
			if typ.Kind == model.Alias {
				if index.aliases == nil {
					index.aliases = make(map[Target]struct{})
				}
				typ.Package = id.Name
				index.aliases[Target{Package: id, Type: typ.QualifiedName()}] = struct{}{}
				continue
			}
			if typ.ReceiverOnly {
				continue
			}
			typ.Package = id.Name
			key := qualifiedName{Language: id.Language, Name: typ.QualifiedName()}
			target := Target{Package: id, Type: key.Name}
			if !slices.Contains(index.types[key], target) {
				index.types[key] = append(index.types[key], target)
			}
		}
	}
	return index
}

func hasPackage(unit *model.SourceUnit) bool {
	for _, diagnostic := range unit.Diagnostics {
		if diagnostic.Code == "syntax-error" || diagnostic.Code == "unsupported-module-declaration" {
			return false
		}
	}
	return true
}

func (i *resolverIndex) localTypes(source Source, name string) []Target {
	var targets []Target
	for _, target := range i.types[qualifiedName{Language: source.Unit.Language, Name: name}] {
		if target.Package.Context == source.Context {
			targets = append(targets, target)
		}
	}
	return targets
}

func (i *resolverIndex) projectsType(source Source, typ model.Type) bool {
	if typ.Kind == model.Alias {
		return false
	}
	if !typ.ReceiverOnly {
		return true
	}
	id := packageID(source)
	typ.Package = id.Name
	_, alias := i.aliases[Target{Package: id, Type: typ.QualifiedName()}]
	return !alias
}

// resolveTargets retains all distinct candidates rather than choosing map or
// input order. A singleton is resolved; no target means genuinely unresolved.
func resolveTargets(targets []Target) resolutionResult {
	if len(targets) == 0 {
		return resolutionResult{Status: Unresolved}
	}
	if len(targets) == 1 {
		return resolutionResult{Status: Resolved, Target: targets[0]}
	}
	unique := make(map[Target]struct{}, len(targets))
	for _, target := range targets {
		unique[target] = struct{}{}
	}
	if len(unique) == 1 {
		return resolutionResult{Status: Resolved, Target: targets[0]}
	}
	candidates := make([]Target, 0, len(unique))
	for target := range unique {
		candidates = append(candidates, target)
	}
	slices.SortFunc(candidates, compareTarget)
	return resolutionResult{Status: Ambiguous, Candidates: candidates}
}

func comparePackageID(a, b PackageID) int {
	if order := cmp.Compare(a.Language, b.Language); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Context, b.Context); order != 0 {
		return order
	}
	return cmp.Compare(a.Name, b.Name)
}

func compareTarget(a, b Target) int {
	if order := comparePackageID(a.Package, b.Package); order != 0 {
		return order
	}
	return cmp.Compare(a.Type, b.Type)
}
