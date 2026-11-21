# Журнал экспериментов (Aurora / MVK-2020, 19–21.11.2026)

Хронология с результатами. Формат: что проверяли → что вышло → вывод.

## 21.11.2026

**1. Fyne-демо (apps/hello-ns220): сборка и запуск на устройстве.**
Сборка на планшете: Go 1.27.4 (/home/opt/go127), Fyne 2.8.2, GLFW v3.4 с патчем
(`/home/opt/aurora-fyne/go-glfw34`), теги `wayland gles egl`; модульный кэш root-овый →
сборка через `devel-su` с `GOCACHE=/tmp/gocache-hello`.
Результат: окно 1200×1920 (Resize до размера экрана), Mali через libhybris
(`/proc/self/maps` → `/vendor/lib/egl/libGLES_mali.so`), IMA off виден в HUD.
Замеры при ~70 объектах: **Mali 40–46 fps**, **llvmpipe 16–18 fps** (≈2.6×).
→ *Вывод: полноценный GUI на Go возможен; Fyne-анимацию гнать через
`fyne.NewAnimation`, а не через `fyne.Do` из горутины (B023).*

**2. Тач/жесты без физического экрана.** `/dev/uinput` отсутствует — синтетические
события ядра недоступны. Сделан `-selftest`: вызов тех же обработчиков Fyne
(MouseDown/Dragged/DragEnd/Tapped/DoubleTapped/Scrolled) по сценарию.
Результат: `taps=3 double=1 hold=2 left=1 right=1 up=1 down=1 drag=1 scroll=1`,
HUD и «волны»/след это показывают. Ошибка классификатора (свайпы не считались,
т.к. MouseUp обнулял состояние до DragEnd) найдена и исправлена.
→ *Вывод: логика жестов проверяема удалённо; доставка wl_touch→mouse проверена
ранее (cterm, doc/09).*

**3. Скриншоты.** `busctl --user call … lipstick saveScreenshot` → «Access denied»
(политика D-Bus). Обход: `-shot` в приложении (Fyne `Canvas().Capture()` → PNG),
снимок 1200×1920 ≈ 230 КБ. → *Вывод: для автоматизации использовать свой снимок.*

**4. RPM + подпись.** `rpmbuild -bb` на планшете: нужен `--define
"__brp_strip_static_archive /bin/true"` (busybox-xargs, B027); для подписи —
`zypper in rpm-sign`, ключ gpg2 2.0.4 (без `--pinentry-mode`), `rpm --addsign
--define "_gpg_name …" --define "__gpg /usr/bin/gpg2"` (B026).
Результат: `aurora-hello-ns220-1.0.0-1.armv7hl.rpm` (12 МБ), `rpm -Kv` → OK;
установка `--define '__transaction_validation %{nil}'`; запуск установленного
`/usr/bin/aurora-hello-ns220-launch` работает.
→ *Вывод: цикл «код → RPM → установка» замкнут; доверие OMP для
`ima_appraise=enforce` — отдельная задача (план 0009).*

## 19.11.2026

**1. Доступ к устройству.** USB `0bb4:0a02` (RNDIS+CDC) → хост `192.168.2.14`,
планшет `192.168.2.15`. `devel-su` пароль `1234` → root. SSH по паролю включён нами.
→ *Вывод: работать по SSH удобно; экран нужен только для boot-меню и клавиатуры.*

**2. Обновление ОС.** `ssu re 4.0.2.340` + `zypper dup` (793 пакета) с публичных репозиториев
`omp/omp-apps/hotfixes`. Adaptation-репо (401) недоступен → 32 пакета (droid-hal, libhybris,
ofono-RIL, gstreamer-droid и др.) заблокированы, остались версии 249.
→ *Вывод: userland обновляется публично, ядро/адаптация — только с доступом OMP.*

**3. Проверка исполнения.** Свои скрипты/бинарники не запускаются (`EACCES`), `dlopen` — нет.
В cmdline: `ima_appraise=enforce`, `module.sig_enforce=1`.
`security.ima` у системных бинарников есть (подпись OMP), `.ima` keyring не принимает наш
самоподписанный сертификат («Could not load certificate … to the .ima keyring»).
→ *Вывод: нужен путь либо отключить аппрайз, либо получить ключи OMP.*

