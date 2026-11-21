package main

// Панель «периферия» внизу окна: микрофон (дБ), G-сенсор, GPS, звук, вибро.
// Все значения обновляются из фоновых подсистем раз в 200 мс (fyne.Do — редко,
// чтобы не забивать очередь UI-потока, см. bugs B023).

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var (
	beepOnTap  atomic.Bool
	vibroOnTap atomic.Bool
)

type panel struct {
	micBar   *widget.ProgressBar
	micText  *canvas.Text
	gText    *canvas.Text
	gpsText  *canvas.Text
	statText *canvas.Text
	beep     *widget.Check
	vibro    *widget.Check
}

func newPanel() *panel {
	p := &panel{
		micBar:   widget.NewProgressBar(),
		micText:  hudText("микрофон: —"),
		gText:    hudText("G-сенсор: —"),
		gpsText:  hudText("GPS: —"),
		statText: hudText(""),
	}
	p.micBar.Min, p.micBar.Max = -60, 0
	p.micBar.SetValue(-60)

	p.beep = widget.NewCheck("пищать при тапе", func(v bool) { beepOnTap.Store(v) })
	p.vibro = widget.NewCheck("виброклик при тапе", func(v bool) { vibroOnTap.Store(v) })
	p.vibro.SetChecked(true)
	vibroOnTap.Store(true)

	go p.updater()
	return p
}

func (p *panel) view() fyne.CanvasObject {
	btn := func(label string, fn func()) *widget.Button {
		b := widget.NewButton(label, fn)
		return b
	}
	buttons := container.NewHBox(
		btn("Тон 440 Гц", func() { go func() { _ = PlayTone(440, 900*time.Millisecond) }() }),
		btn("Тон 880 Гц", func() { go func() { _ = PlayTone(880, 900*time.Millisecond) }() }),
		btn("Вибрация 250 мс", func() { go func() { _, _ = Vibrate(250 * time.Millisecond) }() }),
		btn("Стоп вибро", func() { _ = VibrateStop() }),
	)

	micRow := container.NewHBox(
		hudText("микрофон:"),
		container.NewGridWrap(fyne.NewSize(520, 18), p.micBar),
		p.micText,
	)
	rows := container.NewVBox(
		hudText("ПЕРИФЕРИЯ NS220: микрофон (дБ) · G-сенсор · GPS · звук · вибро"),
		micRow,
		p.gText,
		p.gpsText,
		buttons,
		container.NewHBox(p.vibro, p.beep),
		p.statText,
	)
	return container.NewVBox(rows)
}

func (p *panel) updater() {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	n := 0
	for range t.C {
		n++
		db, peak, micRun, micErr := MicState()
		x, y, z, g, accelOK, age, accelErr := AccelState()
		st, lat, lon, alt, gpsOk, gpsAge, prov, gpsErr := GPSState()

		fyne.Do(func() {
			// микрофон
			if micRun {
				p.micBar.SetValue(math.Max(-60, db))
				p.micText.Text = fmt.Sprintf("%6.1f дБ (пик %5.1f)", db, peak)
				if peak > -20 {
					p.micText.Color = colGreet // громко
				} else {
					p.micText.Color = colGreet2
				}
			} else {
				p.micText.Text = "недоступен: " + micErr
				p.micText.Color = colCross
			}
			p.micText.Refresh()

			// G-сенсор
			if accelErr != "" {
				p.gText.Text = "G-сенсор: " + accelErr
				p.gText.Color = colCross
			} else if !accelOK {
				p.gText.Text = "G-сенсор: HAL не отдаёт данные (HybrisAdaptor::setInterval not ok, bugs B031)"
				p.gText.Color = colCross
			} else if age > 2*time.Second {
				p.gText.Text = fmt.Sprintf("G-сенсор: нет свежих данных (%.0f с) — лежит неподвижно", age.Seconds())
				p.gText.Color = colGreet2
			} else {
				p.gText.Text = fmt.Sprintf("G-сенсор: x=%+.2f y=%+.2f z=%+.2f м/с²  |g|=%.2f (%.0f мс)",
					x, y, z, g, float64(age.Milliseconds()))
				p.gText.Color = colGreet
			}
			p.gText.Refresh()

			// GPS
			if gpsErr != "" && !gpsOk {
				p.gpsText.Text = "GPS: " + gpsErr
				p.gpsText.Color = colCross
			} else if gpsOk {
				p.gpsText.Text = fmt.Sprintf("GPS: %s · %.6f, %.6f · h=%.0f м · %s · %.0f с назад",
					GPSStatusText(st), lat, lon, alt, prov, gpsAge.Seconds())
				p.gpsText.Color = colGreet
			} else {
				p.gpsText.Text = fmt.Sprintf("GPS: %s (%s) — нет фикса (в помещении это нормально)", GPSStatusText(st), prov)
				p.gpsText.Color = colGreet2
			}
			p.gpsText.Refresh()
		})

		// журнал для удалённой проверки — раз в 2 с
		if n%10 == 0 {
			lg.Printf("PERIPH mic=%.1fdB(peak %.1f) run=%v accel=(%+.2f,%+.2f,%+.2f |g|=%.2f ok=%v age=%dms err=%q) gps=(st=%d %.6f,%.6f alt=%.0f ok=%v age=%ds prov=%q err=%q)",
				db, peak, micRun, x, y, z, g, accelOK, age.Milliseconds(), micErr,
				st, lat, lon, alt, gpsOk, int(gpsAge.Seconds()), prov, gpsErr)
		}
	}
}

// hudText — небольшой текст для панели (без виджетов).
func hudText(s string) *canvas.Text {
	t := canvas.NewText(s, colHUD)
	t.TextSize = 15
	return t
}
