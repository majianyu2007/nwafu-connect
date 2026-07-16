package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// cjkTheme wraps the default Fyne theme and replaces every text font with the
// bundled Noto Sans SC so Chinese labels render correctly on macOS, Windows and
// Linux without relying on system font discovery.
type cjkTheme struct {
	fyne.Theme
	cjkFont fyne.Resource
}

func newCJKTheme() cjkTheme {
	return cjkTheme{Theme: theme.DefaultTheme(), cjkFont: fyne.NewStaticResource("NotoSansSC-Regular.otf", notoSansSC)}
}

func (c cjkTheme) Font(style fyne.TextStyle) fyne.Resource {
	switch {
	case style.Monospace:
		return c.cjkFont
	case style.Bold && style.Italic:
		return c.cjkFont
	case style.Bold:
		return c.cjkFont
	case style.Italic:
		return c.cjkFont
	case style.Symbol:
		return c.Theme.Font(style)
	default:
		return c.cjkFont
	}
}

func (c cjkTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return c.Theme.Color(name, theme.VariantLight)
}

func (c cjkTheme) Size(name fyne.ThemeSizeName) float32 {
	return c.Theme.Size(name)
}
