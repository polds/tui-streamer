package bundle

import "testing"

func TestMergeAllowlists(t *testing.T) {
	t.Run("neither", func(t *testing.T) {
		if got := MergeAllowlists(nil, nil); got != nil {
			t.Fatalf("got %#v, want nil (all commands allowed)", got)
		}
	})
	t.Run("cli only", func(t *testing.T) {
		got := MergeAllowlists([]string{"make", "npm"}, nil)
		if len(got) != 2 || got[0] != "make" || got[1] != "npm" {
			t.Fatalf("got %#v", got)
		}
	})
	t.Run("bundle only", func(t *testing.T) {
		got := MergeAllowlists(nil, []string{"ping"})
		if len(got) != 1 || got[0] != "ping" {
			t.Fatalf("got %#v", got)
		}
	})
	t.Run("union dedup", func(t *testing.T) {
		got := MergeAllowlists([]string{"make", "go"}, []string{"go", "gum"})
		want := []string{"make", "go", "gum"}
		if len(got) != len(want) {
			t.Fatalf("got %#v", got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		}
	})
	t.Run("blank entries ignored", func(t *testing.T) {
		got := MergeAllowlists([]string{"", "make"}, []string{"  "})
		if len(got) != 1 || got[0] != "make" {
			t.Fatalf("got %#v", got)
		}
	})
}

func TestCommandAllowed(t *testing.T) {
	if !CommandAllowed(nil, []string{"rm", "-rf", "/"}) {
		t.Fatal("empty allowlist should permit everything")
	}
	if CommandAllowed([]string{"ping"}, nil) {
		t.Fatal("empty command should be rejected when allowlist is set")
	}
	allowed := []string{"ping", "gum"}
	cases := []struct {
		cmd  []string
		want bool
	}{
		{[]string{"ping", "-c", "1"}, true},
		{[]string{"/usr/bin/ping"}, true},
		{[]string{"/tmp/tui/gum", "spin"}, true},
		{[]string{"dig"}, false},
		{[]string{"./gum"}, true},
	}
	for _, tc := range cases {
		if got := CommandAllowed(allowed, tc.cmd); got != tc.want {
			t.Errorf("CommandAllowed(%q) = %v, want %v", tc.cmd, got, tc.want)
		}
	}
}

func TestFilterAndResolveTheme(t *testing.T) {
	got := FilterThemes([]string{"nord", "not-a-theme", "nord", "dracula"})
	if len(got) != 2 || got[0] != "nord" || got[1] != "dracula" {
		t.Fatalf("FilterThemes = %#v", got)
	}
	if FilterThemes(nil) != nil {
		t.Fatal("empty requested should stay empty (all themes)")
	}
	if g := ResolveDefaultTheme("nord", []string{"dracula", "nord"}); g != "nord" {
		t.Errorf("ResolveDefaultTheme = %q", g)
	}
	if g := ResolveDefaultTheme("matrix", []string{"nord"}); g != "nord" {
		t.Errorf("invalid default should fall back to allowlist, got %q", g)
	}
	if g := ResolveDefaultTheme("", nil); g != DefaultTheme {
		t.Errorf("no config should use DefaultTheme, got %q", g)
	}
}
