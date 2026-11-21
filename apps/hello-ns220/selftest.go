package main

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// selfTest прогоняет распознавание жестов без участия тачскрина: вызывает те же
// обработчики, которые Fyne вызывает по событиям (MouseDown/Dragged/DragEnd,
// Tapped, DoubleTapped, Scrolled). Проверяет классификатор, счётчики, след и
// «волны»; на HUD после прогона видны результаты.
//
// Нужен для удалённой проверки приложения (тач физически недоступен), а также
// как smoke-тест при сборке RPM.
func (b *board) selfTest() {
	step := func(d time.Duration) { time.Sleep(d) }
	down := func(p fyne.Position) {
		b.MouseDown(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: p}})
	}
	up := func(p fyne.Position) {
		b.MouseUp(&desktop.MouseEvent{PointEvent: fyne.PointEvent{Position: p}})
	}
	drag := func(p fyne.Position, dx, dy float32) {
		b.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: p},
			Dragged: fyne.NewDelta(dx, dy)})
	}
	swipe := func(from, to fyne.Position) {
		down(from)
		step(40 * time.Millisecond)
		n := 6
		for i := 1; i <= n; i++ {
			f := float32(i) / float32(n)
			p := fyne.NewPos(from.X+(to.X-from.X)*f, from.Y+(to.Y-from.Y)*f)
			drag(p, (to.X-from.X)/float32(n), (to.Y-from.Y)/float32(n))
			step(20 * time.Millisecond)
		}
		up(to)
		b.DragEnd()
	}

	step(1200 * time.Millisecond)
	lg.Printf("SELFTEST start")

	// 1. одиночный тап
	p := fyne.NewPos(300, 700)
	down(p)
	step(50 * time.Millisecond)
	up(p)
	b.Tapped(&fyne.PointEvent{Position: p})
	step(120 * time.Millisecond)

	// 2. двойной тап
	p2 := fyne.NewPos(600, 760)
	down(p2)
	step(40 * time.Millisecond)
	up(p2)
	b.Tapped(&fyne.PointEvent{Position: p2})
	b.Tapped(&fyne.PointEvent{Position: p2})
	b.DoubleTapped(&fyne.PointEvent{Position: p2})
	step(120 * time.Millisecond)

	// 3. долгое нажатие (650 мс без движения)
	p3 := fyne.NewPos(900, 820)
	down(p3)
	step(800 * time.Millisecond)
	up(p3)

	// 4. свайпы в четыре стороны
	swipe(fyne.NewPos(500, 1000), fyne.NewPos(900, 1010)) // вправо
	step(80 * time.Millisecond)
	swipe(fyne.NewPos(900, 1100), fyne.NewPos(500, 1105)) // влево
	step(80 * time.Millisecond)
	swipe(fyne.NewPos(600, 1200), fyne.NewPos(610, 1000)) // вверх
	step(80 * time.Millisecond)
	swipe(fyne.NewPos(600, 1000), fyne.NewPos(605, 1250)) // вниз
	step(80 * time.Millisecond)

	// 5. медленное перетаскивание (drag, не свайп)
	down(fyne.NewPos(400, 1400))
	step(950 * time.Millisecond)
	for i := 0; i < 5; i++ {
		drag(fyne.NewPos(float32(400+i*40), 1400), 40, 0)
		step(60 * time.Millisecond)
	}
	up(fyne.NewPos(600, 1400))
	b.DragEnd()

	// 6. прокрутка
	b.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(700, 900)},
		Scrolled: fyne.NewDelta(0, -40)})

	// 7. звук и вибро (слышно/ощутимо на устройстве)
	step(200 * time.Millisecond)
	lg.Printf("SELFTEST tone 660 Hz")
	if err := PlayTone(660, 500*time.Millisecond); err != nil {
		lg.Printf("SELFTEST tone error: %v", err)
	}
	lg.Printf("SELFTEST vibrate 250 ms")
	if _, err := Vibrate(250 * time.Millisecond); err != nil {
		lg.Printf("SELFTEST vibrate error: %v", err)
	}

	step(200 * time.Millisecond)
	lg.Printf("SELFTEST done: taps=%d double=%d hold=%d left=%d right=%d up=%d down=%d drag=%d scroll=%d",
		b.counters["tap"], b.counters["double"], b.counters["hold"],
		b.counters["swipe-left"], b.counters["swipe-right"],
		b.counters["swipe-up"], b.counters["swipe-down"],
		b.counters["drag"], b.counters["scroll"])
}
