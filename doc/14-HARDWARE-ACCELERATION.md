# 14. Аппаратное ускорение графики на Aurora (MT6735 / Mali-T720)

Дата: 2026-11-21. Устройство: MVK-2020, Aurora 4.0.2.340, MT6735, ядро 3.18.79.
Вопрос: используют ли Qt-приложения на Aurora аппаратное ускорение; можно ли
подключить его для Go+Fyne (GLFW/EGL); нужна ли своя прослойка-адаптер.

**Короткий ответ:**
1. Qt-приложения на Aurora **уже** работают через Mali (подтверждено на живых процессах).
2. Наш Fyne-стек **тоже может** работать на Mali — без единой строки C-кода: достаточно
   `EGL_PLATFORM=wayland` + системные `libEGL`/`libGLESv2` (libhybris).
3. Своя прослойка-адаптер **не обязательна**; она нужна только для оптимизаций
   (damage/partial update, управление числом буферов, damage-трекинг, kiosk-режим
   прямо через hwcomposer). См. §6 — что реально стоит написать.

---

## 1. Что есть в системе (факты)

### 1.1. Загрузка GPU есть, драйвер живой
```
/dev/mali0                      crw-rw-rw- 10,59
/sys/class/misc/mali0/device/   gpuinfo, dvfs_period, js_timeouts, mem_pool_size…
/vendor/lib/egl/libGLES_mali.so exists
```

### 1.2. Стек libhybris (мост «glibc → Android HAL»)
```
/usr/lib/libEGL.so.1.0.0         18 КБ  ← это НЕ Mesa: диспетчер libhybris (dlsym-шим)
/usr/lib/libGLESv2.so.2.0.0      71 КБ  ← тоже шим
/usr/lib/libhybris-eglplatformcommon.so.1
/usr/lib/libhybris/eglplatform_wayland.so      ← клиентская+серверная Wayland-платформа
/usr/lib/libhybris/eglplatform_hwcomposer.so   ← прямой рендер в hwcomposer (без композитора)
/usr/lib/libhybris/eglplatform_fbdev.so        ← рендер во framebuffer
/usr/lib/libhybris/eglplatform_null.so
/usr/lib/libhybris/linker                       ← Android-линкер (грузятся *.so из /system,/vendor)
```
`eglplatform_wayland.so` экспортирует `WaylandNativeWindow` (ANativeWindow поверх wl_surface)
и `ServerWaylandBuffer` (импорт клиентских буферов) — то есть libhybris уже реализует
ровно ту «прослойку», о которой обычно спрашивают:
`wl_egl_window → ANativeWindow → gralloc-буфер → android_wlegl-протокол → wl_surface`.

### 1.3. Что использует lipstick (композитор)
```
pid 1450 /usr/bin/lipstick … 
  /dev/mali0
  /usr/lib/libEGL.so.1.0.0
  /usr/lib/libGLESv2.so.2.0.0
  /usr/lib/libhybris-eglplatformcommon.so.1
  /usr/lib/libhybris/eglplatform_hwcomposer.so      ← композитинг через hwcomposer
  /usr/lib/qt5/plugins/wayland-graphics-integration-server/libwayland-egl.so
  /usr/libexec/droid-hybris/system/lib/android.hardware.graphics.*@2.x.so
```
Вывод: **lipstick композитит на GPU** (hwcomposer + Mali), а не в shm/llvmpipe.
Клиентские буферы принимаются через `android_wlegl` (глобал v2 в реестре Wayland).

