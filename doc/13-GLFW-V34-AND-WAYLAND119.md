# 13. GLFW v3.4 + libwayland 1.19 (для Fyne 2.8.x) — три подводных камня

Fyne **2.8.2** (её требует `cterm`/ClearTERM) использует
`github.com/go-gl/glfw/v3.4/glfw`, а не v3.3. Это меняет схему патча: наш патч для v3.3
(`patches/glfw-aurora.patch`) на v3.4 **не ложится** (15 из 20 хунков).

Ниже — что именно ломается и как это лечится. Проверено: `cterm` собран и запущен.

## 13.1 Патч под v3.4

Скрипт: [`../patches/patch_glfw34.py`](../patches/patch_glfw34.py)
(применяется к копии модуля, например `/home/opt/aurora-fyne/go-glfw34`).

Отличия от v3.3, которые пришлось учесть:

* функции называются `_glfwShowWindowWayland` / `_glfwSetWindowTitleWayland`
  (а не `_glfwPlatform…`);
* у окна заголовок — `window->title` (а не `window->wl.title`);
* `createShellObjects()` сначала пробует `libdecor`, затем xdg → наш fallback вставляется
  перед `libdecor`-веткой;
* в `wl_init.c` биндинг `wl_shell` вставляется перед веткой `zxdg_decoration_manager_v1`.

Патч добавляет то же, что и для v3.3: `wl_shell`-окно, `visible` в configure/ShowWindow,
защиту от двойного создания, проверки NULL перед `xdg_toplevel_*`, поддержку `wl_touch`
(тач → события мыши).

## 13.2 Ошибка: `WL_MARSHAL_FLAG_DESTROY` не описан

```
glfw/include/wayland-client-protocol.h:1402: ошибка: «WL_MARSHAL_FLAG_DESTROY» не описан
```

**Причина:** GLFW v3.4 тащит Wayland-заголовки, сгенерированные **новым** `wayland-scanner`
(≥1.20), которые используют `wl_proxy_marshal_flags(..., WL_MARSHAL_FLAG_DESTROY)`.
В Aurora `libwayland-client` **1.19**: ни макроса, ни символа `wl_proxy_marshal_flags` нет.
Если просто определить макрос — компилируется, но при запуске падает:
`SIGSEGV: PC=0x0` в `glfwInit()` (GLFW загружает символы через `dlopen` и вызывает NULL).

**Решение (правильное):** перегенерировать все протоколы **сканером 1.19** (он генерирует
совместимый код через `wl_proxy_marshal_constructor_versioned`):

```sh
INC=<module>/glfw/include
WP=/home/opt/src/wayland-protocols-1.34

g() { # base xml
  wayland-scanner client-header "$2" "$INC/$1-client-protocol.h"
  wayland-scanner private-code  "$2" "$INC/$1-client-protocol-code.h"
}

g wayland-client                  /usr/share/wayland/wayland.xml   # см. 13.3 — лучше 1.22!
g xdg-shell                       $WP/stable/xdg-shell/xdg-shell.xml
g xdg-decoration-unstable-v1      $WP/unstable/xdg-decoration/xdg-decoration-unstable-v1.xml
g xdg-activation-v1               $WP/staging/xdg-activation/xdg-activation-v1.xml
g fractional-scale-v1             $WP/staging/fractional-scale/fractional-scale-v1.xml
g idle-inhibit-unstable-v1        $WP/unstable/idle-inhibit/idle-inhibit-unstable-v1.xml
g pointer-constraints-unstable-v1 $WP/unstable/pointer-constraints/pointer-constraints-unstable-v1.xml
g relative-pointer-unstable-v1    $WP/unstable/relative-pointer/relative-pointer-unstable-v1.xml
g viewporter                      $WP/stable/viewporter/viewporter.xml
```

Проверка: `grep -rl wl_proxy_marshal_flags $INC/*.h` → пусто.

## 13.3 Ошибка: `WL_OUTPUT_NAME_SINCE_VERSION` не описан

```
glfw/src/wl_monitor.c:168: ошибка: «WL_OUTPUT_NAME_SINCE_VERSION» не описан
```

**Причина:** событие `wl_output.name` появилось в core-протоколе Wayland 1.21; системный
`/usr/share/wayland/wayland.xml` (1.19) его не содержит.

**Решение:** генерировать core-протокол из **wayland.xml 1.22** (сканером 1.19):

```sh
curl -sSL -o /tmp/wayland-1.22.xml \
  https://gitlab.freedesktop.org/wayland/wayland/-/raw/1.22.0/protocol/wayland.xml
wayland-scanner client-header /tmp/wayland-1.22.xml $INC/wayland-client-protocol.h
wayland-scanner private-code  /tmp/wayland-1.22.xml $INC/wayland-client-protocol-code.h
```

Безопасно: GLFW биндит `wl_output` версией ≤2, сервер просто не пришлёт `name`.

## 13.4 Сборка cterm (итог)

```sh
cd /home/user/clear-ssh
export PATH=/home/opt/go127/bin:$PATH                 # Go 1.27.4 (требование cterm)
export GOPATH=/home/opt/gopath127 GOMODCACHE=$GOPATH/pkg/mod GOCACHE=/home/opt/gocache127
export PKG_CONFIG_PATH=/home/opt/pc:/home/opt/usr/lib/pkgconfig:/usr/lib/pkgconfig
export CGO_CFLAGS="-D_GNU_SOURCE -DEGL_NO_X11 -I/home/opt/usr/include"
export CGO_LDFLAGS="-L/home/opt/libs -lwayland-client -lwayland-cursor -lwayland-egl \
                    -lxkbcommon -lEGL -lGLESv2 -lpthread -lm -ldl"

go mod edit -replace github.com/go-gl/glfw/v3.4/glfw=/home/opt/aurora-fyne/go-glfw34
go build -tags "gui wayland gles egl" -o bin/cterm ./cmd/cterm
```

Результат: `bin/cterm` — 33.8 МБ, ARMv7. Запуск:

```sh
XDG_RUNTIME_DIR=/run/user/100000 WAYLAND_DISPLAY=/run/display/wayland-0 \
LD_LIBRARY_PATH=/home/opt/mesa/usr/lib LIBGL_DRIVERS_PATH=/home/opt/mesa/usr/lib/dri \
MESA_LOADER_DRIVER_OVERRIDE=swrast LIBGL_ALWAYS_SOFTWARE=1 EGL_PLATFORM=wayland \
GALLIUM_DRIVER=llvmpipe FYNE_SCALE=2.5 TERM=xterm-256color ./bin/cterm
```

В логе: `cterm configuration …`, `terminal session started command="" cols=80 rows=25` —
значит PTY-сессия поднялась. Дальше проверяем визуально (окно) и ввод.

## 13.5 Замечание про «переиспользуемость»

Если на другом устройстве Wayland ≥1.20 и/или Fyne ≤2.4 (GLFW v3.3) — патчи отличаются:

| Условие | Что применять |
|---|---|
| Fyne ≤2.4 / glfw v3.3 | `patches/glfw-aurora.patch` (готовый diff) |
| Fyne 2.8 / glfw v3.4 | `patches/patch_glfw34.py` + перегенерация протоколов (13.2, 13.3) |
| Wayland ≥1.20 на устройстве | перегенерация не нужна, достаточно патча |
| Есть `xdg_wm_base` | патч не нужен вовсе (проверить `wlinfo`) |
