package main

// Крупная тема для панели периферии: на 1200×1920 при FYNE_SCALE=1.0 стандартные
// кнопки/надписи слишком мелкие. Масштабируем размеры темы (текст ×~1.6,
// внутренние отступы ×2) — кнопки становятся примерно в 1.5 раза шире и в 2 раза
// выше. На canvas-объекты (HUD, подписи панели) тема не влияет — их размеры
// задаются вручную (см. hudText/панель).

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

const (
	panelTextScale = 1.6
	panelPadScale  = 2.0
)

type bigTheme struct {
	base fyne.Theme
}

func newBigTheme() fyne.Theme {
	return &bigTheme{base: theme.DarkTheme()}
}

func (t *bigTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	return t.base.Color(n, v)
}

func (t *bigTheme) Font(style fyne.TextStyle) fyne.Resource { return t.base.Font(style) }

func (t *bigTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return t.base.Icon(n) }

func (t *bigTheme) Size(n fyne.ThemeSizeName) float32 {
	base := t.base.Size(n)
	switch n {
	case theme.SizeNameText,
		theme.SizeNameCaptionText,
		theme.SizeNameHeadingText,
		theme.SizeNameSubHeadingText:
		return base * panelTextScale
	case theme.SizeNamePadding,
		theme.SizeNameInnerPadding,
		theme.SizeNameInlineIcon,
		theme.SizeNameInputBorder,
		theme.SizeNameInputRadius,
		theme.SizeNameSelectionRadius:
		return base * panelPadScale
	case theme.SizeNameScrollBar:
		return base * 1.5
	case theme.SizeNameScrollBarSmall:
		return base * 1.5
	case theme.SizeNameSeparatorThickness:
		return base + 1
	}
	return base
}
