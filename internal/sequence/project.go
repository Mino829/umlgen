package sequence

import "fmt"

// Project retains source regions and expands only selected-source direct calls.
// Its ordering describes source evaluation, never a concrete execution trace.
func Project(program Program, entry Callable, limits Limits) (View, error) {
	if limits.MaxCallDepth < 0 || limits.MaxParticipants <= 0 || limits.MaxMessages <= 0 {
		return View{}, fmt.Errorf("sequence limits require nonnegative call depth and positive participant/message budgets")
	}
	p := projector{
		view:         View{Program: program, Limits: limits},
		callables:    make(map[string]*Callable, len(program.Callables)),
		participants: make(map[string]bool), active: make(map[string]bool),
	}
	for i := range program.Callables {
		c := &program.Callables[i]
		if c.ID == "" {
			return View{}, fmt.Errorf("sequence callable has an empty canonical ID")
		}
		if _, exists := p.callables[c.ID]; exists {
			return View{}, fmt.Errorf("duplicate sequence callable ID %q", c.ID)
		}
		p.callables[c.ID] = c
	}
	root, ok := p.callables[entry.ID]
	if !ok {
		return View{}, fmt.Errorf("sequence entry %q is not in the analyzed program", entry.ID)
	}
	p.view.Entry = *root
	participant, label := callableParticipant(root)
	p.addParticipant(participant, label)
	p.active[root.ID] = true
	p.view.Steps, _ = p.events(root.Body, participant, 0)
	return p.view, nil
}

type projector struct {
	view               View
	callables          map[string]*Callable
	participants       map[string]bool
	active             map[string]bool
	halted             bool
	participantOmitted bool
}

func callableParticipant(c *Callable) (string, string) {
	label := c.Participant
	if label == "" {
		label = c.Qualified
	}
	if label == "" {
		label = c.ID
	}
	return "local:" + label, label
}

func (p *projector) addParticipant(id, label string) bool {
	if p.participants[id] {
		return true
	}
	if len(p.view.Participants) >= p.view.Limits.MaxParticipants {
		return false
	}
	p.participants[id] = true
	p.view.Participants = append(p.view.Participants, Participant{ID: id, Label: label})
	return true
}

func (p *projector) omitted(event Event, from, reason string) Step {
	p.view.Omissions++
	return Step{Event: Event{Kind: Omitted, Label: reason, File: event.File, Span: event.Span}, From: from}
}

func projectedEvent(event Event) Event {
	// Regions live only in projected Step fields; do not retain an unbounded,
	// unprojected second copy that downstream consumers could accidentally render.
	event.Children = nil
	event.Else = nil
	event.Branches = nil
	return event
}

// The result reports whether this source region has no projected continuation,
// either because it returns or because an unsupported barrier hides the rest.
func (p *projector) events(events []Event, from string, depth int) ([]Step, bool) {
	var steps []Step
	for _, event := range events {
		if p.halted {
			break
		}
		step := Step{Event: projectedEvent(event), From: from}
		switch event.Kind {
		case Call, Async, Defer, Send, Receive:
			if p.view.Messages >= p.view.Limits.MaxMessages {
				steps = append(steps, p.omitted(event, from, "Message budget reached; remaining source projection omitted"))
				p.halted = true
				return steps, false
			}
			target := p.callables[event.Target]
			direct := event.Dispatch == "direct" && target != nil
			var to, label string
			if direct {
				to, label = callableParticipant(target)
			} else if event.Kind == Send || event.Kind == Receive {
				to, label = "channel:unknown", "channel peer (not statically known)"
			} else {
				dispatch := event.Dispatch
				if dispatch == "" || dispatch == "direct" {
					dispatch = "unresolved"
					step.Event.Dispatch = dispatch
				}
				label = dispatch + ": " + event.Label
				to = "uncertain:" + label
			}
			if !p.addParticipant(to, label) {
				if !p.participantOmitted {
					steps = append(steps, p.omitted(event, from, "Participant budget reached; calls requiring additional participants omitted"))
					p.participantOmitted = true
				}
				continue
			}
			step.To = to
			p.view.Messages++
			if event.Kind == Call && direct && len(target.Body) > 0 {
				switch {
				case p.active[target.ID]:
					step.Children = []Step{p.omitted(event, to, "Recursive call body omitted: "+target.Selector)}
				case depth >= p.view.Limits.MaxCallDepth:
					step.Children = []Step{p.omitted(event, to, "Call depth budget reached; body omitted: "+target.Selector)}
				default:
					p.active[target.ID] = true
					step.Children, _ = p.events(target.Body, to, depth+1)
					delete(p.active, target.ID)
				}
			}
		case Alt, Opt, Loop:
			var childrenEnd, elseEnd bool
			step.Children, childrenEnd = p.events(event.Children, from, depth)
			step.Else, elseEnd = p.events(event.Else, from, depth)
			allEnd := len(event.Branches) > 0
			for _, branch := range event.Branches {
				body, ended := p.events(branch.Events, from, depth)
				step.Branches = append(step.Branches, StepBranch{Label: branch.Label, Steps: body})
				allEnd = allEnd && ended
			}
			steps = append(steps, step)
			if event.Kind == Alt && ((len(event.Branches) == 0 && (event.Exhaustive || len(event.Else) > 0) && childrenEnd && elseEnd) || (len(event.Branches) > 0 && event.Exhaustive && allEnd)) {
				return steps, true
			}
			continue
		case Return:
			steps = append(steps, step)
			return steps, true
		case Unsupported:
			// These are independently captured source regions, not an assumed
			// exceptional transfer or unconditional execution of every catch.
			step.Children, _ = p.events(event.Children, from, depth)
			step.Else, _ = p.events(event.Else, from, depth)
			for _, branch := range event.Branches {
				body, _ := p.events(branch.Events, from, depth)
				step.Branches = append(step.Branches, StepBranch{Label: branch.Label, Steps: body})
			}
			steps = append(steps, step)
			// Backends may already stop extraction at this barrier. Missing
			// later IR events must not hide that continuation is unmodeled.
			steps = append(steps, p.omitted(event, from, "Unsupported control/body; continuation is not projected"))
			return steps, true
		default:
			// Unknown future event kinds remain visible, never execute hidden bodies.
			if event.Kind != Note && event.Kind != Omitted {
				step.Event.Kind = Unsupported
				step.Event.Label = "Unsupported event " + string(event.Kind) + ": " + event.Label
				steps = append(steps, step)
				steps = append(steps, p.omitted(event, from, "Unsupported control/body; continuation is not projected"))
				return steps, true
			}
			if event.Kind == Omitted {
				p.view.Omissions++
			}
		}
		steps = append(steps, step)
	}
	return steps, false
}
