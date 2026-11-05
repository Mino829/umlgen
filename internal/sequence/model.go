// Package sequence models source-backed static call projections, not execution traces.
package sequence

import "github.com/Mino829/umlgen/internal/model"

// Source is the exact selected input. Root is its analysis source root; semantic
// backends must not reread source files or execute project code/build tools.
type Source struct {
	Path    string
	Root    string
	Content []byte
}

type File struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

type Diagnostic struct {
	Code    string           `json:"code"`
	Message string           `json:"message"`
	File    string           `json:"file"`
	Span    model.SourceSpan `json:"span"`
	Error   bool             `json:"error"`
}

type Program struct {
	Language    string       `json:"language"`
	Backend     string       `json:"backend"`
	Files       []File       `json:"files"`
	Callables   []Callable   `json:"callables"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// IDs include source context; Selector is the qualified name plus parameter
// types, while Name and Qualified support unambiguous short entry selection.
type Callable struct {
	ID          string           `json:"id"`
	Selector    string           `json:"selector"`
	Qualified   string           `json:"qualified"`
	Name        string           `json:"name"`
	Participant string           `json:"participant"`
	Returns     string           `json:"returns"`
	File        string           `json:"file"`
	Span        model.SourceSpan `json:"span"`
	Body        []Event          `json:"body"`
}

type Kind string

const (
	Call        Kind = "call"
	Async       Kind = "async"
	Defer       Kind = "defer"
	Send        Kind = "send"
	Receive     Kind = "receive"
	Alt         Kind = "alt"
	Opt         Kind = "opt"
	Loop        Kind = "loop"
	Return      Kind = "return"
	Note        Kind = "note"
	Unsupported Kind = "unsupported"
	Omitted     Kind = "omitted"
)

type Branch struct {
	Label  string  `json:"label"`
	Events []Event `json:"events"`
}

// Target is set only for a statically bound, selected-source callable. Dynamic
// candidates are alternatives, not bodies to expand in sequence. Children/Else
// retain control regions; Branches represents switch/select/try alternatives.
type Event struct {
	Kind       Kind             `json:"kind"`
	Label      string           `json:"label"`
	File       string           `json:"file"`
	Span       model.SourceSpan `json:"span"`
	Target     string           `json:"target"`
	Candidates []string         `json:"candidates"`
	Dispatch   string           `json:"dispatch"`
	Children   []Event          `json:"children"`
	Else       []Event          `json:"else"`
	Branches   []Branch         `json:"branches"`
	// Exhaustive marks an explicit else/default or a covering select region;
	// it permits propagating termination only when every arm terminates.
	Exhaustive bool `json:"exhaustive"`
}

type Limits struct {
	MaxCallDepth    int
	MaxParticipants int
	MaxMessages     int
}

func DefaultLimits() Limits {
	return Limits{MaxCallDepth: 3, MaxParticipants: 30, MaxMessages: 200}
}

type Participant struct {
	ID    string
	Label string
}

// Step is a projected, source-backed event. From/To are participant IDs; nested
// steps preserve source control regions. A call's Children is its direct body.
type Step struct {
	Event    Event
	From     string
	To       string
	Children []Step
	Else     []Step
	Branches []StepBranch
}

type StepBranch struct {
	Label string
	Steps []Step
}

type View struct {
	Program      Program
	Entry        Callable
	Limits       Limits
	Participants []Participant
	Steps        []Step
	Messages     int
	Omissions    int
}
