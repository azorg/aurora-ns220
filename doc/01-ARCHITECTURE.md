# 01. Архитектура решения и почему именно так

## 1.1 Стек

```
Go/Fyne  →  go-gl/glfw (CGO, GLFW 3.3.x из Go-модуля)
         →  EGL + GLES2 (libhybris → Mali-T720, аппаратный;
            Mesa llvmpipe — резервный программный путь)
         →  wl_surface (wl_shm/EGL-буферы) + wl_shell_surface (окно)
         →  lipstick (QtWaylandCompositor) — /run/display/wayland-0
```

## 1.2 Что отдаёт композитор Aurora

Проверено клиентом `patches/wlinfo.c`:

```
wl_compositor v3, wl_shm v1, wl_seat v3, wl_output v2, wl_data_device_manager v1,
wl_shell v1,
qt_surface_extension v1, qt_windowmanager v1, qt_touch_extension v1,
qt_hardware_integration v1, android_wlegl v2, alien_manager v1,
lipstick_recorder_manager v1
```

**Нет**: `xdg_wm_base`, `zxdg_shell_v6`, `wlr-screencopy`, `wp_viewporter` (на клиенте он есть,
но композитор не объявляет).

Отсюда три следствия:

1. Любой клиент на xdg-shell (GLFW 3.3+, GTK4, Qt6, многие тулкиты) работать не будет.
2. Единственный «стандартный» путь окна — `wl_shell` v1 (deprecated с 2013 г., но живой).
3. QtWayland-клиент умеет `wl_shell` → **Qt/QML работает без правок** (проверено).

## 1.3 Почему не удалось обойтись без патча GLFW

Рассмотренные варианты:

| Вариант | Вердикт |
|---|---|
| **Патч GLFW: fallback на `wl_shell`** | ✅ выбрано. ~150 строк правок, изолировано в одном модуле |
| Свой мини-композитор (wl_shell снаружи, xdg-shell внутрь) | ⚠️ рабочий, но это отдельный продукт: compositing, input-роутинг, реализация xdg-shell сервера. Отложено |
| wlroots/weston вложенно | ❌ не запускается: им нужен xdg-shell от родителя |
| Использовать Qt (QML) вместо Fyne | ✅ работает, но цель — именно Fyne-приложения |
| Ждать xdg-shell в Aurora | ❌ не в нашей власти |

## 1.4 EGL: сначала аппаратный путь (libhybris/Mali), llvmpipe — резерв

**Обновлено 2026-11-21 (см. `14-HARDWARE-ACCELERATION.md`; прежний вывод ниже исправлен).**

Системный `libEGL.so.1` — это **libhybris**, мост к вендорскому драйверу Mali-T720
(`/vendor/lib/egl/libGLES_mali.so`, `/dev/mali0`). Он полностью работоспособен для
наших приложений — при условии, что выбрана правильная платформа:

```sh
export EGL_PLATFORM=wayland      # ← без этого hybris падает на ассерте
```

С `EGL_PLATFORM=wayland` libhybris сам заворачивает `wl_egl_window` в свой
`WaylandNativeWindow` (ANativeWindow), аллоцирует gralloc-буферы через протокол
`android_wlegl`, а lipstick композитит их на GPU:

```
GL_RENDERER = Mali-T720   GL_VERSION = OpenGL ES 3.1   eglSwapBuffers = EGL_TRUE
```

Без этой переменной hybris выбирает неподходящую платформу и падает:

```
eglCreateWindowSurface: Assertion `((struct ANativeWindow *) win)->common.magic ==
ANDROID_NATIVE_WINDOW_MAGIC' failed.
```

Именно этот ассерт раньше был истолкован как «нужен свой мост `android_wlegl` →
`ANativeWindow`». Это оказалось неверно: мост уже реализован в libhybris
(`eglplatform_wayland.so`), писать клиент протокола не нужно.

Замеры (1200×1920, clear+swap): Mali ≈ 13.0 мс/кадр (76.7 fps) против llvmpipe
≈ 37.3 мс/кадр (26.8 fps) — примерно **2.9×** в пользу GPU.

Поэтому:
* **основной режим** — `EGL_PLATFORM=wayland` + системные libhybris-библиотеки;
* **резервный режим** — Mesa llvmpipe из `/home/opt/mesa` через `LD_LIBRARY_PATH`
  (если Mali недоступен/сломан прошивкой);
* выбор режима и откат — `scripts/run-cterm-hw.sh` (`CTERM_GPU=auto|hw|sw`).

Важно: **не** ставить `/home/opt/mesa/usr/lib` в `LD_LIBRARY_PATH` в аппаратном
режиме — иначе подхватится Mesa-`libEGL.so.1` и Mali не будет.

## 1.5 Ключевая последовательность событий окна

1. `wl_compositor.create_surface` — поверхность.
2. `wl_shell.get_shell_surface(surface)` — роль окна.
3. `wl_shell_surface.set_title`, `set_toplevel`.
4. `wl_surface.commit` (без буфера — окно ещё «пустое»).
5. EGL создаёт `wl_egl_window` + `eglCreateWindowSurface` (libhybris/Mali при
   `EGL_PLATFORM=wayland`, иначе Mesa llvmpipe).
6. Первый `eglSwapBuffers` → `wl_surface.attach` + `damage` + `commit` → окно появляется.
7. `wl_shell_surface.ping` → обязателен `pong` (иначе ANR «приложение не отвечает»).

Важно: в GLFW xdg-пути `visible` выставляется в обработчике `configure`, а `swapBuffersEGL`
делает `if (!window->wl.visible) return;`. В `wl_shell` configure приходит иначе/позже —
поэтому в патче `visible` ставится и в `configure`-обработчике, и в `ShowWindow`
(подробно в `02-GLFW-PATCH.md`). Без этого окно «висит» и не рисует (симптом: нет
`attach`, ANR-диалог).

## 1.6 Ограничения архитектуры

* нет системного трея → нужен свой (см. `07-ROADMAP.md`);
* нет CSD/декораций: `wl_shell` в lipstick — окно на весь экран без заголовка;
* нет `xdg-output`/`viewporter`: масштабирование буфера — на стороне приложения (`FYNE_SCALE`);
* GPU есть и используется: Mali-T720 через libhybris при `EGL_PLATFORM=wayland`
  (llvmpipe — только резервный программный путь);
* `lipstick` может показывать ANR-диалог, если клиент не отвечает на ping (см. п. 1.5).
