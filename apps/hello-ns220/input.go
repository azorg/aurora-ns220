package main

import (
	"fmt"
	"image/color"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// Пороги распознавания жестов (в пикселях поверхности / времени).
const (
	swipeMinDist = 60.0
	swipeMaxDur  = 900 * time.Millisecond
	holdDelay    = 650 * time.Millisecond
	holdMaxDist  = 14.0
)

// press — состояние текущего касания (для свайпов/долгого нажатия).
type press struct {
	at        time.Time
	from      fyne.Position
	last      fyne.Position
	dist      float64
	gen       uint64
	holdFired bool
	released  bool
}

// Обязательные интерфейсы Fyne: тап, двойной тап, вторичный тап,
// прокрутка, перетаскивание, мышь/курсор.
var (
	_ fyne.Tappable          = (*board)(nil)
	_ fyne.DoubleTappable    = (*board)(nil)
	_ fyne.SecondaryTappable = (*board)(nil)
	_ fyne.Scrollable        = (*board)(nil)
	_ fyne.Draggable         = (*board)(nil)
	_ desktop.Mouseable      = (*board)(nil)
	_ desktop.Cursorable     = (*board)(nil)
)

func (b *board) Tapped(e *fyne.PointEvent) {
	b.note("tap", "тап")
	if vibroOnTap.Load() {
		// 25 мс мотор не успевает раскрутиться (ощущается как «нет вибро»);
		// 60 мс — короткий отчётливый клик (проверка: bugs B036).
		lg.Printf("VIBRO tap 60ms")
		go func() { _, _ = Vibrate(60 * time.Millisecond) }()
	}
	if beepOnTap.Load() {
		go func() { _ = PlayTone(880, 60*time.Millisecond) }()
	}
	b.touch(e.Position)
	b.ripple(e.Position)
}

func (b *board) DoubleTapped(e *fyne.PointEvent) {
	b.note("double", "двойной тап")
	if vibroOnTap.Load() {
		lg.Printf("VIBRO double 90ms")
		go func() { _, _ = Vibrate(90 * time.Millisecond) }()
	}
	b.touch(e.Position)
	b.ripple(e.Position)
}

func (b *board) TappedSecondary(e *fyne.PointEvent) {
	b.note("secondary", "вторичный тап")
	b.touch(e.Position)
}

func (b *board) Scrolled(e *fyne.ScrollEvent) {
	b.note("scroll", fmt.Sprintf("прокрутка dy=%.0f dx=%.0f", e.Scrolled.DY, e.Scrolled.DX))
	b.touch(e.Position)
	b.mu.Lock()
	b.scrollTotal += float64(e.Scrolled.DY)
	b.scrollPulse = time.Now()
	b.mu.Unlock()
}

func (b *board) MouseDown(e *desktop.MouseEvent) {
	b.mu.Lock()
	b.gen++
	p := &press{at: time.Now(), from: e.Position, last: e.Position, gen: b.gen}
	b.press = p
	g := p.gen
	b.mu.Unlock()

	b.touch(e.Position)

	// Долгое нажатие: если за holdDelay не сдвинулись — сообщаем.
	time.AfterFunc(holdDelay, func() { b.checkHold(g) })
}

func (b *board) MouseUp(e *desktop.MouseEvent) {
	// Нажатие НЕ сбрасываем: Fyne вызывает MouseUp перед DragEnd, и если
	// обнулить состояние здесь, DragEnd не сможет классифицировать свайп.
	// Состояние снимает DragEnd; для «просто тапа» (без drag) его заменит
	// следующее MouseDown.
	b.mu.Lock()
	if b.press != nil && b.press.gen == b.gen {
		b.press.released = true
		b.press.last = e.Position
	}
	b.mu.Unlock()
	b.touch(e.Position)
}

func (b *board) Dragged(e *fyne.DragEvent) {
	b.mu.Lock()
	if b.press != nil {
		b.press.dist += math.Hypot(float64(e.Dragged.DX), float64(e.Dragged.DY))
		b.press.last = e.Position
	}
	b.mu.Unlock()
	b.touch(e.Position)
}

func (b *board) DragEnd() {
	now := time.Now()

	b.mu.Lock()
	p := b.press
	b.press = nil
	b.mu.Unlock()
	if p == nil {
		return
	}
	dx := float64(p.last.X - p.from.X)
	dy := float64(p.last.Y - p.from.Y)
	dist := p.dist
	if dist < 1 {
		dist = math.Hypot(dx, dy)
	}
	dur := now.Sub(p.at)

	switch {
	case dist >= swipeMinDist && dur <= swipeMaxDur && math.Abs(dx) > math.Abs(dy)*1.4:
		if dx > 0 {
			b.note("swipe-right", fmt.Sprintf("свайп вправо (%.0f px)", dx))
		} else {
			b.note("swipe-left", fmt.Sprintf("свайп влево (%.0f px)", dx))
		}
	case dist >= swipeMinDist && dur <= swipeMaxDur && math.Abs(dy) > math.Abs(dx)*1.4:
		if dy > 0 {
			b.note("swipe-down", fmt.Sprintf("свайп вниз (%.0f px)", dy))
		} else {
			b.note("swipe-up", fmt.Sprintf("свайп вверх (%.0f px)", dy))
		}
	case dist >= swipeMinDist:
		b.note("drag", fmt.Sprintf("перетаскивание (%.0f px, %d мс)", dist, dur.Milliseconds()))
	case p.holdFired:
		// уже отчитались как «долгое нажатие»
	}
}

// checkHold — срабатывание таймера долгого нажатия.
func (b *board) checkHold(gen uint64) {
	b.mu.Lock()
	p := b.press
	if p == nil || p.gen != gen || p.dist >= holdMaxDist || p.holdFired || p.released {
		b.mu.Unlock()
		return
	}
	p.holdFired = true
	pos := p.from
	b.mu.Unlock()
	b.note("hold", "долгое нажатие")
	b.ripple(pos)
}

func (b *board) Cursor() desktop.Cursor { return desktop.CrosshairCursor }

// touch запоминает точку касания и кладёт её в след (для перекрестия).
func (b *board) touch(pos fyne.Position) {
	b.mu.Lock()
	b.hasTouch = true
	b.lastPos = pos
	b.trace = append([]touchPoint{{pos: pos, at: time.Now()}}, b.trace...)
	if len(b.trace) > trailLen {
		b.trace = b.trace[:trailLen]
	}
	b.mu.Unlock()
}

// note фиксирует распознанный жест: счётчик, HUD-строка, журнал.
func (b *board) note(kind, desc string) {
	b.mu.Lock()
	b.counters[kind]++
	b.lastEv = desc
	b.mu.Unlock()
	lg.Printf("GESTURE %-12s %s", kind, desc)
}

// ripple запускает «волну» в точке касания (визуальная обратная связь).
func (b *board) ripple(pos fyne.Position) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, rp := range b.ripples {
		if rp.start.IsZero() {
			rp.at = pos
			rp.start = time.Now()
			rp.obj.FillColor = color.Transparent
			rp.obj.StrokeColor = color.NRGBA{colRipple.R, colRipple.G, colRipple.B, 0xc0}
			rp.obj.StrokeWidth = 3
			return
		}
	}
}