### 1.4. Qt-приложения — на Mali
Проверено по `/proc/<pid>/maps` живых Qt-процессов:
```
booster [browser]      pid 2118/2119/2178: libEGL.so.1.0.0, libGLES_mali.so, libGLESv2.so.2, libwayland-egl
booster [silica-qt5]   pid 26913:          libEGL.so.1.0.0, libGLES_mali.so, libGLESv2.so.2, libwayland-egl
lipstick               pid 1450:           libEGL.so.1.0.0, libGLES_mali.so, libGLESv2.so.2, libwayland-egl
```
Обратите внимание: `libwayland-egl` — тот самый клиентский путь (wl_egl_window → hybris),
никакого отдельного Qt-адаптера не требуется. Часть давно запущенных процессов
(`booster-qt5`, `sailfish-homescreen-services`) держит только `libGLESv2.so.2`
без `libGLES_mali` — это процессы, которые EGL-контекст ещё не создавали;
реально рисующие (`browser`, `silica-qt5`, `lipstick`) грузят Mali.

### 1.5. Итог по вопросу «используют ли Qt аппаратное ускорение»
**Да.** И композитор, и Qt-приложения идут через libhybris EGL → Mali-T720.
Софтверный путь (Mesa llvmpipe) в системе отсутствует — `/usr/lib/dri` пуст,
llvmpipe у нас положен отдельно в `/home/opt/mesa` (наш обходной путь).

---

## 2. Проверка: работает ли Mali для нашего клиента (замеры)

Пробник `/tmp/glprobe.c` (C, ~90 строк): `wl_display_connect` → `wl_compositor`/`wl_shell` →
`wl_egl_window_create` → `eglGetDisplay(wl_display)` → `eglCreateWindowSurface` →
`eglMakeCurrent` → `glGetString`. Те же действия делает GLFW в Fyne.

### 2.1. Результаты при `EGL_PLATFORM=wayland` (системные libhybris-библиотеки)
```
EGL 1.4  VENDOR=Android  VERSION="1.4 Android META-EGL"  CLIENT_APIS=OpenGL_ES
EXTENSIONS=… EGL_KHR_partial_update EGL_ANDROID_front_buffer_auto_refresh
             EGL_ANDROID_get_native_client_buffer EGL_HYBRIS_native_buffer2
             EGL_WL_create_wayland_buffer_from_image EGL_WL_bind_wayland_display …
GL_RENDERER = Mali-T720
GL_VENDOR   = ARM
GL_VERSION  = OpenGL ES 3.1 v1.r18p0-00cet0.78299f586f7919862c85cb764ae43fad
GLSL        = OpenGL ES GLSL ES 3.10
eglSwapBuffers = EGL_TRUE
```
То есть **кадр реально уходит в lipstick и композитится на GPU**.

### 2.2. Сравнение производительности (наш пробник, 1200×1920, 1200 кадров clear+swap)
| путь | ms/кадр | fps | примечание |
|---|---|---|---|
| **Mali-T720 (hybris)** | **13.0** | **76.7** | упирается в обмен буферами (3 буфера, композитор) |
| llvmpipe (Mesa, swrast) | 37.3 | 26.8 | чистый CPU-рендер 1280×1920 |

При 400×400: Mali ~3.3 мс/кадр (≈300 fps) против ~7.8 мс/кадр у llvmpipe.
**Вывод: на полном экране GPU даёт ≈2.9× по кадровому времени и снимает нагрузку с CPU.**
Для терминала (перерисовка по вводу/выводу) это означает мгновенную отрисовку скролла
и отсутствие «тормозов» при большом объёме вывода.

### 2.3. Живая проверка на cterm
```
env EGL_PLATFORM=wayland ./bin/cterm --osk …   → PID живёт, /proc/PID/maps:
  /dev/mali0, /vendor/lib/egl/libGLES_mali.so, /usr/lib/libGLESv2.so.2.0.0
Скриншот: UI отрисован корректно (клавиатура, терминал).
```
Замечание: внешний вид немного отличается от llvmpipe (скругления/заливки клавиш
рисуются иначе) — это разница растеризации GL-примитивов, не баг.

---

## 3. Почему раньше «не работало» и что было не так

