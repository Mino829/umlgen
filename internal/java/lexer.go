package java

import (
	"fmt"
	"unicode"
)

type token struct {
	text string
	line int
}

func lex(src []byte) ([]token, error) {
	r := []rune(string(src))
	var out []token
	line := 1
	for i := 0; i < len(r); {
		switch {
		case unicode.IsSpace(r[i]):
			if r[i] == '\n' {
				line++
			}
			i++
		case r[i] == '/' && i+1 < len(r) && r[i+1] == '/':
			i += 2
			for i < len(r) && r[i] != '\n' {
				i++
			}
		case r[i] == '/' && i+1 < len(r) && r[i+1] == '*':
			start := line
			i += 2
			closed := false
			for i+1 < len(r) {
				if r[i] == '\n' {
					line++
				}
				if r[i] == '*' && r[i+1] == '/' {
					i += 2
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated block comment at line %d", start)
			}
		case r[i] == '"' || r[i] == '\'':
			quote, start := r[i], line
			i++
			closed := false
			for i < len(r) {
				if r[i] == '\\' {
					i += 2
					continue
				}
				if r[i] == '\n' {
					line++
				}
				if r[i] == quote {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated literal at line %d", start)
			}
			out = append(out, token{text: "<literal>", line: start})
		case unicode.IsLetter(r[i]) || r[i] == '_' || r[i] == '$':
			start := i
			for i < len(r) && (unicode.IsLetter(r[i]) || unicode.IsDigit(r[i]) || r[i] == '_' || r[i] == '$') {
				i++
			}
			out = append(out, token{text: string(r[start:i]), line: line})
		case unicode.IsDigit(r[i]):
			start := i
			for i < len(r) && (unicode.IsLetter(r[i]) || unicode.IsDigit(r[i]) || r[i] == '.' || r[i] == '_') {
				i++
			}
			out = append(out, token{text: string(r[start:i]), line: line})
		default:
			if i+2 < len(r) && string(r[i:i+3]) == "..." {
				out = append(out, token{text: "...", line: line})
				i += 3
			} else {
				out = append(out, token{text: string(r[i]), line: line})
				i++
			}
		}
	}
	return out, nil
}
