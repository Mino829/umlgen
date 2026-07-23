package focus

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/Mino829/umlgen/internal/model"
)

// Apply returns the focused type and all types connected to it within maxDepth.
// Relationships are undirected for navigation, exposing dependencies and dependants.
func Apply(types []model.Type, name string, maxDepth int) ([]model.Type, error) {
	if name == "" {
		return types, nil
	}
	if maxDepth < 0 {
		return nil, fmt.Errorf("--depth must be zero or greater")
	}
	qualified := make(map[string]int, len(types))
	simple := make(map[string][]int, len(types))
	for i, t := range types {
		qualified[t.QualifiedName()] = i
		simple[t.Name] = append(simple[t.Name], i)
	}
	start, err := resolveFocus(name, qualified, simple, types)
	if err != nil {
		return nil, err
	}

	adjacent := make([]map[int]bool, len(types))
	for i := range adjacent {
		adjacent[i] = map[int]bool{}
	}
	for i, t := range types {
		for _, raw := range relationTypes(t) {
			for _, ref := range typeReferences(raw) {
				target := resolveReference(ref, t.Package, qualified, simple)
				if target < 0 || target == i {
					continue
				}
				adjacent[i][target] = true
				adjacent[target][i] = true
			}
		}
	}

	distance := map[int]int{start: 0}
	queue := []int{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if distance[current] == maxDepth {
			continue
		}
		var neighbors []int
		for next := range adjacent[current] {
			neighbors = append(neighbors, next)
		}
		sort.Ints(neighbors)
		for _, next := range neighbors {
			if _, seen := distance[next]; seen {
				continue
			}
			distance[next] = distance[current] + 1
			queue = append(queue, next)
		}
	}

	result := make([]model.Type, 0, len(distance))
	for i, t := range types {
		if _, keep := distance[i]; keep {
			result = append(result, t)
		}
	}
	return result, nil
}

func resolveFocus(name string, qualified map[string]int, simple map[string][]int, types []model.Type) (int, error) {
	if i, ok := qualified[name]; ok {
		return i, nil
	}
	switch matches := simple[name]; len(matches) {
	case 0:
		return -1, fmt.Errorf("focus type not found: %s", name)
	case 1:
		return matches[0], nil
	default:
		var names []string
		for _, i := range matches {
			names = append(names, types[i].QualifiedName())
		}
		sort.Strings(names)
		return -1, fmt.Errorf("focus type %q is ambiguous; use one of: %s", name, strings.Join(names, ", "))
	}
}

func resolveReference(ref, pkg string, qualified map[string]int, simple map[string][]int) int {
	if i, ok := qualified[ref]; ok {
		return i
	}
	if i, ok := qualified[pkg+"."+ref]; ok {
		return i
	}
	if dot := strings.LastIndex(ref, "."); dot >= 0 {
		ref = ref[dot+1:]
	}
	if matches := simple[ref]; len(matches) == 1 {
		return matches[0]
	}
	return -1
}

func relationTypes(t model.Type) []string {
	result := append([]string{}, t.Extends...)
	result = append(result, t.Implements...)
	for _, f := range t.Fields {
		result = append(result, f.Type)
	}
	for _, m := range t.Methods {
		if !m.Constructor {
			result = append(result, m.ReturnType)
		}
		for _, p := range m.Parameters {
			result = append(result, p.Type)
		}
	}
	return result
}

func typeReferences(text string) []string {
	runes := []rune(text)
	var result []string
	for i := 0; i < len(runes); {
		if !unicode.IsLetter(runes[i]) && runes[i] != '_' && runes[i] != '$' {
			i++
			continue
		}
		start := i
		for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) ||
			runes[i] == '_' || runes[i] == '$' || runes[i] == '.') {
			i++
		}
		result = append(result, string(runes[start:i]))
	}
	return result
}
