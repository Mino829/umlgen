package sequence

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrAmbiguousEntry marks a selector matching more than one declaration.
var ErrAmbiguousEntry = errors.New("ambiguous sequence entry")

// Select resolves a canonical ID, qualified signature/name, or simple name.
// Less-specific forms never silently choose among multiple declarations.
func Select(program Program, selector string) (Callable, error) {
	for tier := range 3 {
		count, index := 0, 0
		for i := range program.Callables {
			if entryMatches(&program.Callables[i], selector, tier) {
				count++
				index = i
			}
		}
		if count == 1 {
			return program.Callables[index], nil
		}
		if count > 1 {
			ids := make([]string, 0, count)
			for i := range program.Callables {
				if entryMatches(&program.Callables[i], selector, tier) {
					ids = append(ids, program.Callables[i].ID)
				}
			}
			sort.Strings(ids)
			return Callable{}, fmt.Errorf("%w %q; canonical candidates: %s", ErrAmbiguousEntry, selector, strings.Join(ids, ", "))
		}
	}
	ids := make([]string, len(program.Callables))
	for i := range program.Callables {
		ids[i] = program.Callables[i].ID
	}
	sort.Strings(ids)
	return Callable{}, fmt.Errorf("sequence entry %q not found; canonical candidates: %s", selector, strings.Join(ids, ", "))
}

func entryMatches(c *Callable, selector string, tier int) bool {
	return selector != "" && ((tier == 0 && c.ID == selector) ||
		(tier == 1 && (c.Selector == selector || c.Qualified == selector)) ||
		(tier == 2 && c.Name == selector))
}
