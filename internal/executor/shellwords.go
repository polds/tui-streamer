package executor

import (
	"fmt"
	"strings"
	"unicode"
)

// SplitCommand splits a command line into words using POSIX-shell-like
// quoting rules so bundle and REST commands such as
//
//	sh -c 'echo "$TUI_PATH" && ls -l'
//
// reach the executor as ["sh", "-c", `echo "$TUI_PATH" && ls -l`].
//
// Rules: unquoted whitespace separates words; single quotes are literal;
// inside double quotes a backslash escapes only `"` and `\`; outside quotes a
// backslash escapes the next character. No variable expansion or globbing is
// performed — TUI_PATH substitution happens later, per word, in ExpandTUIPath.
// An unterminated quote or trailing backslash is an error.
func SplitCommand(s string) ([]string, error) {
	var (
		words  []string
		cur    strings.Builder
		inWord bool
		quote  rune // 0 when outside quotes, otherwise '\'' or '"'
	)
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				cur.WriteRune(c)
			}
		case quote == '"':
			switch {
			case c == '"':
				quote = 0
			case c == '\\' && i+1 < len(rs) && (rs[i+1] == '"' || rs[i+1] == '\\'):
				i++
				cur.WriteRune(rs[i])
			default:
				cur.WriteRune(c)
			}
		case c == '\'' || c == '"':
			quote = c
			inWord = true // so "" still yields an (empty) word
		case c == '\\':
			if i+1 >= len(rs) {
				return nil, fmt.Errorf("trailing backslash")
			}
			i++
			cur.WriteRune(rs[i])
			inWord = true
		case unicode.IsSpace(c):
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(c)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote", quote)
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, nil
}
