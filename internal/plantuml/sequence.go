package plantuml

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Mino829/umlgen/internal/sequence"
)

// SequenceOptions intentionally contains no class/package layout controls.
type SequenceOptions struct {
	Title string
}

// GenerateSequence renders only the supplied bounded static source projection.
func GenerateSequence(view sequence.View, opts SequenceOptions) (string, error) {
	aliases := make(map[string]string, len(view.Participants))
	labels := sequenceParticipantLabels(view.Participants)
	var out strings.Builder
	out.WriteString("@startuml\n")
	if opts.Title != "" {
		fmt.Fprintf(&out, "title %s\n", packageText(opts.Title))
	}
	for i, participant := range view.Participants {
		if participant.ID == "" {
			return "", fmt.Errorf("sequence participant has an empty ID")
		}
		if _, exists := aliases[participant.ID]; exists {
			return "", fmt.Errorf("duplicate sequence participant %q", participant.ID)
		}
		alias := "s" + strconv.Itoa(i)
		aliases[participant.ID] = alias
		fmt.Fprintf(&out, "participant \"%s\" as %s\n", packageText(labels[i]), alias)
	}
	renderer := sequenceRenderer{out: &out, aliases: aliases, paths: sequenceSourcePaths(view.Program.Files), callables: view.Program.Callables}
	if err := renderer.steps(view.Steps); err != nil {
		return "", err
	}
	out.WriteString("\nlegend bottom\nStatic selected-source projection; NOT a runtime trace\n")
	fmt.Fprintf(&out, "Entry: %s\nBackend: %s; language: %s; selected files: %d\n", packageText(view.Entry.Selector), packageText(view.Program.Backend), packageText(view.Program.Language), len(view.Program.Files))
	fmt.Fprintf(&out, "Limits: call depth %d; participants %d; messages %d\n", view.Limits.MaxCallDepth, view.Limits.MaxParticipants, view.Limits.MaxMessages)
	fmt.Fprintf(&out, "Projected messages: %d; visible omissions: %d; selected-source diagnostics: %d\n", view.Messages, view.Omissions, len(view.Program.Diagnostics))
	out.WriteString("Alternatives and loops are source regions, not observed paths or iteration counts\nNo runtime values, implicit initialization, joins, or communication peers are inferred\nDynamic/external/unresolved calls are not expanded; alternatives are not sequential calls\nAsync launches are not joined; defer registers only, conditional LIFO/exit/panic execution is not simulated\nPath-end notes apply to the current source callable/region; return values are not evaluated\nAnalysis is partial, not compiler/build correctness or evidence of absent calls\n")
	if view.Program.Language == "go" {
		out.WriteString("Go: lexical call/receive ordering only; non-call operand order is not modeled\n")
	}
	for _, diagnostic := range view.Program.Diagnostics {
		fmt.Fprintf(&out, "Diagnostic %s: %s [%s]\n", packageText(diagnostic.Code), packageText(diagnostic.Message), packageText(sequenceLocation(diagnostic.File, diagnostic.Span.Start.Line, diagnostic.Span.Start.Column, renderer.paths)))
	}
	out.WriteString("endlegend\n@enduml\n")
	return out.String(), nil
}

type sequenceRenderer struct {
	out        *strings.Builder
	aliases    map[string]string
	paths      map[string]string
	callables  []sequence.Callable
	candidates map[string]string
}

