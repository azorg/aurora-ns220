# 04. Сборка и запуск Fyne-приложения

## 4.1 Проект

```
myapp/
├── go.mod
├── go.sum
└── main.go
```

`go.mod` (минимально):

```
module myapp

go 1.21

require fyne.io/fyne/v2 v2.4.4
```

Подключение патченного GLFW — **без вендоринга**:

```bash
go mod edit -replace github.com/go-gl/glfw/v3.3/glfw=/home/opt/aurora-fyne/go-glfw
go mod tidy
```

> Если предпочитаете vendor: `go mod vendor` + применить
> `patches/glfw-wl-shell.patch` к `vendor/github.com/go-gl/glfw/v3.3/glfw`,
> а затем **обязательно** `go clean -cache` (см. `02-GLFW-PATCH.md` §2.4).

## 4.2 Сборка

Скрипт: [`../scripts/build-fyne.sh`](../scripts/build-fyne.sh)

```bash
scripts/build-fyne.sh -o myapp .
```

Что он выставляет (эквивалент):

```bash
export PATH=/home/opt/go/bin:$PATH
export GOPATH=/home/opt/gopath GOMODCACHE=/home/opt/gopath/pkg/mod GOCACHE=/home/opt/gocache

export PKG_CONFIG_PATH=/home/opt/pc:/home/opt/usr/lib/pkgconfig:/usr/lib/pkgconfig
export CGO_CFLAGS="-D_GNU_SOURCE -DEGL_NO_X11 -I/home/opt/usr/include"
export CGO_LDFLAGS="-L/home/opt/libs \
                   -lwayland-client -lwayland-cursor -lwayland-egl -lxkbcommon \
                   -lEGL -lGLESv2 -lpthread -lm -ldl"

go build -tags "wayland gles egl" -o myapp .
```

**Теги обязательны все три:**

| Тег | Почему |
|---|---|
| `wayland` | GLFW собирается с Wayland-бэкендом (X11 в Aurora нет) |
| `gles` | Fyne использует OpenGL ES 2 (`go-gl/gl/v3.1/gles2`) |
| `egl` | go-gl берёт `eglGetProcAddress`; без тега он тянет `GL/glx.h` (X11) → ошибка сборки |

## 4.3 Запуск

Скрипт: [`../scripts/run-fyne.sh`](../scripts/run-fyne.sh)

```bash
scripts/run-fyne.sh ./myapp
```

Эквивалент переменных:

```bash
XDG_RUNTIME_DIR=/run/user/100000
WAYLAND_DISPLAY=/run/display/wayland-0            # lipstick: сокет именно здесь (не в XDG_RUNTIME_DIR!)
DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/100000/dbus/user_bus_socket

LD_LIBRARY_PATH=/home/opt/mesa/usr/lib            # Mesa вместо libhybris
LIBGL_DRIVERS_PATH=/home/opt/mesa/usr/lib/dri
MESA_LOADER_DRIVER_OVERRIDE=swrast
LIBGL_ALWAYS_SOFTWARE=1
GALLIUM_DRIVER=llvmpipe
EGL_PLATFORM=wayland

FYNE_SCALE=2.5                                     # экран 1200x1920, ~264 dpi
```

## 4.4 Масштаб и размер окна

`FYNE_SCALE` — глобальный масштаб Fyne. Рабочие значения 2.0–3.0.
Дополнительно можно задать размер окна в коде:

```go
w.Resize(fyne.NewSize(900, 500))
```

`wl_shell` в lipstick — окно без декораций, обычно на весь экран; размер контента
определяется приложением.

## 4.5 Пример: минимальное окно

```go
package main

import (
    "fyne.io/fyne/v2"
    "fyne.io/fyne/v2/app"
    "fyne.io/fyne/v2/widget"
)

func main() {
    a := app.New()
    w := a.NewWindow("Fyne on Aurora")
    lbl := widget.NewLabel("Привет с Aurora OS!\nFyne работает.")
    lbl.Alignment = fyne.TextAlignCenter
    w.SetContent(lbl)
    w.Resize(fyne.NewSize(900, 500))
    w.ShowAndRun()
}
```

## 4.6 Проверка успешного запуска

