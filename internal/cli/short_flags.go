package cli

import (
	"flag"
	"strings"
)

func booleanFlag(definition *flag.Flag) bool {
	boolean, ok := definition.Value.(interface{ IsBoolFlag() bool })
	return ok && boolean.IsBoolFlag()
}

// expandShortFlags leaves registered single-dash long names intact. Only known
// one-letter flags form a cluster; value flags consume the remaining bytes (or
// the next argument), so a flag-looking value is never expanded on its own.
func expandShortFlags(args []string, fs *flag.FlagSet, stopAtPositional bool) []string {
	var expanded []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" || stopAtPositional && (!strings.HasPrefix(arg, "-") || arg == "-") {
			if expanded == nil {
				return args
			}
			return append(expanded, args[i:]...)
		}
		name, _, inline := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		definition := fs.Lookup(name)
		if definition != nil && strings.HasPrefix(arg, "-") {
			if expanded != nil {
				expanded = append(expanded, arg)
			}
			if !inline && !booleanFlag(definition) && i+1 < len(args) {
				i++
				if expanded != nil {
					expanded = append(expanded, args[i])
				}
			}
			continue
		}
		valueAt, compact := shortCluster(arg, fs)
		if !compact {
			if expanded != nil {
				expanded = append(expanded, arg)
			}
			continue
		}
		end := len(arg)
		if valueAt >= 0 {
			end = valueAt + 1
		}
		if expanded == nil {
			expanded = make([]string, 0, len(args)+end)
			expanded = append(expanded, args[:i]...)
		}
		for position := 1; position < end; position++ {
			expanded = append(expanded, "-"+arg[position:position+1])
		}
		if valueAt >= 0 {
			if valueAt+1 < len(arg) {
				value := strings.TrimPrefix(arg[valueAt+1:], "=")
				expanded = append(expanded, value)
			} else if i+1 < len(args) {
				i++
				expanded = append(expanded, args[i])
			}
		}
	}
	if expanded == nil {
		return args
	}
	return expanded
}

// shortCluster validates before emitting any flags. Unknown long options must
// still fail as a whole, rather than partially enabling their initial letters.
func shortCluster(arg string, fs *flag.FlagSet) (valueAt int, compact bool) {
	if len(arg) < 3 || arg[0] != '-' || arg[1] == '-' {
		return -1, false
	}
	for position := 1; position < len(arg); position++ {
		definition := fs.Lookup(arg[position : position+1])
		if definition == nil {
			return -1, false
		}
		if !booleanFlag(definition) {
			return position, true
		}
	}
	return -1, true
}
