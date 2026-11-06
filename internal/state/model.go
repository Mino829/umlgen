// Package state separates explicitly configured machines from source assignment
// candidates. An assignment never establishes its previous state or a transition.
package state

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Mino829/umlgen/internal/activity"
	"github.com/Mino829/umlgen/internal/model"
	"github.com/Mino829/umlgen/internal/sequence"
)

var ErrSelection = errors.New("state target selection failed")
var ErrAmbiguous = errors.New("ambiguous state target")

type Target struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Qualified string           `json:"qualified"`
	Field     string           `json:"field"`
	FieldType string           `json:"field_type"`
	File      string           `json:"file"`
	Span      model.SourceSpan `json:"span"`
}

// Key is a language-local semantic constant identity: enum symbols use their
// canonical declaration ID; scalar constants use their exact compiler value.
// Name/Qualified/ID are selector spellings; Literal is a source-display spelling.
type Value struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Qualified string           `json:"qualified"`
	Key       string           `json:"key"`
	Literal   string           `json:"literal"`
	File      string           `json:"file"`
	Span      model.SourceSpan `json:"span"`
}

type Update struct {
	Kind       string           `json:"kind"`
	Expression string           `json:"expression"`
	ValueKey   string           `json:"value_key"`
	Receiver   string           `json:"receiver"`
	FunctionID string           `json:"function_id"`
	File       string           `json:"file"`
	Span       model.SourceSpan `json:"span"`
	Guards     []string         `json:"guards"`
	NodeIDs    []int            `json:"node_ids"`
}

type Analysis struct {
	Language    string                `json:"language"`
	Backend     string                `json:"backend"`
	Files       []sequence.File       `json:"files"`
	Target      Target                `json:"target"`
	Values      []Value               `json:"values"`
	Updates     []Update              `json:"updates"`
	Functions   []activity.Graph      `json:"functions"`
	Diagnostics []sequence.Diagnostic `json:"diagnostics"`
}

// SelectTarget uses the same tiered identity policy as callable selection:
// canonical context ID, qualified name, then unique simple name.
func SelectTarget(targets []Target, selector string) (Target, error) {
	for tier := 0; tier < 3; tier++ {
		count, index := 0, 0
		for i := range targets {
			target := &targets[i]
			if selector != "" && ((tier == 0 && target.ID == selector) || (tier == 1 && target.Qualified == selector) || (tier == 2 && target.Name == selector)) {
				count++
				index = i
			}
		}
		if count == 1 {
			return targets[index], nil
		}
		if count > 1 {
			ids := make([]string, 0, count)
			for i := range targets {
				target := &targets[i]
				if (tier == 0 && target.ID == selector) || (tier == 1 && target.Qualified == selector) || (tier == 2 && target.Name == selector) {
					ids = append(ids, target.ID)
				}
			}
			sort.Strings(ids)
			return Target{}, fmt.Errorf("%w: %w %q; candidates: %s", ErrSelection, ErrAmbiguous, selector, strings.Join(ids, ", "))
		}
	}
	ids := make([]string, len(targets))
	for i := range targets {
		ids[i] = targets[i].ID
	}
	sort.Strings(ids)
	return Target{}, fmt.Errorf("%w: type %q not found; candidates: %s", ErrSelection, selector, strings.Join(ids, ", "))
}
