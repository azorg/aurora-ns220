package main

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

const (
	trailLen   = 22
	rippleLen  = 6
	rippleLife = 700 * time.Millisecond
	hudLines   = 12
	trailR     = 9
)

var (
	colBG     = color.NRGBA{0x0a, 0x0f, 0x17, 0xff}
	colSpoke  = color.NRGBA{0x2f, 0x8f, 0xff, 0xff}
	colDisc   = color.NRGBA{0xf2, 0x9b, 0x4a, 0xff}
	colCross  = color.NRGBA{0xff, 0x4d, 0x6d, 0xff}
	colTrail  = color.NRGBA{0xff, 0xb3, 0x47, 0xff}
	colHUD    = color.NRGBA{0xd8, 0xe4, 0xf2, 0xff}
	colGreet  = color.NRGBA{0xf4, 0xf8, 0xff, 0xff}
	colGreet2 = color.NRGBA{0x8f, 0xb6, 0xe8, 0xff}
	colRipple = color.NRGBA{0x6a, 0xe0, 0xc4, 0xff}
)

type disc struct {
	obj    *canvas.Circle
	x, y   float64
	vx, vy float64
	r      float64
}

type ripple struct {
	at    fyne.Position
	start time.Time
	obj   *canvas.Circle
}

type touchPoint struct {
	pos fyne.Position
	at  time.Time
}

// board — единственный виджет окна: сцена (OpenGL-отрисовка через Fyne),
// HUD и весь ввод (тач приходит как мышь после патча GLFW).
type board struct {
	widget.BaseWidget

	mu     sync.Mutex
	width  float64
	height float64

	// сцена
	spokes []*canvas.Line
	discs  []*disc

	// ввод: перекрестие, точка, след, «волны» от тапов
	crossH, crossV *canvas.Line
	dot            *canvas.Circle
	trail          []*canvas.Circle
	ripples        []*ripple
	trace          []touchPoint
	hasTouch       bool
	lastPos        fyne.Position

	// HUD
	// индикация прокрутки (двухпальцевый жест в GLFW -> Scrolled)
	scrollTotal float64
	scrollPulse time.Time
	scrollText  *canvas.Text
	scrollTrack *canvas.Rectangle
	scrollThumb *canvas.Rectangle

	hudBG    *canvas.Rectangle
	hudText  []*canvas.Text
	greet    *canvas.Text
	greetSub *canvas.Text
	bg       *canvas.Rectangle
	info     []string
	lastEv   string

	// статистика
	counters map[string]int
	frames   int // запрошенные кадры (тики анимации)
	draws    int // фактические перерисовки
	fps      float64
	drawFPS  float64
	fpsAt    time.Time
	t0       time.Time
	last     time.Time

	// состояние нажатия (жесты)
	press *press
	gen   uint64

	anim       *fyne.Animation
	stopped    chan struct{}
	once       sync.Once
	statsEvery time.Duration
	statsAt    time.Time
}

func newBoard(nSpokes, nDiscs int) *board {
	if nSpokes < 3 {
		nSpokes = 3
	}
	if nDiscs < 0 {
		nDiscs = 0
	}
	b := &board{
		counters: map[string]int{},
		stopped:  make(chan struct{}),
		t0:       time.Now(),
		last:     time.Now(),
		fpsAt:    time.Now(),
		info:     envInfo(),
	}
	b.bg = canvas.NewRectangle(colBG)

	for i := 0; i < nSpokes; i++ {
		l := canvas.NewLine(colSpoke)
		l.StrokeWidth = 2
		b.spokes = append(b.spokes, l)
	}
	for i := 0; i < nDiscs; i++ {
		r := 9 + rand.Float64()*20
		c := canvas.NewCircle(colDisc)
		c.StrokeColor = color.NRGBA{0xff, 0xdc, 0xb0, 0xff}
		c.StrokeWidth = 1
		c.Resize(fyne.NewSize(float32(2*r), float32(2*r)))
		b.discs = append(b.discs, &disc{
			obj: c, r: r,
			vx: (60 + rand.Float64()*170) * sign(),
			vy: (60 + rand.Float64()*170) * sign(),
		})
	}

	b.crossH = canvas.NewLine(colCross)
	b.crossV = canvas.NewLine(colCross)
	b.crossH.StrokeWidth = 1
	b.crossV.StrokeWidth = 1
	b.dot = canvas.NewCircle(color.NRGBA{0xff, 0x4d, 0x6d, 0x40})
	b.dot.StrokeColor = colCross
	b.dot.StrokeWidth = 3
	b.dot.Resize(fyne.NewSize(2*14, 2*14))

	for i := 0; i < trailLen; i++ {
		c := canvas.NewCircle(color.Transparent)
		c.Resize(fyne.NewSize(2*trailR, 2*trailR))
		b.trail = append(b.trail, c)
	}
	for i := 0; i < rippleLen; i++ {
		c := canvas.NewCircle(color.Transparent)
		c.StrokeColor = color.Transparent
		b.ripples = append(b.ripples, &ripple{obj: c})
	}

	b.greet = canvas.NewText("Привет с Aurora OS!", colGreet)
	b.greet.TextSize = 46
	b.greet.TextStyle = fyne.TextStyle{Bold: true}
	b.greet.Alignment = fyne.TextAlignCenter

	b.greetSub = canvas.NewText("Go + Fyne + OpenGL на NS220", colGreet2)
	b.greetSub.TextSize = 22
	b.greetSub.Alignment = fyne.TextAlignCenter

	b.scrollTrack = canvas.NewRectangle(color.NRGBA{0x1c, 0x26, 0x36, 0xff})
	b.scrollThumb = canvas.NewRectangle(color.NRGBA{0x35, 0xd0, 0xa5, 0xff})
	b.scrollText = canvas.NewText("", color.NRGBA{0x35, 0xd0, 0xa5, 0xff})
	b.scrollText.TextSize = 26
	b.scrollText.TextStyle = fyne.TextStyle{Bold: true}
	b.scrollText.Alignment = fyne.TextAlignCenter

	b.hudBG = canvas.NewRectangle(color.NRGBA{0x00, 0x00, 0x00, 0xa8})
	for i := 0; i < hudLines; i++ {
		t := canvas.NewText("", colHUD)
		t.TextSize = 15
		b.hudText = append(b.hudText, t)
	}

	b.ExtendBaseWidget(b)
	return b
}

