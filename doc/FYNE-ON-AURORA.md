# Fyne.io GUI на Aurora OS (MVK-2020 / NS220) — как это сделано

**Статус:** РАБОТАЕТ. Проверено: окно Fyne отображается на экране планшета (Aurora 4.0.2.340),
рендеринг **аппаратный — Mali-T720 через libhybris** (`EGL_PLATFORM=wayland`), Mesa llvmpipe
остаётся резервным программным путём; ввод через `wl_shell`.

> **Обновление 2026-11-21:** прежний вывод «системный libhybris-EGL не подходит» **опровергнут**.
> Достаточно `EGL_PLATFORM=wayland`: hybris сам заворачивает `wl_egl_window` в `WaylandNativeWindow`,
> буферы идут через `android_wlegl`, lipstick композитит на GPU. Замеры и детали —
> [`14-HARDWARE-ACCELERATION.md`](14-HARDWARE-ACCELERATION.md).

Документ описывает: почему «просто так» не работает, какие именно правки нужны, как собрать и запустить, какие есть ограничения и что делать дальше (упаковка в RPM, tray-иконка, тач).

---

## 1. Кратко: из чего состоит решение

```
┌──────────────────────────────┐
│ Go-приложение на Fyne        │
└──────────────┬───────────────┘
               │ go-gl/glfw (CGO)
┌──────────────▼───────────────┐
│ GLFW 3.3.x  + ПАТЧ wl_shell  │  ← наш патч (xdg-shell в lipstick отсутствует)
└──────────────┬───────────────┘
               │ EGL + GLES2
┌──────────────▼───────────────┐
│ libhybris EGL → Mali-T720    │  ← аппаратный GL (EGL_PLATFORM=wayland)
│ (Mesa llvmpipe — резерв)     │     программный fallback через LD_LIBRARY_PATH
└──────────────┬───────────────┘
               │ wl_shm / wl_surface
┌──────────────▼───────────────┐
│ lipstick (QtWayland compositor)│  Wayland-сокет /run/display/wayland-0
└──────────────────────────────┘
```

Три «ключа» к успеху:

1. **IMA отключён** в системе (иначе ни один наш бинарник не запускается — см. §2).
2. **GLFW пропатчен**: fallback с `xdg-shell` на `wl_shell` (в Aurora композитор отдаёт только `wl_shell` v1).
3. **Рендеринг**: основной путь — системный libhybris-EGL + Mali при `EGL_PLATFORM=wayland`;
   резервный — **Mesa llvmpipe** через `LD_LIBRARY_PATH` (см. `14-HARDWARE-ACCELERATION.md`).

---

## 2. Предварительное условие: снятие IMA (без этого ничего не работает)

На устройстве ядро грузится с `ima_appraise=enforce module.sig_enforce=1` — запуск любых не-ОМР бинарников блокируется (`EACCES`), включая Go-программы и `.so` через `dlopen`.

**Как сняли** (файл `boot-imaoff-v4.img`, md5 `0ff5bf5bea5e7d8396d1de1cd4467de8`):

* В boot-образе (`ANDROID!`) командная строка лежит по смещению **0x40** (512 байт). Поле `0x30..0x3F` — это `name[16]`, его трогать НЕЛЬЗЯ (частая ошибка: писать cmdline по 0x38 — ломает `bootopt=` и устройство не грузится).
* Было:
  ```
  bootopt=64S3,32N2,32N2 selinux=0 ima_appraise=enforce ima_hash=sha256 ima_tcb module.sig_enforce=1 clearmem=on security=diehard
  ```
* Стало:
  ```
  bootopt=64S3,32N2,32N2 selinux=0 ima_appraise=off ima_hash=sha256 ima_tcb module.sig_enforce=0 clearmem=on security=diehard
  ```
* Прошивка: `fastboot flash boot boot-imaoff-v4.img` или `dd` в `/dev/block/platform/mtk-msdc.0/by-name/boot`, либо через preloader/DA (см. §9).

Проверка: `cat /proc/cmdline | tr ' ' '\n' | grep ima_` → `ima_appraise=off`, `module.sig_enforce=0`.
После этого `gcc hello.c -o hello && ./hello` работает.

