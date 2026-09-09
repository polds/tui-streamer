package executor

import (
	"reflect"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"plain words", "echo hello world", []string{"echo", "hello", "world"}},
		{"collapses whitespace", "  echo \t hi  ", []string{"echo", "hi"}},
		{"single quotes keep everything literal", `sh -c 'echo "$TUI_PATH" && ls -l'`, []string{"sh", "-c", `echo "$TUI_PATH" && ls -l`}},
		{"double quotes group words", `gum confirm "Are you sure?" && echo yes`, []string{"gum", "confirm", "Are you sure?", "&&", "echo", "yes"}},
		{"escaped quote inside double quotes", `echo "say \"hi\""`, []string{"echo", `say "hi"`}},
		{"backslash escapes a space outside quotes", `ls My\ Photos`, []string{"ls", "My Photos"}},
		{"quotes adjacent to text join one word", `echo pre"fix"post`, []string{"echo", "prefixpost"}},
		{"empty quoted argument is preserved", `cmd "" x`, []string{"cmd", "", "x"}},
		{"empty input", "   ", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SplitCommand(tc.in)
			if err != nil {
				t.Fatalf("SplitCommand(%q) error: %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SplitCommand(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSplitCommandUnterminatedQuote(t *testing.T) {
	for _, in := range []string{`sh -c 'echo hi`, `echo "hi`, `echo hi\`} {
		if _, err := SplitCommand(in); err == nil {
			t.Errorf("SplitCommand(%q) expected error, got nil", in)
		}
	}
}
