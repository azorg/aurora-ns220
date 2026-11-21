// Тест: обработка тапов/жестов/скролла и ввод текста (системная клавиатура?).
package main

import (
	"fmt"
	"image/color"
	"log"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var lg = log.New(os.Stderr, "", log.Ltime|log.Lmicroseconds)

// probe — виджет, который логирует все жесты.
type probe struct {
	widget.BaseWidget
	status *widget.Label
}

func newProbe(status *widget.Label) *probe {
	p := &probe{status: status}
	p.ExtendBaseWidget(p)
	return p
}

func (p *probe) set(s string) {
	lg.Println(s)
	p.status.SetText(s)
}

func (p *probe) CreateRenderer() fyne.WidgetRenderer {
	r := canvas.NewRectangle(color.NRGBA{R: 0x22, G: 0x33, B: 0x44, A: 0xff})
	r.StrokeColor = color.NRGBA{R: 0x88, G: 0xaa, B: 0xcc, A: 0xff}
	r.StrokeWidth = 2
	t := canvas.NewText("здесь: тап / свайп / скролл", color.NRGBA{R: 0xcc, G: 0xdd, B: 0xee, A: 0xff})
	t.TextSize = 18
	t.Alignment = fyne.TextAlignCenter
	return widget.NewSimpleRenderer(container.NewStack(r, container.NewCenter(t)))
}

func (p *probe) MouseDown(e *desktop.MouseEvent) {
	lg.Printf("MOUSE down btn=%v pos=%.1f,%.1f", e.Button, e.Position.X, e.Position.Y)
}
func (p *probe) MouseUp(e *desktop.MouseEvent) {
	lg.Printf("MOUSE up btn=%v pos=%.1f,%.1f", e.Button, e.Position.X, e.Position.Y)
}
func (p *probe) Tapped(e *fyne.PointEvent) { p.set(fmt.Sprintf("TAP  x=%.1f y=%.1f", e.Position.X, e.Position.Y)) }
func (p *probe) TappedSecondary(e *fyne.PointEvent) {
	p.set(fmt.Sprintf("TAP2 x=%.1f y=%.1f", e.Position.X, e.Position.Y))
}
func (p *probe) DoubleTapped(e *fyne.PointEvent) {
	p.set(fmt.Sprintf("TAPx2 x=%.1f y=%.1f", e.Position.X, e.Position.Y))
}
func (p *probe) Scrolled(e *fyne.ScrollEvent) {
	p.set(fmt.Sprintf("SCROLL dx=%.1f dy=%.1f", e.Scrolled.DX, e.Scrolled.DY))
}
func (p *probe) Dragged(e *fyne.DragEvent) {
	p.set(fmt.Sprintf("DRAG dx=%.1f dy=%.1f", e.Dragged.DX, e.Dragged.DY))
}
func (p *probe) DragEnd() { p.set("DRAG end") }

func main() {
	a := app.New()
	w := a.NewWindow("Aurora: жесты и клавиатура")

	status := widget.NewLabel("ожидание событий…")
	status.Wrapping = fyne.TextWrapWord

	entry := widget.NewEntry()
	entry.SetPlaceHolder("поле ввода: тапни — появится ли клавиатура?")
	entry.OnChanged = func(s string) {
		lg.Printf("ENTRY changed: %q (%d runes)", s, len([]rune(s)))
	}
	entry.OnSubmitted = func(s string) { lg.Printf("ENTRY submitted: %q", s) }

	// логируем и «сырые» символы, приходящие в фокус
	keyLog := widget.NewLabel("")
	keyLog.Wrapping = fyne.TextWrapWord

	w.SetContent(container.NewBorder(
		container.NewVBox(entry, status, keyLog),
		nil, nil, nil,
		container.NewStack(newProbe(status)),
	))
	w.Resize(fyne.NewSize(900, 600))
	lg.Println("APP START")
	w.ShowAndRun()
}