⚠️ Откат: заводской образ `boot-stock-16m.img` (`mtkrun.sh w boot boot-stock-16m.img --skipwdt`, §9).

---

## 3. Факты о композиторе Aurora (почему нужен патч)

`lipstick` (QtWaylandCompositor) отдаёт на `/run/display/wayland-0`:

```
wl_compositor v3, wl_shm v1, wl_seat v3, wl_output v2, wl_data_device_manager v1,
wl_shell v1, qt_surface_extension v1, qt_windowmanager v1, qt_touch_extension v1,
qt_hardware_integration v1, android_wlegl v2, alien_manager v1, lipstick_recorder_manager v1
```

**Нет `xdg_wm_base` и нет `zxdg_shell_v6`.** GLFW 3.3/3.4 требует xdg-shell → падает с
`PlatformError: Wayland: Failed to find xdg-shell in your compositor`.
Есть только устаревший `wl_shell` v1 — им пользуется Qt (поэтому Qt/QML приложения работают «из коробки»), и именно на него мы переводим GLFW.

Полезная проверка глобалов (клиент на libwayland-client):

```c
#include <stdio.h>
#include <string.h>
#include <wayland-client.h>
static void global(void*d, struct wl_registry*r, uint32_t n, const char*i, uint32_t v){
    (void)d;(void)r;(void)n; printf("  %-45s v%u\n", i, v);
}
static const struct wl_registry_listener li = { global, NULL };
int main(void){
    struct wl_display *dpy = wl_display_connect(NULL);
    if(!dpy) return 1;
    struct wl_registry *reg = wl_display_get_registry(dpy);
    wl_registry_add_listener(reg, &li, NULL);
    wl_display_roundtrip(dpy);
    return 0;
}
```

---

## 4. Патч GLFW (суть)

Патчится **не системный GLFW, а тот, что компилируется внутри Go-модуля** `github.com/go-gl/glfw/v3.3/glfw` (он содержит полные исходники GLFW и собирает их через cgo). Файлы:

`vendor/github.com/go-gl/glfw/v3.3/glfw/glfw/src/{wl_platform.h, wl_init.c, wl_window.c}`

### 4.1 `wl_platform.h` — поля

```c
/* в _GLFWplatform, рядом с compositor: */
    struct wl_shell*            shell;

/* в _GLFWwindowWayland, рядом с surface: */
    struct wl_shell_surface*    shellSurface;
```

### 4.2 `wl_init.c` — биндинг глобала

```c
    else if (strcmp(interface, "xdg_wm_base") == 0)
    {
        _glfw.wl.wmBase = wl_registry_bind(registry, name, &xdg_wm_base_interface, 1);
        xdg_wm_base_add_listener(_glfw.wl.wmBase, &wmBaseListener, NULL);
    }
    else if (strcmp(interface, "wl_shell") == 0)      /* ← добавлено */
    {
        _glfw.wl.shell = wl_registry_bind(registry, name, &wl_shell_interface, 1);
    }
```

И проверка инициализации — не требовать xdg-shell:

```c
    if (!_glfw.wl.wmBase && !_glfw.wl.shell)   /* было: if (!_glfw.wl.wmBase) */
```

### 4.3 `wl_window.c` — создание окна через wl_shell