Наш стенд собирался под llvmpipe, потому что:
* в системе **нет** Mesa (`/usr/lib/dri` пуст) — значит `libEGL.so.1` не Mesa;
* Fyne/GLFW при старте делали `eglGetDisplay(wl_display)` **без** `EGL_PLATFORM`;
  libhybris в этом случае выбирает платформу по `HYBRIS_EGLPLATFORM`, и при
  неверном выборе/неподходящем native window падает с
  `Assertion '((struct ANativeWindow *) win)->common.magic == ANDROID_NATIVE_WINDOW_MAGIC'`;
* мы интерпретировали это как «hybris требует ANativeWindow/android_wlegl, нужен свой мост»
  и ушли на llvmpipe.

Фактически достаточно было выставить `EGL_PLATFORM=wayland` — libhybris сам
заворачивает `wl_egl_window` в `WaylandNativeWindow` и аллоцирует gralloc-буферы
через `android_wlegl`, которые lipstick принимает.

---

## 4. Как включить аппаратный путь (сейчас, без кода)

```sh
# на устройстве, запуск cterm на GPU:
unset LD_LIBRARY_PATH LIBGL_DRIVERS_PATH MESA_LOADER_DRIVER_OVERRIDE GALLIUM_DRIVER
export EGL_PLATFORM=wayland                 # ← ключ: платформа libhybris
export XDG_RUNTIME_DIR=/run/user/100000
export WAYLAND_DISPLAY=/run/display/wayland-0
./bin/cterm --osk
```
Проверка, что пошли через GPU:
```sh
grep -E 'mali|libGLESv2' /proc/$(pgrep -f bin/cterm)/maps
```
Скрипт: `scripts/run-cterm-hw.sh` (сделан), с автоматическим откатом на llvmpipe,
если Mali недоступен.

Важно: **не** ставить `/home/opt/mesa/usr/lib` в `LD_LIBRARY_PATH` — иначе
подхватится Mesa-`libEGL.so.1` и Mali не будет.

---

## 5. Нужна ли своя прослойка-адаптер?

Разложим по слоям, что уже есть и чего нет.

| Слой | Кто реализует | Нужно ли писать нам |
|---|---|---|
| Wayland-клиент ↔ EGL | libhybris `eglplatform_wayland.so` (`WaylandNativeWindow`) | **нет** |
| Аллокация буферов | gralloc + `android_wlegl` (клиент/сервер) | **нет** |
| Композитинг | lipstick + hwcomposer + Mali | **нет** |
| Синхронизация (fences) | `libsync`, `EGL_KHR_fence_sync` | нет (пока) |
| Выбор GPU/CPU-пути, откат | **никто** — каждый апп сам | **да, тонкий Go-адаптер** |
| Partial update / damage | Fyne/GLFW не используют `EGL_KHR_partial_update` | опционально, C-шим |
| Управление числом буферов | `WaylandNativeWindow::setBufferCount` не вызывается | опционально, C-шим |
| Kiosk-режим без композитора | `EGL_PLATFORM=hwcomposer` | опционально, отдельный лаунчер |
| Zero-copy видео/камера | `EGL_HYBRIS_native_buffer2` | когда дойдём до мультимедиа |

### 5.1. Рекомендуемая «прослойка» (то, что реально стоит написать)
Go-пакет `pkg/aurora/gpu` (≈150 строк, без cgo):
1. **Detect()** — определить окружение:
   * есть ли `/dev/mali0` и `libhybris/eglplatform_wayland.so`;
   * какая `GL_RENDERER` получилась (после старта GL — через Fyne `canvas`/glfw или
     одноразовый пробник);
   * вернуть `Mode{GPU: true, Renderer: "Mali-T720"}`.
2. **Env()** — выставить `EGL_PLATFORM=wayland`, убрать Mesa-переменные;
   предусмотреть `CTERM_GPU=0|1|auto` (по умолчанию `auto`).
3. **Fallback()** — если EGL-init упал, повторить запуск в режиме llvmpipe
   (`LD_LIBRARY_PATH=/home/opt/mesa/usr/lib`, `LIBGL_ALWAYS_SOFTWARE=1`).
