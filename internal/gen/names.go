package main

import (
	"fmt"
	"strings"
	"unicode"
)

// initialisms are the words Go writes in capitals.
var initialisms = map[string]string{"id": "ID", "mcp": "MCP", "url": "URL", "api": "API", "ip": "IP"}

// words splits a JSON key, an event type or a value into its words:
// "mcpServerId", "agent_hub.finding.created" and "false_positive" alike.
func words(text string) []string {
	var split []string
	var word []rune
	flush := func() {
		if len(word) > 0 {
			split = append(split, strings.ToLower(string(word)))
			word = nil
		}
	}
	for _, r := range text {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			flush()
		case unicode.IsUpper(r) && len(word) > 0 && !unicode.IsUpper(word[len(word)-1]):
			flush()
			word = append(word, r)
		default:
			word = append(word, r)
		}
	}
	flush()
	return split
}

// goName is text as an exported Go identifier.
func goName(text string) string {
	var name strings.Builder
	for _, word := range words(text) {
		if initialism, ok := initialisms[word]; ok {
			name.WriteString(initialism)
			continue
		}
		name.WriteString(strings.ToUpper(word[:1]) + word[1:])
	}
	return name.String()
}

// goPart is text as part of a Go identifier, refusing text with no letter or
// digit in it, which would add nothing to the name.
func goPart(text string) (string, error) {
	part := goName(text)
	if part == "" {
		return "", fmt.Errorf("%w: %q does not make an identifier", ErrUnsupported, text)
	}
	return part, nil
}

// names refuses a name used twice, which two different keys or values can
// come to once they are turned into identifiers.
type names map[string]string

func (n names) claim(name string, source string) error {
	if name == "" || !unicode.IsLetter(rune(name[0])) {
		return fmt.Errorf("%w: %q does not make an identifier", ErrUnsupported, source)
	}
	if earlier, taken := n[name]; taken {
		return fmt.Errorf("%w: %q and %q both make the name %s", ErrUnsupported, earlier, source, name)
	}
	n[name] = source
	return nil
}