```c
static void wlShellSurfaceHandlePing(void* userData, struct wl_shell_surface* s, uint32_t serial)
{
    wl_shell_surface_pong(s, serial);
}

static void wlShellSurfaceHandleConfigure(void* userData, struct wl_shell_surface* s,
                                          uint32_t edges, int32_t width, int32_t height)
{
    _GLFWwindow* window = (_GLFWwindow*) userData;

    if (width > 0 && height > 0)
    {
        window->wl.width = width;
        window->wl.height = height;
        if (window->wl.egl.window)
            wl_egl_window_resize(window->wl.egl.window, width, height, 0, 0);
        _glfwInputFramebufferSize(window, width, height);
        _glfwInputWindowSize(window, width, height);
    }

    /* КРИТИЧНО: в xdg-пути visible ставится в configure; для wl_shell делаем то же */
    if (!window->wl.visible)
    {
        window->wl.visible = GLFW_TRUE;
        _glfwInputWindowDamage(window);
    }
}

static const struct wl_shell_surface_listener wlShellSurfaceListener =
{
    wlShellSurfaceHandlePing,
    wlShellSurfaceHandleConfigure,
    NULL
};

static GLFWbool createWlShellObjects(_GLFWwindow* window)
{
    if (window->wl.shellSurface)          /* защита от повторного создания */
        return GLFW_TRUE;

    window->wl.shellSurface = wl_shell_get_shell_surface(_glfw.wl.shell, window->wl.surface);
    if (!window->wl.shellSurface)
    {
        _glfwInputError(GLFW_PLATFORM_ERROR, "Wayland: Failed to create wl_shell_surface");
        return GLFW_FALSE;
    }

    wl_shell_surface_add_listener(window->wl.shellSurface, &wlShellSurfaceListener, window);

    if (window->wl.title)
        wl_shell_surface_set_title(window->wl.shellSurface, window->wl.title);

    wl_shell_surface_set_toplevel(window->wl.shellSurface);

    wl_surface_commit(window->wl.surface);
    wl_display_roundtrip(_glfw.wl.display);
    return GLFW_TRUE;
}
```

В `createShellObjects()` — fallback и защита от двойного вызова:

```c
static GLFWbool createShellObjects(_GLFWwindow* window)
{
    if (window->wl.shellSurface || window->wl.xdg.surface)   /* ← защита */
        return GLFW_TRUE;

    if (!_glfw.wl.wmBase)
    {
        if (_glfw.wl.shell)
            return createWlShellObjects(window);             /* ← fallback */
        _glfwInputError(GLFW_PLATFORM_ERROR,
                        "Wayland: Failed to find xdg-shell or wl_shell in your compositor");
        return GLFW_FALSE;
    }
    ... обычный xdg-путь ...
```

В `destroyShellObjects()`:

```c
    if (window->wl.shellSurface)
    {
        wl_shell_surface_destroy(window->wl.shellSurface);
        window->wl.shellSurface = NULL;
    }
```

`_glfwPlatformShowWindow()` — для wl_shell нет handshake «configure перед map», показываем сразу:

```c
void _glfwPlatformShowWindow(_GLFWwindow* window)
{
    if (!window->wl.xdg.toplevel && !window->wl.shellSurface)
        createShellObjects(window);

    if (window->wl.shellSurface)
    {
        window->wl.visible = GLFW_TRUE;
        wl_surface_commit(window->wl.surface);
        _glfwInputWindowDamage(window);
    }
}
```

### 4.4 Защита всех xdg-вызовов

В wl_shell-режиме `window->wl.xdg.toplevel == NULL`, а сгенерированные libwayland-функции не терпят NULL. Все вызовы вида

```c
        xdg_toplevel_set_maximized(window->wl.xdg.toplevel);
```

обёрнуты:

```c
        if (window->wl.xdg.toplevel)
            xdg_toplevel_set_maximized(window->wl.xdg.toplevel);
```

Аналогично для `xdg_toplevel_set_title/fullscreen/minimized/move/resize/show_window_menu`,
`zxdg_toplevel_decoration_v1_set_mode` (там условие на `window->wl.xdg.decoration`).
В `_glfwPlatformSetWindowTitle` добавлена ветка:

```c
        if (window->wl.xdg.toplevel)
            xdg_toplevel_set_title(window->wl.xdg.toplevel, title);
        else if (window->wl.shellSurface)
            wl_shell_surface_set_title(window->wl.shellSurface, title);
```

> `wl_shell` уже объявлен в `wayland-client.h` — генерировать свой протокол НЕ нужно
> (попытка сгенерировать `wl-shell-client-protocol.h` даёт redefinition).

### 4.5 ⚠️ ГЛАВНАЯ ГРАБЛЯ: cgo-кэш

`c_glfw_lin.go` подключает `.c` через `#include`. Go-кэш **не инвалидируется** при правке
этих файлов → изменения молча не попадают в бинарник (симптом: «патч есть, а поведение
старое», одинаковый размер бинарника).