4. **Диагностика в лог**: `gpu: Mali-T720 (hardware)` / `gpu: llvmpipe (software)` —
   чтобы на устройстве было видно, каким путём пошли.

Это «адаптер» уровня приложения: он не переписывает EGL, а выбирает путь и
обеспечивает предсказуемость на разных прошивках (у части устройств Mali может
быть закрыт/сломан — тогда автоматический откат).

### 5.2. Что можно добавить в C-шим позже (если понадобится скорость)
* `eglSwapInterval` + `WaylandNativeWindow::setBufferCount(2)` — меньше латентность;
* `EGL_KHR_partial_update` (`eglSetDamageRegionKHR`) — перерисовка только
  изменившихся строк терминала: на терминале это большой выигрыш (меньше
  композитинга при вводе);
* `EGL_ANDROID_front_buffer_auto_refresh` — для 60 Гц «гладкого» курсора;
* для киоска — `EGL_PLATFORM=hwcomposer` (прямой scanout, без lipstick).

---

## 6. Риски и ограничения

1. **Закрытые блобы.** Mali-драйвер и gralloc — вендорные (`/vendor/lib`),
   обновление прошивки может поменять поведение. Отсюда — обязательный fallback.
2. **Термотроттлинг.** DVFS Mali (`/sys/class/misc/mali0/device/dvfs_period`)
   на MT6735 агрессивный; длительная нагрузка → снижение частоты. Для терминала
   нагрузка кратковременная, это не проблема.
3. **Буферная очередь.** При `swapInterval=0` без ограничения кадров
   возможны «залипания» на `queueBuffer` (мы это видели: 300 кадров подряд дали
   68 fps, а 120 — 300 fps). В приложении нужен нормальный vsync-режим
   (Fyne сам рисует по событиям — ок).
4. **Разница рендеринга.** Мелкие визуальные отличия (скругления, альфа)
   между Mali и llvmpipe — норма; эталоны скриншотов держать по одному пути.
5. **Не все EGL-конфиги.** GLES 3.1 и GLES2-контекст доступны; десктопный
   GL (OpenGL 2.1+) — нет. Fyne на теге `gles` это учитывает.
6. **hwcomposer-платформа** (kiosk) требует прав/отсутствия конкурента за
   дисплей: пробник в этом режиме из ssh-сессии не отработал (нужен запуск
   как сервис/из сессии lipstick). Отложено.

---

## 7. Выводы и план

**Выводы.**
* Аппаратное ускорение на Aurora есть и работает: lipstick, Qt-приложения — на Mali-T720.
* Наш Fyne/GLFW-стек подключается к нему **одной переменной** `EGL_PLATFORM=wayland`.
* Собственный «мост» ANativeWindow/android_wlegl писать не нужно — он уже в libhybris.
* Полноэкранно GPU даёт ~2.9× меньше времени на кадр и разгружает CPU.
* Своя прослойка оправдана как **селектор режима + fallback + диагностика**,
  и позже — как C-шим для partial update/буферов/kiosk-scanout.

**План.**
1. [x] Пробник: подтвердить Mali на wl_surface (сделано, Mali-T720/GLES 3.1).
2. [x] Замеры hw vs llvmpipe (13.0 vs 37.3 мс/кадр на 1200×1920).
3. [x] Запуск cterm на Mali (живой процесс + скриншот).
4. [ ] `pkg/aurora/gpu`: detect/env/fallback + лог (`CTERM_GPU=auto|hw|sw`).
5. [ ] `scripts/run-cterm-hw.sh` в стенде + `run.sh --gpu` (частично сделано).
6. [ ] Перевести `build.sh`/`run.sh`/systemd-юнит на авто-выбор GPU.
7. [ ] Проверить стабильность 30–60 мин под нагрузкой (вывод `yes`, скролл, ввод).
8. [ ] (позже) C-шим: partial update + buffer count + опция kiosk/hwcomposer.
9. [ ] Обновить `04-BUILD-RUN.md`, `06-PACKAGING.md` (зависимости: libhybris уже в системе).
