// Package web embeds the static frontend assets into the binary.
package web

import "embed"

//go:embed static
var Static embed.FS