**4. Эксперименты с boot-образом.** Первые попытки добавить cmdline в заголовок по `0x38` —
boot-loop. Причина: cmdline находится по `0x40`, а `0x30..0x3F` — `name[16]`; запись по 0x38
сдвигала строку и рушила `bootopt=`.
→ *Вывод: `boot-imaoff-v4.img` (md5 `0ff5bf5b…`) с `ima_appraise=off` грузится, свои бинарники
работают (`UNSIGNED_BINARY_OK`).*

**5. Аварийное восстановление.** Preloader отдаёт CDC-ACM; handshake `A0 0A 50 05`
→ `READY` + `5F F5 AF FA`; hwcode `0x321` (MT6735), target config `0x0` (без SBC/SLA/DAA).
mtkclient по serial не успевает → написан pty-мост. DA stage1/2 грузятся, запись раздела
работает (нужны `--skipwdt`, чанки 4 КБ, ACK до 5 с).
→ *Вывод: устройство восстановимо через preloader/DA даже при «кирпиче».*

**6. Переименование пользователя.** `usermod -l` отказывается (процессы), поэтому:
остановить `user@100000`, `mv` домашнего каталога, правка `/etc/passwd|shadow|group|gshadow`.
uid не менялся → файлы/квоты целы. UI (lipstick) и SSH работают.

## 20.11.2026

**7. Wayland-глобалы lipstick.** `wl_compositor, wl_shm, wl_seat, wl_output,
wl_data_device_manager, wl_shell v1, qt_* , android_wlegl, alien_manager,
lipstick_recorder_manager`. Нет `xdg_wm_base`, нет `text_input`.
→ *Вывод: Fyne требует патча под `wl_shell`; системная клавиатура недоступна.*

**8. Сборка GLFW+Fyne.** Go 1.22.12 (armv6l), GLFW 3.4 Wayland-only, Fyne 2.4.4
`-tags "wayland gles egl"`. Ошибки: нет `linux/input-event-codes.h`, нет `wayland-egl.pc`,
нет `gl.pc`, `GL/glx.h` (лечится тегом `egl`), `-lGL` (симлинк на GLESv2).
→ *Вывод: toolchain собирается, но нужен набор заглушек (см. `03-TOOLCHAIN.md`).*

**9. EGL.** Системный hybris-EGL падает на ассерте `ANativeWindow`. Mesa llvmpipe
(libEGL/libGLESv2/libgbm/libdrm + `swrast_dri.so`) в `/home/opt/mesa` решает проблему.
→ *Вывод: рендер программный; для GUI-производительности нужен отдельный путь (wlegl).*

**10. Окно не рисуется.** `WAYLAND_DEBUG` показал 0 `attach`/`damage`; `swapBuffersEGL()`
выходил из-за `wl.visible == false` (в wl_shell некому выставить). Добавлен `visible`
в `configure` и `ShowWindow` — окно появилось.
→ *Вывод: патч из 3 мест; проверять по `attach` в логе.*

**11. cgo-кэш.** Патчи «не применялись» (бинарник тот же) — кэш не видит правки `.c`,
подключённых через `#include`. `go clean -cache` + `touch c_glfw_lin.go` — и правки заработали.
→ *Вывод: это главная ловушка при патчинге GLFW.*

**12. Тач.** `wl_seat.capabilities(7)` — pointer+keyboard+touch, но GLFW не создавал `wl_touch`,
а lipstick не эмулирует мышь. Написан патч `wl_touch → mouse`.
→ *Вывод: после патча приходят `MOUSE down/up`, `DRAG`, `TAP`.*

**13. Координаты.** Тап в пикселе (385,534) → Fyne (162,5.6) при `FYNE_SCALE=2.5`.
По X масштаб ~2.37 (≈2.5), по Y — нет.
→ *Вывод: нужно согласовать масштаб (TODO).*

**14. Debian chroot.** `debootstrap --foreign` на хосте → rootfs 84 МБ → второй этап на планшете.
`apt` без фиксов не резолвит; помогли `APT::Sandbox::User "root"` и `Acquire::ForceIPv4`.
Установлены `tmux/mc/vim/nmap/ncdu/git`.
→ *Вывод: chroot полностью рабочий, GUI из него не запускать (другой glibc).*

