package packagegraph

import (
	"slices"
	"strings"

	"github.com/Mino829/umlgen/internal/model"
)

func (i *resolverIndex) resolveGoImport(source Source, imported model.Import) resolutionResult {
	packages := i.packages[qualifiedName{Language: "go", Name: imported.Name}]
	if len(packages) == 0 {
		return resolutionResult{Status: External, Target: Target{Package: PackageID{
			Language: "go", Context: source.Context, Name: imported.Name,
		}}}
	}
	for _, id := range packages {
		if id.Context == source.Context {
			return resolutionResult{Status: Resolved, Target: Target{Package: id}}
		}
	}
	targets := make([]Target, len(packages))
	for index, id := range packages {
		targets[index] = Target{Package: id}
	}
	return resolveTargets(targets)
}

func (i *resolverIndex) resolveGoType(source Source, reference string, inferred bool) resolutionResult {
	// The source unit, not a finalized type's merged imports, owns Go scope.
	id := packageID(source)
	if inferred {
		return i.goCanonicalType(source, reference)
	}
	if !strings.Contains(reference, ".") {
		if local := i.goTypesInPackage(id, id.Name+"."+reference); len(local) != 0 {
			return resolveTargets(local)
		}
		var results []resolutionResult
		unknownDotPackage := false
		for _, imported := range source.Unit.Imports {
			if imported.Alias == "." {
				if len(i.packages[qualifiedName{Language: "go", Name: imported.Name}]) == 0 {
					unknownDotPackage = true
				}
				results = append(results, i.goImportedType(source, imported, reference, false))
			}
		}
		result := mergeGoResolutions(results)
		if unknownDotPackage && result.Status == Resolved {
			// An unavailable dot import may export this name too. A known
			// singleton is evidence, not proof that ownership is unique.
			return resolutionResult{Status: Ambiguous, Candidates: []Target{result.Target}}
		}
		return result
	}

	// A selector's explicit file-local alias is authoritative. Canonical inferred
	// names were handled above without splitting dots inside module paths.
	qualifier, name, selector := strings.Cut(reference, ".")
	selector = selector && !strings.ContainsAny(qualifier, "/") && !strings.ContainsAny(name, "./")
	if selector {
		var results []resolutionResult
		matched := false
		for _, imported := range source.Unit.Imports {
			if imported.Alias == qualifier && imported.Alias != "_" && imported.Alias != "." {
				matched = true
				results = append(results, i.goImportedType(source, imported, name, true))
			}
		}
		if matched {
			return mergeGoResolutions(results)
		}
		for _, imported := range source.Unit.Imports {
			if imported.Alias != "" {
				continue
			}
			// An import path's last component is not its declared package name.
			// Unknown external default names therefore cannot bind selectors.
			packages := i.resolveGoImport(source, imported)
			for _, target := range goResolutionTargets(packages) {
				if slices.Contains(i.packageNames[target.Package], qualifier) {
					matched = true
					results = append(results, i.goImportedType(source, imported, name, false))
					break
				}
			}
		}
		if matched {
			return mergeGoResolutions(results)
		}
	}

	return resolutionResult{Status: Unresolved}
}

func (i *resolverIndex) goCanonicalType(source Source, reference string) resolutionResult {
	targets := i.types[qualifiedName{Language: "go", Name: reference}]
	if len(targets) == 0 {
		return resolutionResult{Status: Unresolved}
	}
	// The index supplies the package boundary; dots in a module path never do.
	packages := i.resolveGoImport(source, model.Import{Name: targets[0].Package.Name})
	return i.goTypesFromPackages(packages, reference)
}

func (i *resolverIndex) goTypesInPackage(id PackageID, name string) []Target {
	var targets []Target
	for _, target := range i.types[qualifiedName{Language: "go", Name: name}] {
		if target.Package == id {
			targets = append(targets, target)
		}
	}
	return targets
}

func (i *resolverIndex) goImportedType(source Source, imported model.Import, name string, allowExternal bool) resolutionResult {
	packages := i.resolveGoImport(source, imported)
	if packages.Status == External {
		if !allowExternal {
			return resolutionResult{Status: Unresolved}
		}
		packages.Target.Type = imported.Name + "." + name
		return packages
	}
	return i.goTypesFromPackages(packages, imported.Name+"."+name)
}

func (i *resolverIndex) goTypesFromPackages(packages resolutionResult, reference string) resolutionResult {
	var targets []Target
	found := false
	for _, target := range goResolutionTargets(packages) {
		types := i.goTypesInPackage(target.Package, reference)
		if len(types) == 0 {
			// Preserve uncertain package ownership rather than selecting another
			// context merely because its snapshot includes this declaration.
			targets = append(targets, target)
			continue
		}
		found = true
		targets = append(targets, types...)
	}
	if !found {
		return resolutionResult{Status: Unresolved}
	}
	return resolveTargets(targets)
}

func goResolutionTargets(result resolutionResult) []Target {
	switch result.Status {
	case Resolved, External:
		return []Target{result.Target}
	case Ambiguous:
		return result.Candidates
	default:
		return nil
	}
}

func mergeGoResolutions(results []resolutionResult) resolutionResult {
	var targets []Target
	for _, result := range results {
		targets = append(targets, goResolutionTargets(result)...)
	}
	resolved := resolveTargets(targets)
	if resolved.Status == Resolved {
		for _, result := range results {
			if result.Status == External && result.Target == resolved.Target {
				resolved.Status = External
				break
			}
		}
	}
	return resolved
}
