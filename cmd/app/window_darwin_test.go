//go:build darwin

package main

import "testing"

func TestParseHexColor(t *testing.T) {
	if r, g, b, ok := parseHexColor("#ff8000"); !ok || r != 1 || g < 0.50 || g > 0.51 || b != 0 {
		t.Errorf("#ff8000 → %v %v %v %v", r, g, b, ok)
	}
	if _, _, _, ok := parseHexColor("#abc"); !ok {
		t.Errorf("#abc should parse")
	}
	if _, _, _, ok := parseHexColor("rgb(1,2,3)"); ok {
		t.Errorf("rgb() is not hex")
	}
}
