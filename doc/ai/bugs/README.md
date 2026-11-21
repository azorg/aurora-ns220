# Известные проблемы и их решения (Aurora / Go+Fyne / MTK)

Формат: **симптом → причина → решение**. Всё ниже реально встречалось и проверено.

## B001. Патч GLFW применён, но поведение старое
**Симптом:** правки `.c` в Go-модуле не влияют на бинарник (тот же размер, те же ошибки).
**Причина:** cgo-кэш не учитывает содержимое файлов, подключённых через `#include`.
**Решение:** `go clean -cache && rm -rf $GOCACHE/* && touch vendor/.../c_glfw_lin.go`, затем сборка.

## B002. Планшет не грузится после правки cmdline boot-образа
**Причина:** cmdline писали по смещению `0x38` вместо `0x40` → поле `name[16]` затёрто,
строка сдвинута на 8 байт, потерян `bootopt=`.
**Решение:** писать строго в `0x40..0x23F`. Восстановление: заводской образ через
`fastboot flash boot` или preloader/DA (`scripts/mtkrun.sh`).

## B003. `Failed to find xdg-shell in your compositor`
**Причина:** lipstick отдаёт только `wl_shell` v1 (нет `xdg_wm_base`/`zxdg_shell_v6`).
**Решение:** патч GLFW `patches/glfw-aurora.patch` (fallback на `wl_shell`).

## B004. `eglCreateWindowSurface: Assertion ANativeWindow ... MAGIC failed`
**Причина:** системный EGL — libhybris, требует `ANativeWindow` (android_wlegl), а GLFW
передаёт `wl_egl_window`.
**Решение:** Mesa llvmpipe через `LD_LIBRARY_PATH=/home/opt/mesa/usr/lib` +
`EGL_PLATFORM=wayland`, `GALLIUM_DRIVER=llvmpipe`, `MESA_LOADER_DRIVER_OVERRIDE=swrast`.

## B005. Окно создано, но не рисуется / «приложение не отвечает»
**Причина:** `swapBuffersEGL()` выходит по `if (!window->wl.visible) return;`, а в
`wl_shell`-пути `visible` не выставлялся (в xdg это делает `configure`).
**Решение:** патч: ставить `visible` в `configure`-обработчике и в `ShowWindow`.

## B006. На одну `wl_surface` создаются две shell-поверхности
**Причина:** `createShellObjects()` вызывается и при создании окна, и в `ShowWindow`.
**Решение:** guard `if (window->wl.shellSurface || window->wl.xdg.surface) return GLFW_TRUE;`.

## B007. `Permission denied` при запуске Go/бинарников из `/home`
**Причина:** `/home` смонтирован `noexec` (Aurora по умолчанию).
**Решение:** `mount -o remount,exec /home` (скрипты `build-fyne.sh`/`run-fyne.sh`/`debian-chroot`
делают это сами).

## B008. В Debian chroot `apt` не резолвит DNS
**Симптом:** `Could not resolve 'deb.debian.org'` / `Temporary failure resolving`.
**Причина:** 1) `_apt`-пользователь в chroot не получает ответ от connman-резолвера;
2) у планшета нет IPv6, а `deb.debian.org` отдаёт AAAA.
**Решение:** `APT::Sandbox::User "root";` + `Acquire::ForceIPv4 "true";` +
`nameserver 8.8.8.8` внутри chroot.

## B009. Режим «recovery» в boot-меню LK не грузит наш образ
**Причина:** пункт «Recovery Mode» грузит **boot-раздел** с флагом `boot-recovery`,
а не раздел `recovery`. Наша система в этом режиме висит на логотипе.
**Решение:** не использовать; переключать режимы скриптом `boot-mode` (образы в `/boot-modes`).

## B010. Нельзя снять скриншот
**Причина:** `/dev/fb0` → I/O error (framebuffer занят hwcomposer);
`grim/scrot/wayshot/screencapture` отсутствуют.
**Решение:** пока только визуальная проверка; в планах — клиент `lipstick_recorder_manager`.

## B011. Тапы не доходят до приложения
**Причина:** `wl_seat` отдаёт touch, но lipstick **не эмулирует мышь**, а GLFW 3.3
не поддерживает `wl_touch` вообще.
**Решение:** патч `wl_touch → mouse` (см. `doc/09-TOUCH-AND-KEYBOARD.md`).

