package focus

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/relations"
)

type Direction string

const (
	Incoming Direction = "in"
	Outgoing Direction = "out"
	Both     Direction = "both"
)

func ParseDirection(value string) (Direction, error) {
	direction := Direction(value)
	switch direction {
	case Incoming, Outgoing, Both:
		return direction, nil
	default:
		return "", fmt.Errorf("unsupported direction: %s\nSupported directions: in, out, both", value)
	}
}

// Apply returns the focused type and connected types within maxDepth.
func Apply(types []model.Type, name string, maxDepth int, direction Direction) ([]model.Type, error) {
	return ApplyMany(types, []string{name}, maxDepth, direction)
}

// ApplyMany focuses the graph around multiple seeds. Seed names may be simple
// names, display names for nested types, or fully qualified names.
func ApplyMany(types []model.Type, names []string, maxDepth int, direction Direction) ([]model.Type, error) {
	if len(names) == 0 {
		return types, nil
	}
	if maxDepth < 0 {
		return nil, fmt.Errorf("--depth must be zero or greater")
	}
	if _, err := ParseDirection(string(direction)); err != nil {
		return nil, err
	}

	qualified := make(map[string]int, len(types))
	simple := make(map[string][]int, len(types))
	display := make(map[string][]int, len(types))
	for i, t := range types {
		qualified[t.QualifiedName()] = i
		simple[t.Name] = append(simple[t.Name], i)
		display[t.DisplayName()] = append(display[t.DisplayName()], i)
	}
	var starts []int
	for _, name := range names {
		start, err := resolveFocus(name, qualified, simple, display, types)
		if err != nil {
			return nil, err
		}
		starts = append(starts, start)
	}

	adjacent := make([]map[int]bool, len(types))
	for i := range adjacent {
		adjacent[i] = map[int]bool{}
	}
	for _, relation := range relations.Build(types) {
		from, fromOK := qualified[relation.From]
		to, toOK := qualified[relation.To]
		if !fromOK || !toOK {
			continue
		}
		if direction == Outgoing || direction == Both {
			adjacent[from][to] = true
		}
		if direction == Incoming || direction == Both {
			adjacent[to][from] = true
		}
	}

	distance := map[int]int{}
	queue := make([]int, 0, len(starts))
	for _, start := range starts {
		if _, exists := distance[start]; exists {
			continue
		}
		distance[start] = 0
		queue = append(queue, start)
	}
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

func resolveFocus(
	name string,
	qualified map[string]int,
	simple, display map[string][]int,
	types []model.Type,
) (int, error) {
	if i, ok := qualified[name]; ok {
		return i, nil
	}
	matches := display[name]
	if len(matches) == 0 {
		matches = simple[name]
	}
	switch len(matches) {
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
