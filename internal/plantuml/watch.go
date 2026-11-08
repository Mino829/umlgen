package plantuml

import (
	"fmt"
	"strings"
)

// GenerateWatchEmpty represents a successfully observed source scope with zero
// selected files; it is not an analysis/render failure or a Git diff placeholder.
func GenerateWatchEmpty(title string) string {
	var out strings.Builder
	out.WriteString("@startuml\n")
	if title != "" {
		fmt.Fprintf(&out, "title %s\n", packageText(title))
	}
	out.WriteString("note \"No matching source files in watched scope\\nNo previous types or dependencies retained\" as EmptyScope\n@enduml\n")
	return out.String()
}