**Всегда после правки .c:**

```bash
export GOCACHE=/home/opt/gocache
go clean -cache && rm -rf $GOCACHE/*
touch vendor/github.com/go-gl/glfw/v3.3/glfw/c_glfw_lin.go
go build ...
```

Проверка, что патч реально в бинарнике: добавить временный `fprintf(stderr, "TRACE...")` и
убедиться, что он печатается.

---

## 5. Сборка окружения на планшете (с нуля)

Всё ставится в `/home/opt` (важно: `/home` по умолчанию смонтирован **noexec** → нужен
`mount -o remount,exec /home`, иначе Go не запустится).

### 5.1 Системные пакеты (zypper, репозитории OMP публичные)

```bash
zypper -n install cmake wayland-devel libxkbcommon-devel gcc-c++ \
                 qt5-qtdeclarative-devel qt5-qmake        # Qt — для альтернативы QML
```

### 5.2 Заголовки, которых нет в репо (распаковка RPM без установки)

Установка `mesa-llvmpipe-*-devel` конфликтует с adaptation (libhybris), поэтому заголовки
берём распаковкой:

```bash
B="https://releases.omprussia.ru/releases/4.0.2.340/omp/armv7hl/pj:oss:/armv7hl"
for n in mesa-llvmpipe-libEGL-devel mesa-llvmpipe-libGLESv2-devel mesa-llvmpipe-libGL-devel; do
  f="$n-21.1.6+git1-1.4.10.omp.armv7hl.rpm"
  curl -sO "$B/$f" && rpm2cpio "$f" | (mkdir -p /home/opt && cd /home/opt && cpio -idm --quiet)
done
# → /home/opt/usr/include/{EGL,GLES2,GLES3,GL,KHR}, /home/opt/usr/lib/pkgconfig/{egl,glesv2}.pc
```

Плюс отсутствующий kernel-header (в kernel-headers 3.18 его нет):

```bash
curl -sSL -o /home/opt/usr/include/linux/input-event-codes.h \
  https://raw.githubusercontent.com/torvalds/linux/master/include/uapi/linux/input-event-codes.h
```

### 5.3 pkg-config заглушки и симлинки

`/home/opt/pc/libdrm.pc` (egl.pc требует libdrm, которого нет):

```
prefix=/usr
Name: libdrm
Description: stub
Version: 2.4.100
Libs:
Cflags:
```

`/home/opt/pc/wayland-egl.pc`:

```
prefix=/usr
includedir=${prefix}/include
libdir=${prefix}/lib
Name: wayland-egl
Description: Wayland EGL platform
Version: 1.19.0
Requires: wayland-client
Libs: -L${libdir} -lwayland-egl
Cflags: -I${includedir}
```

Симлинки `lib*.so` (в системе только версионированные `.so.N`):

```bash
mkdir -p /home/opt/libs
ln -sf /usr/lib/libEGL.so.1            /home/opt/libs/libEGL.so
ln -sf /usr/lib/libGLESv2.so.2         /home/opt/libs/libGLESv2.so
ln -sf /usr/lib/libwayland-client.so.0 /home/opt/libs/libwayland-client.so
ln -sf /usr/lib/libwayland-cursor.so.0 /home/opt/libs/libwayland-cursor.so
ln -sf /usr/lib/libwayland-egl.so.1    /home/opt/libs/libwayland-egl.so
ln -sf /usr/lib/libxkbcommon.so.0      /home/opt/libs/libxkbcommon.so
```

### 5.4 Go

```bash
curl -sSLo go.tgz https://go.dev/dl/go1.22.12.linux-armv6l.tar.gz   # armv6l работает на ARMv7
tar -xzf go.tgz -C /home/opt        # → /home/opt/go/bin/go
/home/opt/go/bin/go version         # go1.22.12 linux/arm
```

### 5.5 GLFW (для нативных C-тестов; для Fyne не обязателен)

Fyne собирает GLFW из Go-модуля сам. Отдельная сборка GLFW нужна только если хочется
C-утилиты. Если собираете — нужны `-DGLFW_BUILD_X11=OFF -DGLFW_BUILD_WAYLAND=ON`.

