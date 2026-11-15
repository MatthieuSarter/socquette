package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Theme modes stored in the preferences.
const (
	themeAuto  = "auto"
	themeLight = "light"
	themeDark  = "dark"
)

// fixedVariant is the default theme forced to one variant, whatever the OS asks for.
type fixedVariant struct {
	fyne.Theme
	variant fyne.ThemeVariant
}

func (t fixedVariant) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return t.Theme.Color(n, t.variant)
}

// applyTheme sets the theme of a mode; auto follows the OS.
func applyTheme(a fyne.App, mode string) {
	switch mode {
	case themeLight:
		a.Settings().SetTheme(fixedVariant{theme.DefaultTheme(), theme.VariantLight})
	case themeDark:
		a.Settings().SetTheme(fixedVariant{theme.DefaultTheme(), theme.VariantDark})
	default:
		a.Settings().SetTheme(theme.DefaultTheme())
	}
}