## B012. Координаты тапа не совпадают с тем, что видит Fyne
**Симптом:** тап в пикселе (385,534) приходит в Fyne как (162,5.6) при `FYNE_SCALE=2.5`.
**Причина:** не согласован масштаб между координатами поверхности и логическими точками Fyne.
**Решение (TODO):** делить координаты на scale в touch-обработчике либо задать content scale
через GLFW; проверить с `FYNE_SCALE=1`.

## B013. Нет системной экранной клавиатуры для Fyne
**Причина:** в глобалах lipstick нет `zwp_text_input_manager*`/`input_method`.
**Решение:** собственная экранная клавиатура на canvas (EN/RU + модификаторы).

## B014. preloader зависает и не сбрасывается
**Причина:** отправлен `0x70 SEND_PARTITION_DATA` и не дослан пакет; watchdog выключен.
**Решение:** только сброс питания (отключить USB, Power 40–60 с). Не обрывать пакеты.

## B015. mtkclient не видит устройство / «Sync error»
**Причина:** окно preloader ~2.5 с; CDC-ACM; mtkclient не успевает и гасит watchdog.
**Решение:** pty-мост (`scripts/mtk_bridge7.py`) + `mtkrun.sh` + `--skipwdt`;
запись DA чанками 4 КБ, ACK до 5 с.

## B016. `usermod -l` не переименовывает пользователя
**Симптом:** `user defaultuser is currently used by process N`.
**Решение:** остановить сессию (`systemctl stop user@100000.service`), затем аккуратно
править `/etc/passwd|shadow|group|gshadow|subuid|subgid` (uid не меняется), `mv` домашнего
каталога. Бэкап обязателен.

## B017. Свой developer-сертификат не принимается `.ima` keyring
**Симптом:** `Could not load certificate (….) to the .ima keyring`; файлы с нашей подписью
не запускаются при `ima_appraise=enforce`.
**Причина:** ядро разрешает добавлять в `.ima` только ключи, подписанные доверенным (OMP).
**Решение (TODO):** проверить hash-аппрайз (`security.ima` = ima-ng хеш) при выключенном EVM;
иначе — developer-сертификат OMP / `sdk-deploy-rpm`.

## B018. `rpm -i` своего пакета: «does not have developer signature»
**Решение:** `rpm -i --define '__transaction_validation %{nil}'` (работает при `ima_appraise=off`)
либо подпись `rpmsign-external` с CA, добавленным в `/etc/keys/packages/ca.pem`.

## B019. Каталог `~/aurora` исчезал в окружении агента
**Симптом:** «Working directory does not exist».
**Решение:** пересоздать каталог (или сделать симлинк на рабочий), продолжить.

## B020. `SIGSEGV PC=0x0` в `glfwInit()` (Fyne 2.8 / GLFW v3.4)
**Причина:** GLFW v3.4 использует `wl_proxy_marshal_flags` (libwayland ≥1.20),
в Aurora libwayland 1.19 — символа нет, GLFW вызывает NULL.
**Решение:** перегенерировать Wayland-протоколы в модуле сканером 1.19
(см. `doc/13-GLFW-V34-AND-WAYLAND119.md` §13.2).

## B021. `WL_OUTPUT_NAME_SINCE_VERSION` не описан
**Причина:** событие `wl_output.name` появилось в core-протоколе 1.21, а системный
`wayland.xml` — 1.19.
**Решение:** генерировать core-протокол из `wayland.xml` 1.22 (`doc/13` §13.3).

## B022. Патч GLFW v3.3 не применяется к v3.4
**Симптом:** 15 из 20 хунков FAILED.
**Причина:** Fyne 2.8 использует `go-gl/glfw/v3.4`; другие имена функций, `window->title`,
libdecor-ветка.
**Решение:** `patches/patch_glfw34.py` (см. `doc/13` §13.1).

## B023. `fyne.Do` из своего таймера «залипает»: скриншот/диалог срабатывает через 10–20 с
**Симптом:** анимация обновляется из горутины через `fyne.Do` каждый кадр — всё
работает, но любое другое отложенное действие (`w.Canvas().Capture()`, `a.Quit()`,
диалог) выполняется с задержкой в десятки секунд.
**Причина:** `fyne.Do` складывает вызов в очередь, которую разбирает цикл Fyne;
60 вызовов/с от анимации вытесняют остальные.
**Решение:** анимацию запускать штатным механизмом:
`fyne.NewAnimation(time.Second/fps, tick)` + `RepeatCount = fyne.AnimationRepeatForever`
(тик идёт в потоке отрисовки, `fyne.Do` не нужен). `fyne.Do` — только для
редких событий (снимок, выход, диалог). Горутины для «снять снимок через N сек»
работают предсказуемо.