func (r *sequenceRenderer) steps(steps []sequence.Step) error {
	for _, step := range steps {
		from, ok := r.aliases[step.From]
		if !ok {
			return fmt.Errorf("sequence step references missing participant %q", step.From)
		}
		event := step.Event
		var label strings.Builder
		label.WriteString(packageText(event.Label))
		if event.Dispatch != "" && event.Dispatch != "direct" {
			label.WriteString("\\n")
			label.WriteString(packageText("[" + event.Dispatch + "; body not expanded]"))
		}
		if len(event.Candidates) > 0 {
			if r.candidates == nil {
				r.candidates = sequenceCandidateLabels(r.callables)
			}
			label.WriteString("\\npossible targets, not a trace; open-world dispatch:")
			for _, id := range event.Candidates {
				name := r.candidates[id]
				if name == "" {
					name = id
				}
				label.WriteString("\\n")
				label.WriteString(packageText(name))
			}
		}
		location := sequenceLocation(event.File, event.Span.Start.Line, event.Span.Start.Column, r.paths)
		if location != "" {
			label.WriteString("\\n")
			label.WriteString(packageText("[" + location + "]"))
		}
		safe := label.String()
		switch event.Kind {
		case sequence.Call, sequence.Async, sequence.Defer, sequence.Send, sequence.Receive:
			to, ok := r.aliases[step.To]
			if !ok {
				return fmt.Errorf("sequence message references missing participant %q", step.To)
			}
			arrow := "->"
			switch event.Kind {
			case sequence.Async:
				arrow = "->>"
				safe = "async launch; no join: " + safe
			case sequence.Defer:
				arrow = "-->"
				safe = "defer registration only: " + safe
			case sequence.Send:
				arrow = "->>"
				safe = "send; peer unknown: " + safe
			case sequence.Receive:
				from, to = to, from
				arrow = "->>"
				safe = "receive; peer unknown: " + safe
			}
			fmt.Fprintf(r.out, "%s %s %s : %s\n", from, arrow, to, safe)
			if len(step.Children) > 0 {
				if event.Kind != sequence.Call || event.Dispatch != "direct" {
					return fmt.Errorf("non-direct sequence message contains an expanded body")
				}
				r.out.WriteString("group direct selected-source body\n")
				if err := r.steps(step.Children); err != nil {
					return err
				}
				r.out.WriteString("end\n")
			}
		case sequence.Alt, sequence.Opt, sequence.Loop:
			fragment := "alt"
			if event.Kind == sequence.Opt {
				fragment = "opt"
			} else if event.Kind == sequence.Loop {
				fragment = "loop"
			}
			if len(step.Branches) > 0 {
				fmt.Fprintf(r.out, "group %s\n", safe)
				if err := r.steps(step.Children); err != nil {
					return err
				}
				for i, branch := range step.Branches {
					keyword := "else"
					if i == 0 {
						keyword = "alt"
					}
					fmt.Fprintf(r.out, "%s %s\n", keyword, packageText(branch.Label))
					if err := r.steps(branch.Steps); err != nil {
						return err
					}
				}
				r.out.WriteString("end\n")
				if err := r.steps(step.Else); err != nil {
					return err
				}
				r.out.WriteString("end\n")
			} else {
				fmt.Fprintf(r.out, "%s %s\n", fragment, safe)
				if err := r.steps(step.Children); err != nil {
					return err
				}
				if len(step.Else) > 0 || (event.Kind == sequence.Alt && event.Exhaustive) {
					if fragment != "alt" {
						return fmt.Errorf("sequence %s region has an invalid else arm", fragment)
					}
					r.out.WriteString("else otherwise\n")
					if err := r.steps(step.Else); err != nil {
						return err
					}
				}
				r.out.WriteString("end\n")
			}
		case sequence.Return:
			fmt.Fprintf(r.out, "note over %s : path ends in current callable/region: %s\n", from, safe)
		case sequence.Unsupported:
			fmt.Fprintf(r.out, "note over %s : unsupported: %s\n", from, safe)
			if len(step.Children) > 0 {
				if err := r.uncertainRegion("body", step.Children); err != nil {
					return err
				}
			}
			if len(step.Else) > 0 {
				if err := r.uncertainRegion("otherwise", step.Else); err != nil {
					return err
				}
			}
			for _, branch := range step.Branches {
				if err := r.uncertainRegion(branch.Label, branch.Steps); err != nil {
					return err
				}
			}
		case sequence.Note, sequence.Omitted:
			fmt.Fprintf(r.out, "note over %s : %s: %s\n", from, event.Kind, safe)
		default:
			return fmt.Errorf("unsupported projected sequence event %q", event.Kind)
		}
	}
	return nil
}

func (r *sequenceRenderer) uncertainRegion(label string, steps []sequence.Step) error {
	fmt.Fprintf(r.out, "opt source region only; execution/transfer NOT simulated: %s\n", packageText(label))
	if err := r.steps(steps); err != nil {
		return err
	}
	r.out.WriteString("end\n")
	return nil
}

func sequenceLocation(file string, line, column int, paths map[string]string) string {
	if display, known := paths[file]; known {
		file = display
	}
	if line > 0 {
		return fmt.Sprintf("%s:%d:%d", file, line, column)
	}
	return file
}
