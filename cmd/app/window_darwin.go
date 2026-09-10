//go:build darwin

package main

/*
#include <stdlib.h>
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static NSRect splash_centered(double w, double h) {
	NSRect screen = [[NSScreen mainScreen] visibleFrame];
	return NSMakeRect(NSMidX(screen) - w / 2, NSMidY(screen) - h / 2, w, h);
}

// Borderless, centred, fixed-size card. The WKWebView content view is kept.
static void splash_apply_card(void *win, double w, double h, int hasColor, double r, double g, double b) {
	NSWindow *window = (NSWindow *)win;
	[window setStyleMask:NSWindowStyleMaskBorderless];
	if (hasColor) {
		[window setBackgroundColor:[NSColor colorWithSRGBRed:r green:g blue:b alpha:1.0]];
	}
	[window setOpaque:YES];
	[window setHasShadow:YES];
	[window setMovableByWindowBackground:YES];
	[window setFrame:splash_centered(w, h) display:YES];
	[window makeKeyAndOrderFront:nil];
}

// Back to a normal titled, resizable window, animating the frame change.
static void splash_restore_main(void *win, double w, double h, const char *title) {
	NSWindow *window = (NSWindow *)win;
	[window setStyleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
	                      NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable)];
	[window setMovableByWindowBackground:NO];
	[window setTitle:[NSString stringWithUTF8String:title]];
	[window setFrame:splash_centered(w, h) display:YES animate:YES];
	[window makeKeyAndOrderFront:nil];
}
*/
import "C"

import (
	"strconv"
	"strings"
	"unsafe"
)

// parseHexColor accepts #rgb / #rrggbb; ok is false for anything else.
func parseHexColor(s string) (r, g, b float64, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255, true
}

// applyCardWindow turns the webview's window into a borderless centred card.
//
// Threading: must be called synchronously on the main goroutine, before
// wv.Run() starts the webview's event loop — not via wv.Dispatch. Calling it
// after Run() (or off the main goroutine) risks a visible flash of the
// titled window and is not the pattern this function is written against.
func applyCardWindow(win unsafe.Pointer, w, h int, background string) {
	r, g, b, ok := parseHexColor(background)
	has := 0
	if ok {
		has = 1
	}
	C.splash_apply_card(win, C.double(w), C.double(h), C.int(has), C.double(r), C.double(g), C.double(b))
}

// restoreMainWindow returns the window to its normal chrome and size.
//
// Threading: unlike applyCardWindow, this runs after wv.Run() has started
// the webview's event loop, so it must be called on the UI thread via
// wv.Dispatch — never directly from another goroutine (AppKit is not
// thread-safe for window mutation).
func restoreMainWindow(win unsafe.Pointer, w, h int, title string) {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	C.splash_restore_main(win, C.double(w), C.double(h), ct)
}