## B024. `SetFullScreen(true)` на wl_shell даёт чёрный кадр
**Симптом:** окно пропадает/чёрное; при этом Resize до размера экрана рисует нормально.
**Причина:** lipstick реализует `wl_shell_surface` v1, fullscreen-путь GLFW/Fyne
на нём не работает.
**Решение:** не вызывать `SetFullScreen`; брать размер экрана через GLFW
(`glfw.GetPrimaryMonitor().GetVideoMode()`) и делать `w.Resize(size)` после
`Show` (см. `apps/hello-ns220/screen.go`).

## B025. Окно 320×240 (или вообще не создаётся) — «Failed to create XKB compose table»
**Симптом:** в логе `PlatformError: Wayland: Failed to create XKB compose table`,
поверхность остаётся минимальной 320×240, EGL-контекст не создаётся.
**Причина:** при `LANG=C`/POSIX xkbcommon не находит Compose-файл; в Aurora нет
`/usr/share/X11/locale`.
**Решение:** `LANG=ru_RU.UTF-8` (или en_US.UTF-8) и `XCOMPOSEFILE` на существующий
Compose (см. `packaging/aurora-hello-ns220-launch.sh`). Проверка: без неё окно
не поднимается, с ней — 1200×1920.

## B026. `rpm --addsign`: «/usr/bin/rpmsign: No such file or directory»
**Симптом:** подпись RPM не выполняется, хотя `rpm`/`rpmbuild` есть.
**Причина:** пакет `rpm-sign` (с `/usr/bin/rpmsign`) не установлен по умолчанию.
**Решение:** `zypper in rpm-sign`; ключ — gpg2 (в Aurora 2.0.4, **без**
`--pinentry-mode`, `%no-protection` игнорируется — ключ и так без пароля):
`gpg2 --batch --gen-key params` и `rpm --addsign --define "_gpg_name <uid>" --define "__gpg /usr/bin/gpg2"`.
Проверка: `rpm --import pub.asc && rpm -Kv pkg.rpm` → `Header V3 RSA/SHA1 Signature ... OK`.

## B027. `rpmbuild` падает на `brp-strip-static-archive` (busybox xargs)
**Симптом:** `error: Bad exit status ... (%install)`, в логе usage от `xargs`.
**Причина:** busybox-`xargs` не поддерживает `-d`, а brp-скрипт его передаёт.
**Решение:** при сборке: `--define "__brp_strip_static_archive /bin/true"`
(и `--define "debug_package %{nil}"`), см. `apps/hello-ns220/packaging/build-rpm.sh`.

## B028. GPU в приложении определяется как «EGL (драйвер не определён)»
**Причина:** проверка `/proc/self/maps` на `libGLES_mali` выполняется до создания
GL-контекста (в `app.New`/`newBoard`), библиотеки ещё не загружены.
**Решение:** определять GPU лениво, при обновлении HUD (после старта GL) —
`gpuInUse()` в каждом кадре/статистике, см. `apps/hello-ns220/devinfo.go`.

## B029. Приложение из меню Aurora: «крутится кружок» и закрывается; при первом запуске — вопрос о разрешениях
**Симптом:** `.desktop` установлен, иконка в меню есть; при запуске спиннер и окно не
появляется; на первом запуске система спрашивает разрешения на периферию (аудио,
камера, …). Из shell (`env -i … aurora-hello-ns220-launch`) то же приложение работает.
**Причина:** в Aurora приложение без секции `[X-Sailjail]` запускается в песочнице
по «Default Profile» (`Audio;Bluetooth;Camera;Compatibility;Internet;Location;
MediaIndexing;Microphone;NFC;RemovableMedia;UserDirs;WebView`) через sailjaild →
внутри песочницы недоступны `/run/display/wayland-0` и `/dev/mali0`, процесс падает
до вывода ошибки. Дополнительно у нас был нестандартный ключ
`X-Nemo-Application-Type=no-invoker` (lipstick его не знает).
**Решение:** в `.desktop`:
`[X-Sailjail]` + `Sandboxing=Disabled` (так сделано у системных `fingerterm.desktop`,
`csd.desktop`); убрать `X-Nemo-Application-Type`. Лаунчер писать журнал в
`$HOME/hello-ns220.log` (из меню нет терминала). Пример —
`apps/hello-ns220/packaging/aurora-hello-ns220.desktop`.
Для настоящей песочницы (если понадобится) — описать `Permissions=` и
`OrganizationName`/`ApplicationName`, а не отключать.