**15. Qt/QML как альтернатива.** `qt5-qtdeclarative-devel` есть, `qmlscene` — битая ссылка,
`QtQuick`-заголовков нет, `qt5-qtgui-devel` конфликтует с adaptation (заголовки извлечены из RPM).
Минимальный C++ раннер (`QGuiApplication` + `QQmlApplicationEngine`) — окно работает через
`wl_shell` штатным QtWayland-клиентом.
→ *Вывод: Qt-путь рабочий там, где Fyne не справляется (например, tray/системные окна).*

**16. Проверка fallback-идей.** Раздел `recovery` как второй boot-образ — не работает
(LK грузит boot с `boot-recovery`). `/dev/fb0` — I/O error (скриншоты недоступны).
→ *Вывод: режимы переключаем скриптом `boot-mode`; для скриншотов — только клиент
`lipstick_recorder_manager` (в планах).*

**17. Сборка cterm под Aurora (21.11).** `cterm` требует Go 1.27.4 и Fyne 2.8.2 →
go-gl/glfw **v3.4**. Поставили Go 1.27.4 на планшет, перенесли исходники (130 МБ, без `android/`).
Патч v3.3 не лёг на v3.4 → написан `patches/patch_glfw34.py`. Далее два блокера:
`WL_MARSHAL_FLAG_DESTROY` (нет в libwayland 1.19) и `WL_OUTPUT_NAME_SINCE_VERSION`
(core-протокол 1.21). Оба решены **перегенерацией протоколов сканером 1.19**
(core — из `wayland.xml` 1.22). Итог: `bin/cterm` (33.8 МБ) собран, запускается,
PTY-сессия стартует (`terminal session started … rows=25`).

## 2026-11-21 — аппаратное ускорение (Mali-T720) подтверждено

* lipstick (pid 1450) держит `/dev/mali0`, `eglplatform_hwcomposer.so`, `libGLES_mali.so` → композитор на GPU.
* Qt-приложения (`booster [browser]`, `booster [silica-qt5]`) — в maps `libEGL.so.1.0.0`, `libGLES_mali.so`, `libwayland-egl` → Qt работает на Mali.
* Пробник на wl_surface с `EGL_PLATFORM=wayland` (системный libhybris): `GL_RENDERER=Mali-T720`, GLES 3.1, `eglSwapBuffers=TRUE`.
* Без `EGL_PLATFORM` → assertion `ANativeWindow.magic` в hybris `egl.c:305` (отсюда прежний вывод «нужен свой ANativeWindow-мост» — ошибочный).
* Замеры (1200x1920, 1200 кадров clear+swap): Mali 13.0 мс/кадр (76.7 fps) vs llvmpipe 37.3 мс/кадр (26.8 fps) → ~2.9x.
* 400x400: Mali 3.3 мс/кадр, llvmpipe 7.8 мс/кадр.
* cterm на Mali запускается и рисует корректно; скрипт `scripts/run-cterm-hw.sh` (CTERM_GPU=auto|hw|sw, fallback на llvmpipe).
* Подводный камень: при `FYNE_SCALE` > 1 клавиатура/строки обрезаются → в лаунчере по умолчанию 1.0.
* Документ: `doc/14-HARDWARE-ACCELERATION.md`.

## 2026-11-21 — сборка и тесты на Linux-хосте (до планшета)

* Хост: Debian 13, Go 1.27.4, gcc, X11/EGL/Wayland dev — сборка `-tags gui` проходит (38 МБ).
* Подводный камень: `/tmp` — tmpfs 7.3 ГБ, был заполнен на 99% → линковка падала
  с «На устройстве не осталось свободного места». Лечение: `TMPDIR=$HOME/tmpbuild`.
* Добавлены юнит-тесты `cmd/cterm/osk_test.go` (без экрана): три состояния
  модификаторов, Ctrl+C/D/Z, Alt-префикс, Shift (буквы и цифры), xterm-последовательности.
* Тесты сразу нашли расхождение с R7: Shift+4 давал `4`, а должен `$`.
  Добавлены `shiftRune()`/`shiftMap` и перемаркировка клавиш под Shift.
* Xvfb-прогон + xdotool: клики q/w/e/Пробел → в PTY пришло `qwe `.
* Артефакт Xvfb: canvas меньше окна (845 vs 1196) → правый край OSK обрезан; на
  Aurora полноэкранный wl_shell — раскладка влезает.
