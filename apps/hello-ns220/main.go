// aurora-hello-ns220 — демонстрационное приложение «Hello world» для планшета
// NS220 (MVK-2020) под Aurora OS.
//
// Доказывает, что на устройстве возможна полноценная разработка GUI на Go:
//   - анимация отрисовывается через OpenGL (Fyne/GLFW → EGL, Mali-T720);
//   - показывается точка касания и распознанные жесты (тап, двойной тап,
//     долгое нажатие, свайпы, drag, прокрутка);
//   - на экране — фактическое окружение: ОС, ядро, архитектура, GPU, режим
//     IMA (подпись исполняемых файлов), версии Fyne/GLFW.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
)

// setupEnv выставляет окружение, нужное Fyne/GLFW на Aurora, если его не задали
// извне. Делает бинарник самодостаточным: .desktop может иметь
// Exec=/usr/bin/aurora-hello-ns220 без shell-обёртки — lipstick запускает
// .desktop через invoker, и shell-скрипт там ведёт себя непредсказуемо (B035).
func setupEnv() {
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		os.Setenv("XDG_RUNTIME_DIR", fmt.Sprintf("/run/user/%d", os.Getuid()))
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		os.Setenv("WAYLAND_DISPLAY", "/run/display/wayland-0")
	}
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		os.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+os.Getenv("XDG_RUNTIME_DIR")+"/dbus/user_bus_socket")
	}
	if os.Getenv("EGL_PLATFORM") == "" {
		os.Setenv("EGL_PLATFORM", "wayland")
	}
	if os.Getenv("FYNE_SCALE") == "" {
		os.Setenv("FYNE_SCALE", "1.0")
	}
	lang := os.Getenv("LANG")
	if lang == "" || lang == "C" || lang == "POSIX" {
		lang = "ru_RU.UTF-8"
		os.Setenv("LANG", lang)
	}
	if os.Getenv("LC_ALL") == "" {
		os.Setenv("LC_ALL", lang)
	}
	if os.Getenv("XCOMPOSEFILE") == "" {
		for _, f := range []string{
			"/usr/share/X11/locale/en_US.UTF-8/Compose",
			"/usr/share/X11/locale/ru_RU.UTF-8/Compose",
			"/usr/share/X11/xkb/compose/en_US.UTF-8",
		} {
			if _, err := os.Stat(f); err == nil {
				os.Setenv("XCOMPOSEFILE", f)
				break
			}
		}
	}
	// не подмешивать Mesa llvmpipe поверх libhybris
	os.Unsetenv("LD_LIBRARY_PATH")
	os.Unsetenv("MESA_LOADER_DRIVER_OVERRIDE")
	os.Unsetenv("GALLIUM_DRIVER")
}

// setupLog пишет журнал в stderr и в файл (из меню Aurora stderr не видно).
// Файл обрезается при размере > 1 МиБ.
func setupLog() {
	path := os.Getenv("HELLO_LOG")
	if path == "" {
		if h, err := os.UserHomeDir(); err == nil {
			path = h + "/hello-ns220.log"
		}
	}
	var w io.Writer = os.Stderr
	if path != "" {
		if fi, err := os.Stat(path); err == nil && fi.Size() > 1<<20 {
			_ = os.Remove(path)
		}
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			w = io.MultiWriter(os.Stderr, f)
		}
	}
	lg = log.New(w, "", log.Ltime|log.Lmicroseconds)
	lg.Printf("START pid=%d uid=%d wayland=%s egl=%s", os.Getpid(), os.Getuid(),
		os.Getenv("WAYLAND_DISPLAY"), os.Getenv("EGL_PLATFORM"))
}

