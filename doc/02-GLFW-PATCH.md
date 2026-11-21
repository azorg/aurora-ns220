# 02. Патч GLFW: fallback на wl_shell

## 2.1 Что патчится

Патчится **не системный GLFW**, а копия внутри Go-модуля `github.com/go-gl/glfw/v3.3/glfw`
(модуль содержит полные исходники GLFW и собирает их cgo).

Файлы (относительно корня модуля):

* `glfw/src/wl_platform.h` — объявления полей;
* `glfw/src/wl_init.c` — биндинг глобала `wl_shell`;
* `glfw/src/wl_window.c` — создание окна, listener, показ, защита xdg-вызовов.

Готовый diff: [`../patches/glfw-wl-shell.patch`](../patches/glfw-wl-shell.patch) (152 изменённые
строки в 3 файлах). Применение:

```bash
# вариант A: на модуль в GOPATH-кэше (сделать копию, кэш read-only)
cp -a $(go env GOMODCACHE)/github.com/go-gl/glfw/v3.3/glfw@* /home/opt/aurora-fyne/go-glfw
chmod -R u+w /home/opt/aurora-fyne/go-glfw
patch -p1 -d /home/opt/aurora-fyne/go-glfw < patches/glfw-wl-shell.patch
# затем в проекте:
go mod edit -replace github.com/go-gl/glfw/v3.3/glfw=/home/opt/aurora-fyne/go-glfw

# вариант B: в vendor/ проекта
go mod vendor
patch -p1 -d vendor/github.com/go-gl/glfw/v3.3/glfw < patches/glfw-wl-shell.patch

# вариант C: скриптом (идемпотентно, правит vendor/)
python3 patches/patch_glfw.py
```

Проверено: на чистом модуле `go-gl/glfw v0.0.0-20221017161538-93cebf72946b` патч
применяется **без конфликтов**.

## 2.2 Содержание патча

### (1) `wl_platform.h`

```c
/* в struct _GLFWplatform: */
    struct wl_shell*            shell;

/* в struct _GLFWwindowWayland: */
    struct wl_shell_surface*    shellSurface;
```

### (2) `wl_init.c`

```c
    else if (strcmp(interface, "wl_shell") == 0)
    {
        _glfw.wl.shell = wl_registry_bind(registry, name, &wl_shell_interface, 1);
    }
```

и ослабление проверки:

```c
-    if (!_glfw.wl.wmBase)
+    if (!_glfw.wl.wmBase && !_glfw.wl.shell)
```

### (3) `wl_window.c`

* новый listener + функция создания `wl_shell`-окна:

```c
static void wlShellSurfaceHandlePing(void* userData, struct wl_shell_surface* s, uint32_t serial)
{ wl_shell_surface_pong(s, serial); }

static void wlShellSurfaceHandleConfigure(void* userData, struct wl_shell_surface* s,
                                          uint32_t edges, int32_t width, int32_t height)
{
    _GLFWwindow* window = (_GLFWwindow*) userData;
    if (width > 0 && height > 0) { /* resize + события */ }

    if (!window->wl.visible)          /* ← КРИТИЧНО (см. 2.3) */
    {
        window->wl.visible = GLFW_TRUE;
        _glfwInputWindowDamage(window);
    }
}

static GLFWbool createWlShellObjects(_GLFWwindow* window)
{
    if (window->wl.shellSurface)      /* защита от повторного создания */
        return GLFW_TRUE;
    window->wl.shellSurface = wl_shell_get_shell_surface(_glfw.wl.shell, window->wl.surface);
    wl_shell_surface_add_listener(window->wl.shellSurface, &wlShellSurfaceListener, window);
    if (window->wl.title) wl_shell_surface_set_title(window->wl.shellSurface, window->wl.title);
    wl_shell_surface_set_toplevel(window->wl.shellSurface);
    wl_surface_commit(window->wl.surface);
    wl_display_roundtrip(_glfw.wl.display);
    return GLFW_TRUE;
}
```