```bash
# 1) процесс жив
pgrep -f myapp

# 2) протокол: окно реально рисуется
WAYLAND_DEBUG=1 scripts/run-fyne.sh ./myapp 2>&1 | grep -c "wl_surface@.*attach"   # >=1
WAYLAND_DEBUG=1 scripts/run-fyne.sh ./myapp 2>&1 | grep -c "get_shell_surface"      # ==1

# 3) какой путь рендеринга используется
grep -oE "libGLES_mali\.so|llvmpipe" /proc/$(pgrep -f myapp)/maps | sort -u
awk '{print "utime="$14" stime="$15}' /proc/$(pgrep -f myapp)/stat
```

На экране должно появиться окно с содержимым.

## 4.7 Известные сообщения (не фатальны)

```
Fyne error: Unable to connect to session D-Bus      → задать DBUS_SESSION_BUS_ADDRESS
PlatformError: Wayland: Standard cursor ... not found → нет темы X11-курсоров
xkbcommon: ERROR: couldn't find a Compose file ...    → LANG/локаль (не влияет на вывод текста)
PlatformError: Wayland: Window position retrieval not supported → wl_shell не даёт позицию
```

## 4.8 Аппаратное ускорение (Mali) и резервный режим

Подробности и замеры — `14-HARDWARE-ACCELERATION.md`. Кратко:

```bash
# аппаратно (libhybris + Mali-T720):
unset LD_LIBRARY_PATH LIBGL_DRIVERS_PATH MESA_LOADER_DRIVER_OVERRIDE GALLIUM_DRIVER
export EGL_PLATFORM=wayland
./myapp

# программно (llvmpipe), если Mali недоступен:
export LD_LIBRARY_PATH=/home/opt/mesa/usr/lib
export LIBGL_DRIVERS_PATH=/home/opt/mesa/usr/lib/dri
export LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe EGL_PLATFORM=wayland
./myapp
```

Готовый лаунчер с авто-выбором: `scripts/run-cterm-hw.sh` (для любого бинарника —
`CTERM_BIN=./myapp CTERM_GPU=auto scripts/run-cterm-hw.sh <аргументы>`).

## 4.9 Сборка и тесты на Linux-хосте (без планшета)

Проверка кода до заливки на устройство. Нужны: Go 1.27.x, gcc, pkg-config и
X11/EGL/Wayland dev-пакеты (`libx11-dev libxcursor-dev libxrandr-dev libxi-dev
libxinerama-dev libxxf86vm-dev libgl1-mesa-dev libegl1-mesa-dev
libwayland-dev libxkbcommon-dev`).

```bash
cd ~/work/clear-ssh
export TMPDIR=$HOME/tmpbuild      # ВАЖНО: /tmp — tmpfs, при сборке линковщик падает
mkdir -p $TMPDIR                  #        с «На устройстве не осталось свободного места»
unset HTTP_PROXY HTTPS_PROXY http_proxy https_proxy

# 1) сборка десктопного варианта (X11, тег gui)
go build -tags "gui" -o /tmp/cterm-host ./cmd/cterm

# 2) юнит-тесты логики OSK (не требуют экрана)
go test -tags gui ./cmd/cterm -run 'TestModsTriState|TestCtrl|TestAlt|TestShift|TestKeySeq' -v

# 3) запуск без дисплея (Xvfb) + снимок окна
Xvfb :99 -screen 0 1200x1920x24 &
DISPLAY=:99 FYNE_SCALE=1.0 /tmp/cterm-host --osk --geometry 92x24 &
DISPLAY=:99 import -window "$(DISPLAY=:99 xdotool search --onlyvisible --name ClearTERM | head -1)" /tmp/win.png

# 4) «клик по клавише» через xdotool — проверка, что тап доходит до PTY
DISPLAY=:99 xdotool mousemove --window <id> 63 319 click 1     # клавиша q
```

Проверено 2026-11-21: сборка OK (38 МБ), все тесты PASS, клики q/w/e/Пробел
дают `qwe ` в строке приглашения.

Замечание по X11-прогону: без оконного менеджера окно получает canvas меньше
окна (845 против 1196), поэтому правый край OSK обрезается — артефакт Xvfb,
на Aurora (полноэкранный `wl_shell`) раскладка влезает целиком.
