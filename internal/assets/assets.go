// Package assets embeds the application icon.
package assets

import _ "embed"

// Icon is the application icon (PNG, 256x256), drawn by scripts/make_icon.py.
//
//go:embed icon.png
var Icon []byte
