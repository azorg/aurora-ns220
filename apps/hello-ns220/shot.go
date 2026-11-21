package main

import (
	"image/png"
	"os"

	"fyne.io/fyne/v2"
)

// saveShot сохраняет текущее содержимое окна в PNG (для удалённой проверки
// «что видно на экране» без доступа к lipstick-скриншоту).
func saveShot(w fyne.Window, path string) error {
	img := w.Canvas().Capture()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
