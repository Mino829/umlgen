package java

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/umlgen/umlgen/internal/model"
)

var modifiers = map[string]bool{
	"public": true, "protected": true, "private": true, "static": true,
	"final": true, "abstract": true, "synchronized": true, "native": true,
	"transient": true, "volatile": true, "strictfp": true, "default": true,
	"sealed": true, "non-sealed": true,
}

type ParseError struct {
	Path string
	Err  error
}

func (e ParseError) Error() string { return fmt.Sprintf("%s: %v", e.Path, e.Err) }

func ParseFile(path string) ([]model.Type, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tokens, err := lex(data)
	if err != nil {
		return nil, err
	}
	pkg := parsePackage(tokens)
	var types []model.Type
	depth := 0
	for i := 0; i < len(tokens); i++ {
		switch tokens[i].text {
		case "{":
			depth++
		case "}":
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unexpected } at line %d", tokens[i].line)
			}
		case "class", "interface":
			if depth != 0 || i+1 >= len(tokens) {
				continue
			}
			t, end, err := parseType(tokens, i, pkg, path)
			if err != nil {
				return nil, err
			}
			types = append(types, t)
			i = end
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced braces")
	}
	return types, nil
}

func parsePackage(ts []token) string {
	for i := 0; i < len(ts); i++ {
		if ts[i].text != "package" {
			continue
		}
		var parts []string
		for i++; i < len(ts) && ts[i].text != ";"; i++ {
			parts = append(parts, ts[i].text)
		}
		return strings.Join(parts, "")
	}
	return ""
}

func parseType(ts []token, at int, pkg, path string) (model.Type, int, error) {
	kind := model.Class
	if ts[at].text == "interface" {
		kind = model.Interface
	}
	t := model.Type{
		Package: pkg, Name: ts[at+1].text, Kind: kind, Source: path,
		Visibility: visibilityBefore(ts, at),
	}
	open := -1
	mode := ""
	var current []token
	angle := 0
	for i := at + 2; i < len(ts); i++ {
		x := ts[i].text
		if x == "{" && angle == 0 {
			open = i
			break
		}
		if x == "<" {
			angle++
		} else if x == ">" && angle > 0 {
			angle--
		}
		if angle == 0 && (x == "extends" || x == "implements" || x == "permits") {
			flushSuper(&t, mode, current)
			mode, current = x, nil
			continue
		}
		if mode != "" {
			current = append(current, ts[i])
		}
	}
	flushSuper(&t, mode, current)
	if open < 0 {
		return t, at, fmt.Errorf("type %s has no body at line %d", t.Name, ts[at].line)
	}
	close := matchingBrace(ts, open)
	if close < 0 {
		return t, at, fmt.Errorf("type %s has an unclosed body at line %d", t.Name, ts[at].line)
	}
	parseMembers(&t, ts[open+1:close])
	return t, close, nil
}

func flushSuper(t *model.Type, mode string, ts []token) {
	if mode == "" || mode == "permits" {
		return
	}
	for _, part := range splitTopLevel(ts, ",") {
		name := cleanType(joinType(part))
		if name == "" {
			continue
		}
		if mode == "implements" {
			t.Implements = append(t.Implements, name)
		} else {
			t.Extends = append(t.Extends, name)
		}
	}
}

func parseMembers(t *model.Type, ts []token) {
	start := 0
	for i := 0; i < len(ts); {
		if ts[i].text == ";" {
			parseMember(t, ts[start:i])
			start, i = i+1, i+1
			continue
		}
		if ts[i].text == "{" {
			segment := ts[start:i]
			if contains(segment, "(") {
				parseMethod(t, segment)
			}
			close := matchingBrace(ts, i)
			if close < 0 {
				return
			}
			start, i = close+1, close+1
			continue
		}
		i++
	}
}

func parseMember(t *model.Type, ts []token) {
	ts = stripAnnotations(ts)
	if len(ts) == 0 || contains(ts, "class") || contains(ts, "interface") {
		return
	}
	if contains(ts, "(") {
		parseMethod(t, ts)
		return
	}
	base := stripModifiers(ts)
	if len(base) < 2 {
		return
	}
	eq := index(base, "=")
	if eq >= 0 {
		base = base[:eq]
	}
	parts := splitTopLevel(base, ",")
	if len(parts) == 0 {
		return
	}
	firstName := lastIdentifier(parts[0])
	if firstName <= 0 {
		return
	}
	fieldType := joinType(parts[0][:firstName])
	addField := func(part []token, first bool) {
		n := lastIdentifier(part)
		if n < 0 {
			return
		}
		name := part[n].text
		if first {
			name = parts[0][firstName].text
		}
		t.Fields = append(t.Fields, model.Field{
			Name: name, Type: fieldType, Visibility: memberVisibility(t, ts), Static: contains(ts, "static") || t.Kind == model.Interface,
		})
	}
	for i, p := range parts {
		addField(p, i == 0)
	}
}