### 5.6 Mesa llvmpipe (программный GL)

```bash
for n in mesa-llvmpipe-libEGL mesa-llvmpipe-libGLESv2 mesa-llvmpipe-libglapi \
         mesa-llvmpipe-libgbm mesa-llvmpipe-dri-swrast-driver libdrm; do
  f=$(... найти href в repodata ...); curl -sO "$B/$f"
  rpm2cpio "$f" | (cd /home/opt/mesa && cpio -idm --quiet)
done
# симлинки: libEGL.so.1, libGLESv2.so.2, libglapi.so.0, libgbm.so.1, libdrm.so.2
```

Проверка, что нужные файлы на месте: `/home/opt/mesa/usr/lib/{libEGL.so.1,libGLESv2.so.2,libgbm.so.1}`, `/home/opt/mesa/usr/lib/dri/swrast_dri.so`.

---

## 6. Сборка Fyne-приложения

### 6.1 Проект

`go.mod`:

```
module fyneedemo
go 1.21
require fyne.io/fyne/v2 v2.4.4
```

`main.go` (пример):

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

### 6.2 Патч GLFW в vendor + сборка

```bash
cd <project>
go mod vendor                     # копия GLFW с исходниками в vendor/
# применить патчи из §4 (wl_platform.h, wl_init.c, wl_window.c)
go clean -cache && touch vendor/github.com/go-gl/glfw/v3.3/glfw/c_glfw_lin.go

export PATH=/home/opt/go/bin:$PATH
export GOMODCACHE=/home/opt/gopath/pkg/mod GOCACHE=/home/opt/gocache GOPATH=/home/opt/gopath
export PKG_CONFIG_PATH=/home/opt/pc:/home/opt/usr/lib/pkgconfig:/usr/lib/pkgconfig
export CGO_CFLAGS="-D_GNU_SOURCE -DEGL_NO_X11 -I/home/opt/usr/include"
export CGO_LDFLAGS="-L/home/opt/libs -lwayland-client -lwayland-cursor -lwayland-egl \
                    -lxkbcommon -lEGL -lGLESv2 -lpthread -lm -ldl"

go build -mod=vendor -tags "wayland gles egl" -o myapp .
```

**Обязательные теги:**
* `wayland` — GLFW использует Wayland-бэкенд;
* `gles` — Fyne использует OpenGL ES 2 (`go-gl/gl/v3.1/gles2`);
* `egl` — go-gl берёт `eglGetProcAddress`, иначе тянет `GL/glx.h` (X11) и падает.

### 6.3 Запуск

```bash
XDG_RUNTIME_DIR=/run/user/100000 \
WAYLAND_DISPLAY=/run/display/wayland-0 \
DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/100000/dbus/user_bus_socket \
LD_LIBRARY_PATH=/home/opt/mesa/usr/lib \
LIBGL_DRIVERS_PATH=/home/opt/mesa/usr/lib/dri \
MESA_LOADER_DRIVER_OVERRIDE=swrast \
LIBGL_ALWAYS_SOFTWARE=1 \
GALLIUM_DRIVER=llvmpipe \
EGL_PLATFORM=wayland \
FYNE_SCALE=2.5 \
./myapp
```

`FYNE_SCALE` — обязательный практический параметр: экран 1200×1920 при ~264 dpi, при
масштабе 1.0 текст нечитаемо мелкий. Рабочее значение 2.0–3.0.

---

## 7. Диагностика (что и как проверять)

