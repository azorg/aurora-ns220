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
	quit       func()
	board      *board
	micBar     *widget.ProgressBar
	micText    *canvas.Text
	gText      *canvas.Text
	gpsText    *canvas.Text
	statText   *canvas.Text
	brightText *canvas.Text
	btnText    *canvas.Text
	modemText  *canvas.Text
	beep       *widget.Check
	vibro      *widget.Check
}

func newPanel(quit func(), b *board) *panel {
	p := &panel{
		quit:       quit,
		board:      b,
		micBar:     widget.NewProgressBar(),
		micText:    hudText("микрофон: —"),
		gText:      hudText("G-сенсор: —"),
		gpsText:    hudText("GPS: —"),
		statText:   hudText(""),
		brightText: hudText("подсветка: —"),
		btnText:    hudText("кнопки: —"),
		modemText:  hudText("модем: —"),
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
		btn("⟲ 90°", func() { p.board.Rotate(-90) }),
		btn("⟳ 90°", func() { p.board.Rotate(90) }),
		btn("0°", func() { p.board.SetRot(0) }),
		btn("Выход", func() { p.quit() }),
	)

	micRow := container.NewHBox(
		hudText("микрофон:"),
		container.NewGridWrap(fyne.NewSize(620, 34), p.micBar),
		p.micText,
	)
	rows := container.NewVBox(
		hudText("ПЕРИФЕРИЯ NS220: микрофон (дБ) · G-сенсор · GPS · звук · вибро"),
		micRow,
		p.gText,
		p.gpsText,
		buttons,
		container.NewHBox(
			btn("−", func() { go func() { _, _, _ = SetBrightness(-20) }() }),
			btn("+", func() { go func() { _, _, _ = SetBrightness(20) }() }),
			p.brightText,
			btn("Сон", func() { go func() { _ = DisplayOff() }() }),
			btn("Экран", func() { go func() { _ = DisplayOn() }() }),
		),
		container.NewHBox(p.vibro, p.beep),
		p.modemText,
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
		lastKey, keyCount, keyAge, keysOK, keysErr := ButtonsState()
		var brightCur, brightMax int
		var brightErr error
		if n%10 == 1 { // раз в 2 с (exec хелпера)
			brightCur, brightMax, brightErr = Brightness()
			fyne.Do(func() {
				if brightErr == nil && brightMax > 0 {
					p.brightText.Text = fmt.Sprintf("подсветка: %d/%d", brightCur, brightMax)
					p.brightText.Color = colGreet2
				} else {
					p.brightText.Text = "подсветка: нет доступа (aurora-brightness)"
					p.brightText.Color = colCross
					lg.Printf("BRIGHT: cur=%d max=%d err=%v", brightCur, brightMax, brightErr)
				}
				p.brightText.Refresh()
			})
		}
		if n%10 == 1 {
			modOK, modOnline, modIMEI, modRev, modSIM, modErr := ModemState()
			fyne.Do(func() {
				switch {
				case modErr != "":
					p.modemText.Text = "модем: " + modErr
					p.modemText.Color = colCross
				case !modOK:
					p.modemText.Text = "модем: нет данных"
					p.modemText.Color = colCross
				default:
					sim := "нет"
					if modSIM {
						sim = "есть"
					}
					online := "offline"
					if modOnline {
						online = "online"
					}
					keyShow := lastKey
					if keyShow == "" {
						keyShow = "—"
					}
					p.modemText.Text = fmt.Sprintf("кнопки: %s   ·   модем: %s · SIM: %s · IMEI %s · 112: нет сети", keyShow, online, sim, modIMEI)
					if modSIM && modOnline {
						p.modemText.Text += " (см. doc/15 §15.8)"
					}
					p.modemText.Color = colGreet2
					_ = modRev
				}
				p.modemText.Refresh()
			})
		}
		db, peak, micRun, micErr := MicState()
		x, y, z, g, azimuth, orient, accelOK, age, accelErr := AccelState()
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
			} else if !accelOK && accelErr != "" {
				p.gText.Text = "G-сенсор: " + accelErr
				p.gText.Color = colCross
			} else if !accelOK {
				if brightCur == 0 {
					p.gText.Text = "G-сенсор: экран погашен — сенсоры приостановлены (коснитесь экрана)"
				} else {
					p.gText.Text = "G-сенсор: нет данных (helper не отдаёт; sensorfwd?)"
				}
				p.gText.Color = colCross
			} else {
				p.gText.Text = fmt.Sprintf("G: x=%+.2f y=%+.2f z=%+.2f |g|=%.2f · %s · компас %.0f° · %s",
					x, y, z, g, TiltText(x, y, z), azimuth, OrientationText(orient))
				if age > 2*time.Second {
					p.gText.Color = colGreet2
				} else {
					p.gText.Color = colGreet
				}
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

			// кнопки
			if keysErr != "" {
				p.btnText.Text = "кнопки: " + keysErr
				p.btnText.Color = colCross
			} else if !keysOK {
				p.btnText.Text = "кнопки: evdev недоступен"
				p.btnText.Color = colCross
			} else if keyCount == 0 {
				p.btnText.Text = "кнопки: нажмите volume/power"
				p.btnText.Color = colGreet2
			} else {
				p.btnText.Text = fmt.Sprintf("кнопки: %s ×%d (%.0f с)", lastKey, keyCount, keyAge.Seconds())
				p.btnText.Color = colGreet
			}
			p.btnText.Refresh()

		})

		// журнал для удалённой проверки — раз в 2 с
		if n%10 == 0 {
			lg.Printf("PERIPH mic=%.1fdB(peak %.1f) run=%v accel=(%+.2f,%+.2f,%+.2f |g|=%.2f az=%.0f orient=%d ok=%v age=%dms err=%q) gps=(st=%d %.6f,%.6f alt=%.0f ok=%v age=%ds prov=%q err=%q)",
				db, peak, micRun, x, y, z, g, azimuth, orient, accelOK, age.Milliseconds(), accelErr,
				st, lat, lon, alt, gpsOk, int(gpsAge.Seconds()), prov, gpsErr)
		}
	}
}

// hudText — небольшой текст для панели (без виджетов).
func hudText(s string) *canvas.Text {
	t := canvas.NewText(s, colHUD)
	t.TextSize = 22
	return t
}