func main() {
	setupEnv()
	setupLog()
	fps := flag.Int("fps", 60, "целевая частота кадров анимации")
	spokes := flag.Int("spokes", 28, "число лучей «радара» (нагрузка на отрисовку)")
	bounces := flag.Int("bounces", 14, "число движущихся кругов")
	shot := flag.String("shot", "", "сохранить PNG-снимок окна и выйти")
	shotDelay := flag.Duration("shot-delay", 2500*time.Millisecond, "задержка перед снимком")
	stats := flag.Duration("stats", 5*time.Second, "интервал журнала статистики (0 — выкл.)")
	micOn := flag.Bool("mic", true, "измерять уровень микрофона (libpulse)")
	accelOn := flag.Bool("accel", true, "читать G-сенсор (sensorfw)")
	gpsOn := flag.Bool("gps", true, "читать GPS (geoclue-hybris)")
	beep := flag.Bool("beep", false, "пищать при тапе (по умолчанию выкл.)")
	rotDeg := flag.Int("rot", 0, "поворот сцены при старте: 0/90/180/270")
	buttonsOn := flag.Bool("buttons", true, "читать аппаратные кнопки (evdev)")
	modemOn := flag.Bool("modem", true, "читать модем/SIM через ofono")
	keepAwake := flag.Bool("keepawake", true, "не давать экрану гаснуть (сенсоры спят при погашенном экране)")
	vibro := flag.Bool("vibro", true, "виброклик при тапе (по умолчанию вкл.)")
	fullscreen := flag.Bool("fullscreen", true, "развернуть окно на весь экран (ресайз, не SetFullScreen)")
	selftest := flag.Bool("selftest", false, "прогнать тап/двойной/долгое/свайпы/drag/скролл без тачскрина")
	winW := flag.Float64("w", 1000, "ширина окна (если не fullscreen)")
	winH := flag.Float64("h", 1600, "высота окна (если не fullscreen)")
	flag.Parse()

	a := app.NewWithID("ru.aurora.hello.ns220")
	a.Settings().SetTheme(newBigTheme())

	w := a.NewWindow(fmt.Sprintf("Hello NS220 — %s", runtime.Version()))

	b := newBoard(*spokes, *bounces)
	defer b.stop()

	// --shot=PATH: через shot-delay сохранить PNG содержимого окна и выйти.
	// Нужно для удалённой проверки (скриншот через lipstick-D-Bus закрыт политикой).
	if *shot != "" {
		go func() {
			time.Sleep(*shotDelay)
			fyne.Do(func() {
				if err := saveShot(w, *shot); err != nil {
					lg.Printf("SHOT error: %v", err)
				} else {
					lg.Printf("SHOT saved: %s", *shot)
				}
				a.Quit()
			})
			b.stop()
		}()
	}

	// Один экземпляр: если уже запущено — просим показать окно и выходим.
	si, already := acquireSingleInstance(w)
	if already {
		return
	}
	defer si.close()

	if *micOn {
		StartMic()
	}
	if *accelOn {
		StartAccel()
	}
	if *gpsOn {
		StartGPS()
	}
	if *buttonsOn {
		StartButtons()
	}
	if *modemOn {
		StartModem()
	}
	if *keepAwake {
		StartKeepAwake()
	}
	b.SetRot(*rotDeg)
	beepOnTap.Store(*beep)
	vibroOnTap.Store(*vibro)

	p := newPanel(func() {
		lg.Printf("EXIT по кнопке")
		b.stop()
		a.Quit()
	}, b)
	p.beep.SetChecked(*beep)
	p.vibro.SetChecked(*vibro)

	w.SetContent(container.NewBorder(nil, p.view(), nil, nil, b))
	if *fullscreen {
		// Не SetFullScreen: на wl_shell (lipstick) он даёт чёрный кадр.
		// Разворачиваем окно ресайзом до размера экрана — это работает.
		go func() {
			time.Sleep(300 * time.Millisecond)
			fyne.Do(func() {
				sz := screenSize()
				if sz.Width <= 0 {
					sz = fyne.NewSize(1200, 1920)
				}
				w.Resize(sz)
				lg.Printf("SIZE screen=%.0fx%.0f canvas=%.0fx%.0f", sz.Width, sz.Height,
					w.Canvas().Size().Width, w.Canvas().Size().Height)
			})
		}()
	} else {
		w.Resize(fyne.NewSize(float32(*winW), float32(*winH)))
	}

	// Аккуратное завершение по SIGINT/SIGTERM (запуск из скриптов/по ssh).
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sig
		lg.Printf("SIGNAL %v — выходим", s)
		b.stop()
		fyne.Do(func() { a.Quit() })
	}()
	// Диагностика жизненного цикла: если окно закроет WM/lipstick, увидим это в журнале.
	w.SetOnClosed(func() { lg.Printf("WINDOW closed (Fyne)") })
	a.Lifecycle().SetOnStopped(func() { lg.Printf("APP stopped") })

	b.statsEvery = *stats
	b.start(*fps)
	if *selftest {
		go b.selfTest()
	}
	w.ShowAndRun()
}