func parseMethod(t *model.Type, ts []token) {
	ts = stripAnnotations(ts)
	open := index(ts, "(")
	if open <= 0 {
		return
	}
	close := matchingParen(ts, open)
	if close < 0 {
		return
	}
	nameAt := open - 1
	name := ts[nameAt].text
	if !isIdentifier(name) || name == "if" || name == "for" || name == "while" || name == "switch" {
		return
	}
	prefix := stripModifiers(ts[:nameAt])
	constructor := name == t.Name
	ret := ""
	if !constructor && len(prefix) > 0 {
		ret = joinType(prefix)
	}
	m := model.Method{
		Name: name, ReturnType: ret, Constructor: constructor,
		Visibility: memberVisibility(t, ts), Static: contains(ts, "static"),
	}
	for _, p := range splitTopLevel(ts[open+1:close], ",") {
		p = stripAnnotations(stripModifiers(p))
		n := lastIdentifier(p)
		if n < 0 {
			continue
		}
		m.Parameters = append(m.Parameters, model.Parameter{
			Name: p[n].text, Type: joinType(p[:n]),
		})
	}
	t.Methods = append(t.Methods, m)
}

func memberVisibility(t *model.Type, ts []token) model.Visibility {
	v := visibility(ts)
	if v == model.Package && t.Kind == model.Interface {
		return model.Public
	}
	return v
}

func visibilityBefore(ts []token, at int) model.Visibility {
	start := at - 1
	for start >= 0 && ts[start].text != ";" && ts[start].text != "}" && ts[start].text != "{" {
		start--
	}
	return visibility(ts[start+1 : at])
}

func visibility(ts []token) model.Visibility {
	for _, t := range ts {
		switch t.text {
		case "public":
			return model.Public
		case "protected":
			return model.Protected
		case "private":
			return model.Private
		}
	}
	return model.Package
}

func stripAnnotations(ts []token) []token {
	var out []token
	for i := 0; i < len(ts); {
		if ts[i].text != "@" {
			out = append(out, ts[i])
			i++
			continue
		}
		i++
		for i < len(ts) && (isIdentifier(ts[i].text) || ts[i].text == ".") {
			i++
		}
		if i < len(ts) && ts[i].text == "(" {
			end := matchingParen(ts, i)
			if end < 0 {
				return out
			}
			i = end + 1
		}
	}
	return out
}

func stripModifiers(ts []token) []token {
	i := 0
	for i < len(ts) && modifiers[ts[i].text] {
		i++
	}
	// Method type parameters, e.g. <T> T find().
	if i < len(ts) && ts[i].text == "<" {
		depth := 0
		for ; i < len(ts); i++ {
			if ts[i].text == "<" {
				depth++
			}
			if ts[i].text == ">" {
				depth--
				if depth == 0 {
					i++
					break
				}
			}
		}
	}
	return ts[i:]
}

func matchingBrace(ts []token, open int) int { return matching(ts, open, "{", "}") }
func matchingParen(ts []token, open int) int { return matching(ts, open, "(", ")") }
func matching(ts []token, open int, left, right string) int {
	depth := 0
	for i := open; i < len(ts); i++ {
		if ts[i].text == left {
			depth++
		} else if ts[i].text == right {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitTopLevel(ts []token, separator string) [][]token {
	var out [][]token
	start, angle, paren, bracket := 0, 0, 0, 0
	for i, t := range ts {
		switch t.text {
		case "<":
			angle++
		case ">":
			if angle > 0 {
				angle--
			}
		case "(":
			paren++
		case ")":
			paren--
		case "[":
			bracket++
		case "]":
			bracket--
		}
		if t.text == separator && angle == 0 && paren == 0 && bracket == 0 {
			out = append(out, ts[start:i])
			start = i + 1
		}
	}
	out = append(out, ts[start:])
	return out
}

func joinType(ts []token) string {
	var b strings.Builder
	for i, t := range ts {
		if t.text == "final" || t.text == "volatile" || t.text == "transient" {
			continue
		}
		if i > 0 && needsSpace(ts[i-1].text, t.text) {
			b.WriteByte(' ')
		}
		b.WriteString(t.text)
	}
	return strings.TrimSpace(b.String())
}

func needsSpace(a, b string) bool {
	return isIdentifier(a) && isIdentifier(b)
}

func cleanType(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "<"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSuffix(strings.TrimSuffix(s, "[]"), "...")
}

func contains(ts []token, value string) bool {
	return index(ts, value) >= 0
}

func index(ts []token, value string) int {
	for i, t := range ts {
		if t.text == value {
			return i
		}
	}
	return -1
}

func lastIdentifier(ts []token) int {
	for i := len(ts) - 1; i >= 0; i-- {
		if isIdentifier(ts[i].text) {
			return i
		}
	}
	return -1
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	r := []rune(s)
	return unicode.IsLetter(r[0]) || r[0] == '_' || r[0] == '$'
}

func SortTypes(types []model.Type) {
	sort.Slice(types, func(i, j int) bool {
		return types[i].QualifiedName() < types[j].QualifiedName()
	})
}

func SourceLabel(path string) string {
	p, err := filepath.Abs(path)
	if err == nil {
		return p
	}
	return path
}