* fallback в `createShellObjects()` (плюс защита от двойного вызова — иначе на одну
  `wl_surface` создаются **две** shell-поверхности = ошибка протокола):

```c
static GLFWbool createShellObjects(_GLFWwindow* window)
{
    if (window->wl.shellSurface || window->wl.xdg.surface) return GLFW_TRUE;
    if (!_glfw.wl.wmBase)
    {
        if (_glfw.wl.shell) return createWlShellObjects(window);
        _glfwInputError(GLFW_PLATFORM_ERROR, "Wayland: Failed to find xdg-shell or wl_shell ...");
        return GLFW_FALSE;
    }
    ...
```

* `destroyShellObjects()` — уничтожать `wl_shell_surface`;
* `_glfwPlatformShowWindow()` — для `wl_shell` нет «configure-before-map»:

```c
    if (window->wl.shellSurface)
    {
        window->wl.visible = GLFW_TRUE;
        wl_surface_commit(window->wl.surface);
        _glfwInputWindowDamage(window);
    }
```

* `_glfwPlatformSetWindowTitle()` — ветка `wl_shell_surface_set_title`;
* **все** вызовы `xdg_toplevel_*` / `zxdg_*` обёрнуты проверками на NULL
  (в `wl_shell`-режиме `window->wl.xdg.toplevel == NULL`, а сгенерированные функции
  libwayland не переносят NULL → segfault).

## 2.3 Симптомы, которые лечит каждая часть

| Правка | Без неё |
|---|---|
| биндинг `wl_shell` + ослабленная проверка | `PlatformError: Wayland: Failed to find xdg-shell in your compositor` |
| `createWlShellObjects` + fallback | окно не создаётся |
| защита от двойного создания | два `get_shell_surface` на одну поверхность → протокольная ошибка, окно не показывается |
| `visible` в `configure`/`ShowWindow` | окно создано, **но не рисуется**: `swapBuffersEGL()` выходит по `if (!window->wl.visible) return;` → нет `attach`, `lipstick` показывает «приложение не отвечает» |
| проверки NULL перед `xdg_*` | segfault при `SetWindowTitle`/`Maximize`/`resize` |

## 2.4 ⚠️ ГЛАВНАЯ ГРАБЛЯ: cgo-кэш не видит правки `.c`

`c_glfw_lin.go` подключает C-файлы через `#include`:

```go
// #include "glfw/src/wl_window.c"
import "C"
```

Go строит ключ кэша по **.go** файлам и не учитывает содержимое включённых `.c`.
После правки `.c` сборка берёт **старые** объектные файлы: патч «применён», поведение не
меняется, размер бинарника тот же.

Лечение:

```bash
export GOCACHE=/home/opt/gocache
go clean -cache && rm -rf $GOCACHE/*
touch vendor/github.com/go-gl/glfw/v3.3/glfw/c_glfw_lin.go
go build -mod=vendor -tags "wayland gles egl" -o app .
```

Проверка, что патч действительно в бинарнике — временный `fprintf(stderr, "TRACE...")`
в ключевых функциях (`createWlShellObjects`, `_glfwPlatformShowWindow`, `swapBuffersEGL`
в `egl_context.c`) и запуск: трассы должны печататься. (В артефактах трассы удалены.)

## 2.5 Минимальный набор проверок после патча

```bash
# 1) wl_shell привязался
WAYLAND_DEBUG=1 ./app 2>&1 | grep -c "get_shell_surface"      # ровно 1

# 2) окно рисуется
WAYLAND_DEBUG=1 ./app 2>&1 | grep -c "wl_surface@.*attach"     # >= 1
WAYLAND_DEBUG=1 ./app 2>&1 | grep -c "pong"                    # если композитор пингует

# 3) нет ошибок EGL
./app 2>&1 | grep -i "EGL:" | grep -v cursor
```
