package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// cjkTheme wraps the default Fyne theme with the Apple-inspired light surface
// palette from DESIGN.md (#0066cc Action Blue, #0071e3 Focus Blue, #f5f5f7
// parchment canvas, #ffffff cards, #1d1d1f ink, #e0e0e0 hairlines) and bundled
// Noto Sans SC for crisp Chinese rendering on macOS, Windows, and Linux.
type cjkTheme struct {
	fyne.Theme
	cjkFont fyne.Resource
}

func newCJKTheme() cjkTheme {
	return cjkTheme{Theme: theme.DefaultTheme(), cjkFont: fyne.NewStaticResource("NotoSansSC-Regular.otf", notoSansSC)}
}

func (c cjkTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Symbol {
		return c.Theme.Font(style)
	}
	return c.cjkFont
}

func (c cjkTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNamePrimary, theme.ColorNameHyperlink:
		return color.NRGBA{R: 0x00, G: 0x66, B: 0xcc, A: 0xff} // #0066cc Action Blue
	case theme.ColorNameFocus:
		return color.NRGBA{R: 0x00, G: 0x71, B: 0xe3, A: 0xff} // #0071e3 Focus Blue
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0xf5, G: 0xf5, B: 0xf7, A: 0xff} // #f5f5f7 Parchment
	case theme.ColorNameOverlayBackground, theme.ColorNameMenuBackground, theme.ColorNameInputBackground, theme.ColorNameButton:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff} // #ffffff Pure Canvas
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 0xfa, G: 0xfa, B: 0xfc, A: 0xff} // #fafafc Subtle Header
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0x1d, G: 0x1d, B: 0x1f, A: 0xff} // #1d1d1f Primary Ink
	case theme.ColorNameForegroundOnPrimary:
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 0x7a, G: 0x7a, B: 0x7a, A: 0xff} // #7a7a7a Muted Slate
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 0xa6, G: 0xa6, B: 0xa6, A: 0xff}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 0xf0, G: 0xf0, B: 0xf0, A: 0xff}
	case theme.ColorNameInputBorder, theme.ColorNameSeparator:
		return color.NRGBA{R: 0xe0, G: 0xe0, B: 0xe0, A: 0xff} // #e0e0e0 Hairline
	case theme.ColorNameHover:
		return color.NRGBA{R: 0xf0, G: 0xf0, B: 0xf2, A: 0xff}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 0xe5, G: 0xe5, B: 0xea, A: 0xff}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0x00, G: 0x71, B: 0xe3, A: 0x29}
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00} // Flat elevation per DESIGN.md
	case theme.ColorNameSuccess:
		return color.NRGBA{R: 0x1d, G: 0x7a, B: 0x46, A: 0xff}
	case theme.ColorNameError:
		return color.NRGBA{R: 0xd7, G: 0x00, B: 0x15, A: 0xff}
	default:
		return c.Theme.Color(name, theme.VariantLight)
	}
}

func (c cjkTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 10
	case theme.SizeNameInputRadius:
		return 10
	case theme.SizeNameSelectionRadius:
		return 8
	case theme.SizeNameText:
		return 14
	default:
		return c.Theme.Size(name)
	}
}
