package main

import (
	"github.com/go-gl/glfw/v3.4/glfw"

	"fyne.io/fyne/v2"
)

// screenSize возвращает разрешение экрана в пикселях.
//
// На Aurora/lipstick SetFullScreen() через wl_shell даёт чёрный кадр
// (проверено в cterm), а Resize до размера экрана работает и визуально
// даёт полноэкранное окно. Поэтому размер берём у GLFW напрямую —
// драйвер Fyne такого интерфейса не предоставляет.
func screenSize() fyne.Size {
	mon := glfw.GetPrimaryMonitor()
	if mon == nil {
		return fyne.Size{}
	}
	mode := mon.GetVideoMode()
	if mode == nil || mode.Width <= 0 || mode.Height <= 0 {
		return fyne.Size{}
	}
	return fyne.NewSize(float32(mode.Width), float32(mode.Height))
}
