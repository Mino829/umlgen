package packagegraph

import (
	"strings"

	"github.com/Mino829/umlgen/internal/model"
)

func (i *resolverIndex) resolveJavaImport(source Source, imported model.Import) resolutionResult {
	var targets []Target
	if imported.Wildcard {
		targets = i.javaImportOwners(imported)
	} else {
		targets = append(targets, i.javaTypes(imported.Name)...)
		if imported.Static {
			// A static import can name a member rather than a type. Its package
			// comes from a known declaring type, not the spelling of the member.
			if owner, _, ok := javaSplitLast(imported.Name); ok {
				targets = append(targets, i.javaTypes(owner)...)
			}
		}
	}
	for n := range targets {
		targets[n].Type = ""
	}
	return resolveTargets(targets)
}

func (i *resolverIndex) resolveJavaType(source Source, owner model.Type, reference string) resolutionResult {
	// Lexically enclosing declarations belong to this source root. A type in
	// another compilation unit of the same package is considered after imports.
	parts := make([]string, 0, len(owner.Enclosing)+2)
	if owner.Package != "" {
		parts = append(parts, owner.Package)
	}
	parts = append(parts, owner.Enclosing...)
	parts = append(parts, owner.Name)
	packageDepth := 0
	if owner.Package != "" {
		packageDepth = 1
	}
	for depth := len(parts); depth > packageDepth; depth-- {
		name := strings.Join(parts[:depth], ".") + "." + reference
		if targets := i.localTypes(source, name); len(targets) != 0 {
			return resolveTargets(targets)
		}
	}

	first, remainder, qualified := strings.Cut(reference, ".")
	samePackageName := reference
	if owner.Package != "" {
		samePackageName = owner.Package + "." + reference
	}
	for _, declared := range source.Unit.Types {
		if len(declared.Enclosing) == 0 && declared.Name == first {
			return resolveTargets(i.localTypes(source, samePackageName))
		}
	}
	var explicit []Target
	hasExplicit, unknownExplicit := false, false
	for _, imported := range source.Unit.Imports {
		if imported.Wildcard {
			continue
		}
		_, simple, _ := javaSplitLast(imported.Name)
		if simple != first {
			continue
		}
		hasExplicit = true
		name := imported.Name
		if qualified {
			name += "." + remainder
		}
		targets := i.javaTypes(name)
		if len(targets) == 0 {
			unknownExplicit = true
		}
		explicit = append(explicit, targets...)
	}
	if hasExplicit {
		return javaResolveKnown(explicit, unknownExplicit)
	}

	if !qualified || owner.Package != "" {
		if targets := i.localTypes(source, samePackageName); len(targets) != 0 {
			return resolveTargets(targets)
		}
	}

	if qualified {
		if targets := i.javaTypes(reference); len(targets) != 0 {
			return resolveTargets(targets)
		}
	}

	var onDemand []Target
	var ambiguousOwners []Target
	unknownOnDemand := false
	for _, imported := range source.Unit.Imports {
		if !imported.Wildcard {
			continue
		}
		owners := i.javaImportOwners(imported)
		if len(owners) == 0 {
			unknownOnDemand = true
			continue
		}
		if ownership := resolveTargets(owners); ownership.Status == Ambiguous {
			for _, candidate := range ownership.Candidates {
				ambiguousOwners = append(ambiguousOwners, Target{Package: candidate.Package})
			}
		}
		for _, target := range i.javaTypes(imported.Name + "." + reference) {
			for _, importedOwner := range owners {
				if target.Package == importedOwner.Package {
					onDemand = append(onDemand, target)
					break
				}
			}
		}
	}
	// java.lang is an implicit on-demand namespace, never import evidence.
	// Its absence from the source index is not an unknown explicit import.
	onDemand = append(onDemand, i.javaTypes("java.lang."+reference)...)
	if len(ambiguousOwners) != 0 {
		// Parsing a member under only one of several possible owners does not
		// establish which owner the import denotes. Preserve package ownership
		// candidates rather than infer missing member declarations.
		for _, candidate := range onDemand {
			ambiguousOwners = append(ambiguousOwners, Target{Package: candidate.Package})
		}
		return resolveTargets(ambiguousOwners)
	}
	return javaResolveKnown(onDemand, unknownOnDemand)
}

func (i *resolverIndex) javaTypes(name string) []Target {
	return i.types[qualifiedName{Language: "java", Name: name}]
}

// javaImportOwners retains both interpretations of a non-static on-demand
// prefix: a package and a type can share the exact same qualified name.
func (i *resolverIndex) javaImportOwners(imported model.Import) []Target {
	var targets []Target
	if !imported.Static {
		for _, id := range i.packages[qualifiedName{Language: "java", Name: imported.Name}] {
			targets = append(targets, Target{Package: id})
		}
	}
	targets = append(targets, i.javaTypes(imported.Name)...)
	return targets
}

func javaSplitLast(name string) (string, string, bool) {
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		return name[:dot], name[dot+1:], true
	}
	return "", name, false
}

// An unknown import can hide another candidate, but cannot justify an invented
// package ID. Retain known candidates while refusing a false unique resolution.
func javaResolveKnown(targets []Target, unknown bool) resolutionResult {
	result := resolveTargets(targets)
	if unknown && result.Status == Resolved {
		return resolutionResult{Status: Ambiguous, Candidates: []Target{result.Target}}
	}
	return result
}
