# 05. Диагностика (troubleshooting)

## 5.1 Таблица симптомов

| Симптом | Причина | Решение |
|---|---|---|
| `PlatformError: Wayland: Failed to find xdg-shell in your compositor` | патч GLFW не применён **или не пересобран** | применить `patches/glfw-wl-shell.patch`, затем `go clean -cache` (см. §5.3) |
| `eglCreateWindowSurface: Assertion ... ANDROID_NATIVE_WINDOW_MAGIC failed` | выбран неверный путь: не выставлен `EGL_PLATFORM=wayland` (hybris) либо подмешан Mesa-`libEGL` | аппаратно: `EGL_PLATFORM=wayland` и **убрать** `/home/opt/mesa/usr/lib` из `LD_LIBRARY_PATH`; программно: `LD_LIBRARY_PATH=/home/opt/mesa/usr/lib LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe` (см. doc/14) |
| непонятно, на чём рисуем (GPU или CPU) | не задан/переопределён `EGL_PLATFORM`, мешанина `LD_LIBRARY_PATH` | `grep -E 'libGLES_mali|llvmpipe' /proc/$(pgrep -f bin/cterm)/maps`; запуск через `scripts/run-cterm-hw.sh` пишет `gpu: Mali-T720 (hardware)` / `gpu: llvmpipe (software)` |
| окно не появляется, процесс жив, в логе тишина | `window->wl.visible` не выставлен → `swapBuffersEGL()` делает ранний `return` | патч `visible` в `configure`/`ShowWindow` (`doc/02` §2.3) |
| lipstick показывает «приложение не отвечает» (ANR) | окно создано, но буферы не отправляются / нет `pong` на ping | то же + проверить `pong` в `WAYLAND_DEBUG` |
| `get_shell_surface` в логе **дважды** на одну `wl_surface` | нет защиты от двойного `createShellObjects` (вызов из create + из `ShowWindow`) | патч содержит guard; проверить, что он в бинарнике (§5.3) |
| сегфолт при `SetWindowTitle`/`Maximize`/`resize` | `xdg_*` вызывается с NULL в wl_shell-режиме | в патче все `xdg_*` обёрнуты проверками |
| текст нечитаемо мелкий | экран 1200×1920, ~264 dpi | `FYNE_SCALE=2.5` + `w.Resize(...)` |
| `Permission denied` при `/home/.../go` или при запуске бинарника | `/home` смонтирован `noexec` | `mount -o remount,exec /home` |
| `Unable to connect to session D-Bus` | нет адреса сессионной шины | `DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/100000/dbus/user_bus_socket` |
| `EGL: Failed to create window surface` | Mesa не нашла `swrast_dri.so` | `LIBGL_DRIVERS_PATH=/home/opt/mesa/usr/lib/dri`, `MESA_LOADER_DRIVER_OVERRIDE=swrast` |
| `libgbm.so.1: cannot open shared object` | не хватает Mesa-компонентов | доставить `mesa-llvmpipe-libgbm`, `libdrm` (§03) |
| Go-модули не скачиваются | нет сети/прокси | проверить `curl https://proxy.golang.org`; прокси на устройстве не нужен |

## 5.2 Быстрые проверки

```bash
# Wayland-глобалы (есть ли wl_shell, нет ли xdg_wm_base)
gcc -o wlinfo patches/wlinfo.c -lwayland-client
XDG_RUNTIME_DIR=/run/user/100000 WAYLAND_DISPLAY=/run/display/wayland-0 ./wlinfo

# Эталон: минимальный wl_shell-клиент (должен показать красный прямоугольник)
gcc -o wltest patches/wltest.c -lwayland-client
XDG_RUNTIME_DIR=/run/user/100000 WAYLAND_DISPLAY=/run/display/wayland-0 ./wltest org.test

# Протокольная активность приложения
WAYLAND_DEBUG=1 ./app 2>&1 | grep -E "get_shell_surface|attach|damage|commit|pong" | head -20

# Что реально загружено (Mesa или hybris)
LD_LIBRARY_PATH=/home/opt/mesa/usr/lib ldd ./app | grep -E "EGL|GLES|gbm|wayland"

# IMA выключен?
cat /proc/cmdline | tr ' ' '\n' | grep -E '^ima_appraise|^module.sig_enforce'
```

## 5.3 Проверка, что патч GLFW реально в бинарнике

Симптом «патч применён, но ничего не изменилось» — почти всегда **cgo-кэш**.
C-файлы подключаются через `#include` и не участвуют в ключе кэша.

```bash
export GOCACHE=/home/opt/gocache
go clean -cache && rm -rf $GOCACHE/*
touch vendor/github.com/go-gl/glfw/v3.3/glfw/c_glfw_lin.go   # или go build -a
go build -mod=vendor -tags "wayland gles egl" -o app .
```

Контрольный признак: добавить `fprintf(stderr, "TRACE ...")` в `createWlShellObjects`,
`_glfwPlatformShowWindow`, `swapBuffersEGL` — после пересборки трассы должны печататься.

## 5.4 Скриншоты (ограничение)

* `/dev/fb0` — **I/O error** (framebuffer держит hwcomposer/hybris).
* `grim`, `scrot`, `wayshot`, `screencapture` в Aurora отсутствуют.
* В протоколе есть `lipstick_recorder_manager` v1 — можно реализовать клиент для
  захвата/записи экрана (см. `07-ROADMAP.md`).
* Пока единственный надёжный способ — визуальная проверка оператором.

## 5.5 Если приложение упало с SIGABRT в cgo

Смотреть первые строки stderr: обычно это EGL-ассерт (libhybris) или отсутствие
`swrast_dri.so`. Проверить `ldd`, переменные `LD_LIBRARY_PATH`, `LIBGL_DRIVERS_PATH`.

## 5.6 Если «залип» планшет (после экспериментов с boot)

См. `08-DEVICE-FACTS.md` §5: preloader/DA-прошивка заводского образа
(`scripts/mtkrun.sh w boot ... --skipwdt`), обязательный сброс питания (Power 40–60 с).