func sign() float64 {
	if rand.Intn(2) == 0 {
		return -1
	}
	return 1
}

func (b *board) CreateRenderer() fyne.WidgetRenderer {
	r := &boardRenderer{b: b}
	return r
}

type boardRenderer struct{ b *board }

// Refresh вызывается Fyne один раз на фактическую перерисовку — по нему
// считаем настоящий FPS (шт/с), а не запрошенные кадры.
func (r *boardRenderer) Refresh() { r.b.drew() }

func (r *boardRenderer) Layout(size fyne.Size) {
	r.b.layout(size)
}

func (r *boardRenderer) MinSize() fyne.Size { return fyne.NewSize(320, 240) }

func (r *boardRenderer) Destroy() {}

func (r *boardRenderer) Objects() []fyne.CanvasObject {
	b := r.b
	objs := []fyne.CanvasObject{b.bg}
	for _, l := range b.spokes {
		objs = append(objs, l)
	}
	for _, d := range b.discs {
		objs = append(objs, d.obj)
	}
	for _, c := range b.trail {
		objs = append(objs, c)
	}
	for _, rp := range b.ripples {
		objs = append(objs, rp.obj)
	}
	objs = append(objs, b.greet, b.greetSub)
	objs = append(objs, b.crossH, b.crossV, b.dot)
	objs = append(objs, b.scrollTrack, b.scrollThumb, b.scrollText)
	objs = append(objs, b.hudBG)
	for _, t := range b.hudText {
		objs = append(objs, t)
	}
	return objs
}

// layout раскладывает статичные элементы под текущий размер поверхности.
func (b *board) layout(size fyne.Size) {
	b.mu.Lock()
	b.width = float64(size.Width)
	b.height = float64(size.Height)
	w, h := b.width, b.height
	b.mu.Unlock()
	if w <= 0 || h <= 0 {
		return
	}

	b.bg.Resize(size)

	// HUD — левый верхний угол
	pad := float32(10)
	hudW := float32(math.Min(w-2*float64(pad), 940))
	hudH := float32(len(b.hudText))*19 + 2*pad
	b.hudBG.Move(fyne.NewPos(pad, pad))
	b.hudBG.Resize(fyne.NewSize(hudW, hudH))
	for i, t := range b.hudText {
		t.Move(fyne.NewPos(pad+8, pad+6+float32(i)*19))
		t.Resize(fyne.NewSize(hudW-16, 18))
	}

	// приветствие — по центру
	b.greet.Move(fyne.NewPos(float32(w)*0.06, float32(h)*0.42))
	b.greet.Resize(fyne.NewSize(float32(w)*0.88, 60))
	b.greetSub.Move(fyne.NewPos(float32(w)*0.06, float32(h)*0.42+64))
	b.greetSub.Resize(fyne.NewSize(float32(w)*0.88, 30))
}