| Симптом | Причина / решение |
|---|---|
| `Failed to find xdg-shell in your compositor` | патч GLFW не применён или **не пересобран** (cgo-кэш, §4.5) |
| `eglCreateWindowSurface: Assertion ANativeWindow ... MAGIC` | не выставлен `EGL_PLATFORM=wayland` (или подмешан Mesa-`libEGL`) | `export EGL_PLATFORM=wayland` и убрать `/home/opt/mesa/usr/lib` из `LD_LIBRARY_PATH`; программный путь: `LD_LIBRARY_PATH=/home/opt/mesa/usr/lib` + `GALLIUM_DRIVER=llvmpipe` (см. `14-HARDWARE-ACCELERATION.md`) |
| окно не появляется, но процесс жив | `window->wl.visible` не выставлен (см. configure/show в §4.3); проверять `WAYLAND_DEBUG=1` — должны быть `wl_surface.attach`, `damage`, `commit` |
| ANR «приложение не отвечает» | окно создано, но буферы не отправляются (та же причина, что выше) |
| текст мелкий | `FYNE_SCALE=2.5` + `w.Resize(fyne.NewSize(...))` |
| `Unable to connect to session D-Bus` | задать `DBUS_SESSION_BUS_ADDRESS` (не фатально) |
| `Standard cursor ... not found` | в Aurora нет курсоров тем X11 — не фатально (можно завести свою тему в `~/.icons`) |
| `couldn't find a Compose file for locale "C"` | не фатально; помогает `LANG=ru_RU.UTF-8` при наличии локалей |
| `Permission denied` при запуске бинарника из `/home` | `/home` c `noexec` → `mount -o remount,exec /home` |
| Go на `/home` «Permission denied» | то же самое |

Полезные проверки:

```bash
# сколько протокольных событий реально произошло
WAYLAND_DEBUG=1 ./myapp 2>&1 | grep -c "wl_surface@.*attach"

# трассы GLFW (если оставили fprintf)
grep TRACE /tmp/fyne.tr

# CPU-время процесса (llvmpipe всегда грузит CPU при отрисовке)
awk '{print "utime="$14" stime="$15}' /proc/$(pgrep -f myapp)/stat
```

**Скриншоты:** `/dev/fb0` недоступен (I/O error, его держит hwcomposer), `grim/scrot/wayshot`
в системе нет. Пока единственный надёжный способ «увидеть» картинку — глазами оператора
или реализовать клиент `lipstick_recorder_manager` (задел на будущее).

---

## 8. Что уже проверено и работает

* Окно Fyne отображается, рисуется (llvmpipe), `attach/damage/commit` идут.
* Масштаб/размер окна управляются (`FYNE_SCALE`, `w.Resize`).
* Qt/QML приложения работают штатно (через `wl_shell`, QtWayland-клиент) — проверено
  отдельным Hello World (`QGuiApplication` + `QQmlApplicationEngine`, `qmlscene` в Aurora
  нерабочий: симлинк на qtchooser без бинарника).

---

## 9. Восстановление/перепрошивка boot (если что-то пошло не так)

Preloader планшета отдаёт **CDC-ACM** (`/dev/ttyACM*`) и говорит на MTK-протоколе:

* handshake: `A0 0A 50 05` → ответ `READY` + `5F F5 AF FA`;
* далее команды MTK (`0xFD` → hw code `0x321` = MT6735), `target config 0x0` — SBC/SLA/DAA
  выключены, устройство незащищённое.