## B030. Иконка в меню не находится: `Failed to get image from provider: image://theme/<icon>`
**Причина:** lipstick берёт иконку из темы Aurora:
`/usr/share/themes/aurora-default/meegotouch/z1.0/icons/<Icon>.png`.
В `/usr/share/icons/hicolor` **нет** `index.theme`, поэтому Qt-фолбэк оттуда иконку
не находит.
**Решение:** ставить PNG ещё и в каталог темы (достаточно 86×86 и/или 256×256),
имя файла = значение `Icon=` из `.desktop`. См. `apps/hello-ns220/packaging/*.spec`.

## B031. G-сенсор: данные не приходят (HybrisAdaptor::setInterval not ok)
**Симптом:** sensorfw `requestSensor("accelerometersensor", …)` даёт id, объект
`/SensorManager/accelerometersensor` есть, но сигналов `dataAvailable(tiii)` нет;
QtSensors: `isActive()=1`, `error()=0`, значения всегда (0,0,0).
**Причина:** Android-адаптер sensorfw (`libhybrisaccelerometeradaptor-qt5.so`) не
может задать интервал — MTK HAL отвергает `setDelay()`:
`HybrisAdaptor::setInterval(unsigned int, int) setInterval not ok`.
**Проверено (не помогло):** `busctl --system monitor` (сигналов нет);
`/dev/gsensor` ioctl `0xF001/0xF002/0xF00A/0xF00B` → `-1`; прямой HAL через
libhybris (`hw_get_module` → `open` → `activate`) — список датчиков читается
(ACCELEROMETER handle=1), но `activate()` = SIGSEGV; `/dev/i2c-*` отсутствует
(i2c-dev), драйвер BMA222 не привязан (сенсор за MTK SCP).
**Решение:** пока нет. Варианты: заставить hybris-адаптер использовать `batch()`
(HAL 1.3), свой адаптер на `ISensors` (hwbinder), либо ядерный интерфейс SCP.
В приложении честно показывается «HAL не отдаёт данные».

## B032. Geoclue: «no usable Position providers» и `dbus.Store: length mismatch`
**Симптом:** `Master.Create()` → клиент, но `GetPosition` → «master client has no
usable Position providers»; в Go `dbus.Store: length mismatch` на `GetPosition`.
**Причина:** (1) провайдер `geoclue-provider-hybris` в прошивке **disabled**, его
D-Bus-файл `Exec=/bin/false` (запуск только через systemd-юнит), поэтому мастер
не видит провайдера; (2) `GetPosition` возвращает `iiddd(idd)` — структуру в
скобках, `dbus.Store` в `[]interface{}` разложить не может.
**Решение:** запускать юнит
`org.freedesktop.systemd1.Manager.StartUnit("geoclue-providers-hybris.service","fail")`
на user-шине (или `systemctl --user start geoclue-providers-hybris`), опрашивать
**провайдера напрямую** (`org.freedesktop.Geoclue.Position.GetPosition`), а тело
ответа разбирать вручную (`call.Body`), не через `Store`. См. `apps/hello-ns220/gps.go`.

## B033. `pkill -f <имя>` убивает собственную ssh-сессию
**Симптом:** команда через ssh «обрывается» без вывода, хотя процесс жив/убит.
**Причина:** `pkill -f hello-ns220` матчит строку `sh -c '… hello-ns220 …'`
(сама команда содержит имя) и убивает сессию до печати результата.
**Решение:** `pkill -x hello-ns220` (точное имя процесса) либо хранить pid.

## B034. cgo: `invalid flag in #cgo LDFLAGS: -l:libpulse-simple.so.0`
**Симптом:** сборка падает на `-l:` синтаксисе (Go не пропускает его в LDFLAGS).
**Причина:** cgo проверяет whitelist флагов; `-l:` не входит.
**Решение:** создать dev-симлинки в каталоге, который уже в `CGO_LDFLAGS -L`:
`ln -s /usr/lib/libpulse-simple.so.0 /home/opt/libs/libpulse-simple.so` и так же
для `libpulse.so`; линковать обычными `-lpulse-simple -lpulse`.