// start запускает анимацию встроенным механизмом Fyne: Tick вызывается в
// потоке отрисовки (каждый кадр), поэтому fyne.Do не нужен — и очередь
// вызовов fyne.Do не растёт (иначе отложенные fyne.Do «залипают»: кадры
// вытесняют их из очереди).
func (b *board) start(rate int) {
	if rate < 1 {
		rate = 1
	}
	b.anim = fyne.NewAnimation(time.Second/time.Duration(rate), func(float32) {
		select {
		case <-b.stopped:
			return
		default:
		}
		b.frame(time.Now())
	})
	b.anim.RepeatCount = fyne.AnimationRepeatForever
	b.anim.Curve = fyne.AnimationLinear
	b.anim.Start()
}

func (b *board) stop() {
	b.once.Do(func() {
		close(b.stopped)
		if b.anim != nil {
			b.anim.Stop()
		}
	})
}

// frame — один кадр анимации: двигаем объекты, считаем FPS, обновляем HUD.
func (b *board) frame(now time.Time) {
	b.mu.Lock()
	if b.width < 20 || b.height < 20 {
		b.mu.Unlock()
		return
	}
	w, h := b.width, b.height
	dt := now.Sub(b.last).Seconds()
	if dt <= 0 || dt > 0.5 {
		dt = 0.016
	}
	b.last = now
	el := now.Sub(b.t0).Seconds()

	// «радар»: лучи с пульсирующей длиной
	cx, cy := w/2, h/2
	R := math.Min(w, h) * 0.44
	n := float64(len(b.spokes))
	for i, l := range b.spokes {
		base := 2 * math.Pi * float64(i) / n
		a := base + el*0.6
		k := 0.55 + 0.45*math.Abs(math.Sin(a*0.8+float64(i)*0.2))
		ln := R * k
		l.Position1 = fyne.NewPos(float32(cx), float32(cy))
		l.Position2 = fyne.NewPos(float32(cx+ln*math.Cos(a)), float32(cy+ln*math.Sin(a)))
		g := uint8(90 + 130*(1-k))
		l.StrokeColor = color.NRGBA{uint8(40 + g), 0x8f, 0xff, 0xff}
	}

	// круги: упругий отскок от краёв
	for _, d := range b.discs {
		if d.x == 0 && d.y == 0 {
			d.x = d.r + rand.Float64()*(w-2*d.r)
			d.y = d.r + rand.Float64()*(h-2*d.r)
		}
		d.x += d.vx * dt
		d.y += d.vy * dt
		if d.x < d.r {
			d.x, d.vx = d.r, math.Abs(d.vx)
		}
		if d.x > w-d.r {
			d.x, d.vx = w-d.r, -math.Abs(d.vx)
		}
		if d.y < d.r {
			d.y, d.vy = d.r, math.Abs(d.vy)
		}
		if d.y > h-d.r {
			d.y, d.vy = h-d.r, -math.Abs(d.vy)
		}
		d.obj.Move(fyne.NewPos(float32(d.x-d.r), float32(d.y-d.r)))
	}

	// след касаний (затухает по времени)
	for i, c := range b.trail {
		c.FillColor = color.Transparent
		if i < len(b.trace) {
			tp := b.trace[i]
			age := now.Sub(tp.at).Seconds()
			if age < 1.4 {
				al := uint8(200 * (1 - age/1.4))
				c.FillColor = color.NRGBA{colTrail.R, colTrail.G, colTrail.B, al}
				c.Move(fyne.NewPos(tp.pos.X-trailR, tp.pos.Y-trailR))
			}
		}
	}

	// «волны» от тапов
	for _, rp := range b.ripples {
		if rp.start.IsZero() {
			continue
		}
		age := now.Sub(rp.start)
		if age >= rippleLife {
			rp.start = time.Time{}
			rp.obj.FillColor = color.Transparent
			rp.obj.StrokeColor = color.Transparent
			continue
		}
		f := float64(age) / float64(rippleLife)
		rad := float32(8 + 110*f)
		al := uint8(220 * (1 - f))
		rp.obj.FillColor = color.Transparent
		rp.obj.StrokeColor = color.NRGBA{colRipple.R, colRipple.G, colRipple.B, al}
		rp.obj.Move(fyne.NewPos(rp.at.X-rad, rp.at.Y-rad))
		rp.obj.Resize(fyne.NewSize(2*rad, 2*rad))
	}

	// перекрестие в последней точке касания
	if b.hasTouch {
		b.crossH.Position1 = fyne.NewPos(0, b.lastPos.Y)
		b.crossH.Position2 = fyne.NewPos(float32(w), b.lastPos.Y)
		b.crossV.Position1 = fyne.NewPos(b.lastPos.X, 0)
		b.crossV.Position2 = fyne.NewPos(b.lastPos.X, float32(h))
		const ds = 14
		b.dot.Move(fyne.NewPos(b.lastPos.X-ds, b.lastPos.Y-ds))
	}

	// индикатор прокрутки: трек у правого края + «ползунок» + всплывающий текст
	const trackW = 14
	trackH := h * 0.55
	trackY := (h - trackH) / 2
	b.scrollTrack.Move(fyne.NewPos(float32(w-trackW-8), float32(trackY)))
	b.scrollTrack.Resize(fyne.NewSize(trackW, float32(trackH)))
	const thumbH = 90
	span := trackH - thumbH
	pos := math.Mod(b.scrollTotal, 2*span)
	if pos < 0 {
		pos += 2 * span
	}
	if pos > span {
		pos = 2*span - pos
	}
	b.scrollThumb.Move(fyne.NewPos(float32(w-trackW-8), float32(trackY+pos)))
	b.scrollThumb.Resize(fyne.NewSize(trackW, thumbH))
	if now.Sub(b.scrollPulse) < 900*time.Millisecond && b.hasTouch {
		b.scrollText.Text = fmt.Sprintf("⇅ %.0f", b.scrollTotal)
		b.scrollText.Color = color.NRGBA{0x35, 0xd0, 0xa5, 0xff}
		b.scrollText.Move(fyne.NewPos(b.lastPos.X-90, b.lastPos.Y-70))
		b.scrollText.Resize(fyne.NewSize(180, 32))
	} else {
		b.scrollText.Text = ""
	}

	// FPS: тики анимации и фактические отрисовки
	b.frames++
	if d := now.Sub(b.fpsAt); d >= time.Second {
		b.fps = float64(b.frames) / d.Seconds()
		b.drawFPS = float64(b.draws) / d.Seconds()
		b.frames, b.draws = 0, 0
		b.fpsAt = now
	}
	fps, drawFPS := b.fps, b.drawFPS
	counters := make(map[string]int, len(b.counters))
	for k, v := range b.counters {
		counters[k] = v
	}
	lastEv := b.lastEv
	scrollTotal := b.scrollTotal
	lastPos := b.lastPos
	hasTouch := b.hasTouch
	info := b.info
	b.mu.Unlock()

	// HUD (вне блокировки)
	if b.statsEvery > 0 && now.Sub(b.statsAt) >= b.statsEvery {
		b.statsAt = now
		lg.Printf("STATS fps=%.1f drawfps=%.1f taps=%d swipes=%d scroll=%d surface=%.0fx%.0f gpu=%s",
			fps, drawFPS, counters["tap"],
			counters["swipe-left"]+counters["swipe-right"]+counters["swipe-up"]+counters["swipe-down"],
			counters["scroll"], w, h, gpuInUse())
	}

	lines := []string{
		"Hello NS220 — Go + Fyne + OpenGL",
		fmt.Sprintf("FPS %.1f (тики) / %.1f (отрисовка) · поверхность %.0f×%.0f", fps, drawFPS, w, h),
		fmt.Sprintf("тап %d · двойной %d · долгое %d · вторичный %d",
			counters["tap"], counters["double"], counters["hold"], counters["secondary"]),
		fmt.Sprintf("свайпы ←%d →%d ↑%d ↓%d · drag %d · скролл %d",
			counters["swipe-left"], counters["swipe-right"], counters["swipe-up"], counters["swipe-down"],
			counters["drag"], counters["scroll"]),
		fmt.Sprintf("жест: %s · прокрутка всего: %.0f", orDash(lastEv), scrollTotal),
	}
	if hasTouch {
		lines = append(lines, fmt.Sprintf("точка касания: x=%.0f y=%.0f", lastPos.X, lastPos.Y))
	} else {
		lines = append(lines, "точка касания: коснитесь экрана")
	}
	lines = append(lines, "")
	// GPU определяем на ходу: до создания GL-контекста в /proc/self/maps
	// ещё нет libGLES_mali.so, поэтому в envInfo() его нет.
	info = replaceLine(info, "GPU:", "GPU: "+gpuInUse())
	lines = append(lines, info...)
	for i, t := range b.hudText {
		s := ""
		if i < len(lines) {
			s = lines[i]
		}
		if t.Text != s {
			t.Text = s
			t.Color = colHUD
			if i >= 6 {
				t.Color = colGreet2
			}
			t.Refresh()
		}
	}

	// Один запрос перерисовки на кадр: объекты уже получили новые значения
	// полей, отдельных Refresh на каждый объект не нужно (это и было главным
	// тормозом: 50+ запросов перерисовки на кадр).
	b.Refresh()
}

// drew — инкремент счётчика фактических отрисовок (из renderer.Refresh).
func (b *board) drew() {
	b.mu.Lock()
	b.draws++
	b.mu.Unlock()
}

// replaceLine заменяет в наборе строку по префиксу (первое совпадение).
func replaceLine(lines []string, prefix, newLine string) []string {
	for i, l := range lines {
		if strings.HasPrefix(l, prefix) {
			out := make([]string, len(lines))
			copy(out, lines)
			out[i] = newLine
			return out
		}
	}
	return lines
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