`mtkclient` по serial не успевает (окно preloader'а ~2.5 с), поэтому используется **pty-мост**,
который сам делает handshake и держит сессию, а mtkclient работает через `/tmp/ttyMTK`:

```bash
/tmp/mtkrun.sh w boot boot-stock-16m.img --skipwdt       # заводской boot
/tmp/mtkrun.sh w boot boot-imaoff-v4.img  --skipwdt      # с отключённым IMA
```

Грабли:
* обязателен `--skipwdt` (иначе mtkclient гасит watchdog и зависание становится вечным);
* чанки записи DA — не больше 16 КБ (лучше 4 КБ), ожидание ACK до 5 с;
* не обрывать пакет `0x70 SEND_PARTITION_DATA` — preloader зависнет (лечится сбросом питания);
* при зависании: отключить USB, Power 40–60 с, подключить USB.

---

## 10. Ограничения и планы

| Возможность | Статус |
|---|---|
| Окно Fyne, отрисовка, масштаб | ✅ работает |
| Работа без GPU (llvmpipe) | ✅ работает, но CPU-нагрузка высокая (программный растеризатор) |
| Аппаратное ускорение через libhybris | ⚠️ возможно, но нужен «android-wlegl» путь: hybris-EGL требует `ANativeWindow`, создаваемый через `android_wlegl` (так делает Qt). Требует отдельной работы. |
| Тач-ввод | ⚠️ не проверялось. `wl_seat` v3 отдаёт `touch`; GLFW умеет `wl_touch` → Fyne получает touch-события как мышь/жесты. Проверить и при необходимости включить `qt_touch_extension` не нужно. |
| trayIcon | ❌ в Aurora нет системного трея. Варианты: (а) мини-панель на Qt/QML, которая по D-Bus (StatusNotifier-подобный протокол) показывает иконки и меню наших Fyne-приложений; (б) «свернуть в уведомление» через `ngfd`/notifications; (в) плавающее окно-иконка средствами самого Fyne. |
| Курсоры мыши | ⚠️ нет темы X11-курсоров; можно положить свою (`~/.icons`) либо игнорировать (в Fyne можно скрыть курсор). |
| D-Bus | ⚠️ нужно передавать `DBUS_SESSION_BUS_ADDRESS`; для полноценной интеграции — запускать через `invoker`/booster как обычное Aurora-приложение. |
| Упаковка | ⏳ план в §11 |

---

## 11. План упаковки: `aurora-fyne-runtime` (RPM)

Идея: один пакет, который делает «Fyne-приложение собираемым и запускаемым» без ручной возни.

```
/opt/aurora-fyne/
├── mesa/                      # libEGL, libGLESv2, libglapi, libgbm, libdrm, dri/swrast_dri.so
├── include/                   # EGL/GLES/GL headers + linux/input-event-codes.h
├── pc/                        # libdrm.pc, wayland-egl.pc (заглушки)
├── libs/                      # симлинки lib*.so на системные библиотеки
├── go-glfw/                   # ПАТЧЕННЫЙ модуль github.com/go-gl/glfw/v3.3/glfw
│                              #   (подключается через go.mod replace)
└── go/                        # (опционально) тулчейн Go для armv6l
/usr/bin/fyne-aurora-build     # go build с нужными env/тегами
/usr/bin/fyne-aurora-run       # запуск с корректным окружением
/etc/profile.d/fyne-aurora.sh  # переменные окружения (PATH, FYNE_SCALE и т.п.)
```

Способ подключения патченного GLFW — **без вендоринга в каждом проекте**:

```bash
go mod edit -replace github.com/go-gl/glfw/v3.3/glfw=/opt/aurora-fyne/go-glfw
go mod tidy
```

`fyne-aurora-build` (эскиз):

```sh
#!/bin/sh
set -e
export PATH=/opt/aurora-fyne/go/bin:$PATH
export GOMODCACHE=${GOMODCACHE:-/home/user/.cache/go-mod}
export PKG_CONFIG_PATH=/opt/aurora-fyne/pc:/opt/aurora-fyne/include/../lib/pkgconfig:/usr/lib/pkgconfig
export CGO_CFLAGS="-D_GNU_SOURCE -DEGL_NO_X11 -I/opt/aurora-fyne/include"
export CGO_LDFLAGS="-L/opt/aurora-fyne/libs -lwayland-client -lwayland-cursor -lwayland-egl \
                    -lxkbcommon -lEGL -lGLESv2 -lpthread -lm -ldl"
exec go build -tags "wayland gles egl" "$@"
```

`fyne-aurora-run` (эскиз):

```sh
#!/bin/sh
export XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR:-/run/user/$(id -u)}
export WAYLAND_DISPLAY=${WAYLAND_DISPLAY:-/run/display/wayland-0}
export DBUS_SESSION_BUS_ADDRESS=${DBUS_SESSION_BUS_ADDRESS:-unix:path=$XDG_RUNTIME_DIR/dbus/user_bus_socket}
export LD_LIBRARY_PATH=/opt/aurora-fyne/mesa/usr/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}
export LIBGL_DRIVERS_PATH=/opt/aurora-fyne/mesa/usr/lib/dri
export MESA_LOADER_DRIVER_OVERRIDE=swrast LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe EGL_PLATFORM=wayland
export FYNE_SCALE=${FYNE_SCALE:-2.5}
exec "$@"
```

Пакет надо собирать на устройстве (ARM, `armv7hl`), с `rpmbuild`:

```bash
rpmbuild -bb --define "_topdir /home/user/rpmbuild" \
         --define "__brp_strip_static_archive /bin/true" myapp.spec
```

(В Aurora `rpmbuild` требует FQDN-hostname и обход `brp-strip-static-archive` — busybox-xargs
без `-d`.)

Для установки собственных RPM: подпись developer-сертификатом OMP либо обход плагина
валидации `rpm -i --define '__transaction_validation %{nil}'` (файлы при этом не получают
`security.ima`, но при `ima_appraise=off` это не мешает).

---

## 12. Карта файлов на устройстве (текущее состояние)

```
/home/opt/go                      Go 1.22.12 (arm)
/home/opt/usr/include             EGL/GLES/GL + linux/input-event-codes.h
/home/opt/pc/*.pc                 заглушки pkg-config
/home/opt/libs                    симлинки lib*.so
/home/opt/mesa/usr/lib            Mesa llvmpipe (libEGL, libGLESv2, libgbm, dri/swrast_dri.so)
/home/opt/glfw                    GLFW 3.4 (Wayland-only, статическая сборка) — для C-тестов
/home/user/fyne-demo/             демо-проект: main.go, main.qml, vendor/ (патченный GLFW), demo-wl
/home/user/hello-qml/             Qt/QML Hello World (hello-qml + main.qml)
/home/user/*.tgz                  исходники (go, glfw, headers, mesa) — можно удалить после упаковки
```

Скрипты-обёртки, использованные при отладке: `/tmp/mtkrun.sh`, `/tmp/mtk_bridge7.py`,
`/tmp/build_fyne_ws.sh`, `/tmp/fix_ws.sh`, `/tmp/patch_glfw.py`.

---

## 13. Чек-лист «поднять Fyne с нуля»

1. `ima_appraise=off` в cmdline (§2). Иначе — стоп.
2. `mount -o remount,exec /home` (иначе Go не запускается).
3. Поставить `cmake wayland-devel libxkbcommon-devel gcc-c++` (zypper).
4. Распаковать заголовки mesa-devel + `input-event-codes.h` в `/home/opt/usr/include`.
5. Положить заглушки `libdrm.pc`, `wayland-egl.pc` в `/home/opt/pc`; симлинки в `/home/opt/libs`.
6. `rpm2cpio` → Mesa llvmpipe в `/home/opt/mesa/usr/lib` (+ симлинки).
7. Go armv6l в `/home/opt/go`.
8. `go mod vendor`, применить патчи GLFW (§4), `go clean -cache`.
9. `go build -mod=vendor -tags "wayland gles egl"` с CGO-флагами.
10. Запуск с env из §6.3 и `FYNE_SCALE=2.5`.

Если после шага 10 окно не появляется — смотреть `WAYLAND_DEBUG=1` на наличие `attach`
(§7) и убедиться, что GLFW реально пересобран (§4.5).

---

## 14. Итог для тех, кто будет писать прикладное ПО

* Пишем обычное Fyne-приложение (`app.New()`, `widget`, `canvas`) — без Aurora-специфики.
* Сборка — через `fyne-aurora-build` (после упаковки) или вручную с флагами §6.2.
* Запуск — через `fyne-aurora-run`, масштаб — `FYNE_SCALE` (2.0–3.0).
* Ограничения помнить: нет системного трея, нет X11, (GPU-ускорение ЕСТЬ: Mali-T720 при
  `EGL_PLATFORM=wayland`; llvmpipe — резерв, см. `14-HARDWARE-ACCELERATION.md`),
  курсоры/локали могут ругаться в лог (не фатально).
* GUI-альтернатива, если нужен «родной» вид — Qt/QML (работает штатно через `wl_shell`).

---

*Документ составлен по результатам отладки 19–20.11.2026 на устройстве MVK-2020,
Aurora 4.0.2.340 (Balakovo), kernel 3.18.79. Все параметры (md5 образов, смещения,
версии пакетов) проверены на месте.*
