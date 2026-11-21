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

## B035. Приложение из меню Aurora не открывает окно (спиннер и ничего)
**Симптом:** `.desktop` виден в меню, иконка есть; при нажатии — спиннер,
окно не появляется; процесс приложения при этом живёт и рисует кадры
(в `~/hello-ns220.log` идут `STATS fps=…`). Повторные нажатия ничего не меняют.
**Причина (две части):**
1. `Exec=` в `.desktop` был **shell-скриптом** (`aurora-hello-ns220-launch`).
   lipstick запускает приложения через
   `invoker --type=generic --id=<name> --single-instance <Exec>`;
   для скрипта booster/invoker ведёт себя непредсказуемо (второй запуск
   подвисал в `invoker`, окно не активировалось).
2. `Exec`-скрипт с `--single-instance`: пока жив первый экземпляр, lipstick
   считает приложение «уже запущенным» и лишь сигналит ему; приложение сигнал
   не обрабатывает — визуально «не стартует». Плюс оставался старый процесс,
   который не убивался (`pkill -x` по имени `aurora-hello-ns220` не срабатывает:
   ядро хранит `comm` обрезанным до 15 символов — `aurora-hello-ns`).
**Решение:** `Exec` — обычный ELF; всё окружение (XDG_RUNTIME_DIR,
WAYLAND_DISPLAY, DBUS_SESSION_BUS_ADDRESS, EGL_PLATFORM, FYNE_SCALE, LANG/LC_ALL,
XCOMPOSEFILE) выставляет само приложение (`setupEnv()` в `main.go`) до `app.New`;
журнал — `setupLog()` в `$HOME/hello-ns220.log`. В `.desktop` добавлены
`X-Nemo-Application-Type=no-invoker`, `TryExec`, `StartupNotify`, `StartupWMClass`.
Проверено: `env -i … /usr/bin/aurora-hello-ns220` и
`invoker --type=generic --id=aurora-hello-ns220 --single-instance /usr/bin/aurora-hello-ns220`
запускаются и рисуют окно 1200×1920.

## B036. Виброклик на тап не ощущается (хотя «Вибрация 250 мс» работает)
**Симптом:** кнопка «Вибрация 250 мс» даёт отчётливый отклик, а виброклик на тап
(25 мс) не чувствуется вовсе.
**Причина:** ядро MTK (`timed_output/vibrator`) честно включает мотор на 25 мс, но
этого мало для раскрутки ротора — ощутимая длительность начинается примерно с
40–60 мс.
**Решение:** тап — 60 мс, двойной тап — 90 мс; в журнал пишется `VIBRO tap 60ms`
(для проверки без пальцев). Значения — в `apps/hello-ns220/input.go`.

## B037. Прямой доступ к Android sensors HAL: vtable устройства пустой
**Симптом:** `hw_get_module("sensors")` отдаёт модуль «MTK SENSORS Module» и список
датчиков (ACCELEROMETER handle=1), `open()` возвращает 0, но у устройства
`activate/setDelay/poll/batch = NULL`; вызов `activate` = SIGSEGV (переход на 0).
**Причина:** HAL в этой прошивке **binderized**: модуль — лишь прокси, реальная
работа идёт через `ISensors`/hwbinder, и в клиентском процессе указатели не
заполняются. Именно поэтому sensorfwd (который ходит через hwbinder) получает
`setDelay not ok` от MTK-реализации, а наши прямые вызовы падают.
**Что это значит:** нужен клиент `android.hardware.sensors@1.0::ISensors` через
libhwbinder/libhidlbase (в `/usr/lib` их нет, но в `/system/lib` есть HIDL-библиотеки
Android) либо патч sensorfwd (LD_PRELOAD-обёртка `hw_get_module`, которая
переводит `setDelay` в `batch`), см. doc/15 §15.6 и план 0016 P7.

## B038. После `reboot` планшет остановился в меню загрузчика LK (USB 0bb4:0afe)
**Симптом:** после `devel-su reboot` устройство не поднимает сеть; на хосте USB
`0bb4:0afe` (LK boot-menu/UMS), появляется `/dev/sda` нулевого размера; SSH/RNDIS нет.
**Причина:** LK в этой прошивке иногда показывает boot-menu на перезагрузке и ждёт
выбора (наблюдалось и раньше, REPORT §19). Внешне это выглядит как «не включается».
**Решение:** нужен человек у экрана — выбрать **Normal/Start** (или коротко нажать
питание). После выбора система грузится штатно. Автоматически из хоста не
выбирается: fastboot в этом режиме недоступен, UMS отдаёт пустой диск.
**Профилактика:** перезагружать лучше кнопкой питания (штатное выключение/включение)
или проверять, что устройство вернулось в `0bb4:0a02` (RNDIS), прежде чем слать
команды. Для «умного» сброса — предусмотреть в скрипте ожидание сети с сообщением
«нажмите Normal на планшете».

## B039. Экран перевернулся на 180° после опытов с sensors HAL
**Симптом:** UI Aurora/lipstick оказывается перевёрнутым; поворот «залип».
**Причина:** поворот экрана в Aurora ведёт lipstick по данным ориентации из
sensorfw; наши прямые эксперименты с Android sensors HAL (вызовы
`activate`/`poll` на NULL-vtable, сессии `requestSensor` без `releaseSensor`)
подвесили sensorfwd — ориентация перестала обновляться и осталась в последнем
состоянии.
**Решение:** перезагрузка (сбрасывает sensorfwd, lipstick и HAL-сервисы) — после
неё поворот в норме. Мягкая альтернатива: `systemctl --user restart lipstick`
и `systemctl restart sensorfwd`, но при зависшем HAL надёжнее ребут.
**Профилактика:** не вызывать напрямую методы HAL на устройстве (только через
sensorfwd/QtSensors), после `requestSensor` делать `releaseSensor`.

## B040. «Закрытие» приложения из Aurora убирает окно, но процесс живёт
**Симптом:** закрываешь приложение обычным способом (свайп/крестик в Aurora) —
окно исчезает, но процесс продолжает работать (в журнале идут `STATS fps=…`);
повторный запуск из меню ничего не показывает (invoker `--single-instance`
лишь сигналит старому процессу). Приходится `killall`.
**Причина:** lipstick управляет окнами, но процесс не завершает — это делает
booster/mapplauncherd для «настоящих» Aurora-приложений. У нас `Exec` — обычный
ELF без booster (и с `X-Nemo-Application-Type=no-invoker`), а `wl_shell` не даёт
клиенту уведомления «окно закрыто WM» — Fyne/GLFW об этом не узнают.
**Решение (в hello-ns220 1.0.0-7):**
* кнопка **«Выход»** в панели — явный `a.Quit()`;
* корректный выход по `SIGTERM`/`SIGINT` (для `kill`/`killall`), с записью
  `SIGNAL … — выходим` в журнал;
* **один экземпляр**: сокет `$XDG_RUNTIME_DIR/aurora-hello-ns220.sock` — повторный
  запуск просит первый экземпляр показать окно (`show`) и завершается сам;
  в журнале `SINGLE: …`;
* диагностика жизненного цикла: `WINDOW closed (Fyne)`, `APP stopped`.
**Правильный путь для «настоящих» приложений Aurora:** booster-совместимый
бинарник (mapplauncherd) или `.desktop` через `invoker`, тогда закрытие из
липстика завершает процесс; для наших dev-приложений достаточно Exit+single-instance.

## B041. `mount --bind /home /run` ломает сессию: нет `/run/user/*`, `devel-su` не принимает пароль, ребут
**Симптом:** после эксперимента `mount --bind /home /run` (и `umount -l /run`)
пропадает systemd-сессия пользователя (`There is no systemd user session running`,
`Failed to connect to bus`), `devel-su` с верным паролем отвечает «не подходит»,
устройство может уйти в перезагрузку.
**Причина:** `/run` — tmpfs с сокетами systemd/logind/D-Bus и `/run/user/<uid>`;
bind-монтирование поверх него прячет всё это, systemd теряет свои сокеты, а
аутентификация `devel-su` (через PAM/logind) перестаёт работать. Ленивое
`umount -l /run` не спасает: сервисы уже потеряли пути, ядро уходит в watchdog-ребут.
**Решение:** не монтировать ничего поверх `/run` и `/`; для exec-доступа к `/home`
использовать отдельный каталог (`/run/debian-home` и т.п. — но НЕ сам `/run`),
либо `mount -o remount,exec /home`. Восстановление — только перезагрузка
(при необходимости — выбор Normal в boot-menu LK, см. B038).

## B042. В chroot-обёртке `devel-su` спрашивает пароль и не принимает его (из терминала/меню)
**Симптом:** при запуске `debian-chroot` из fingerterm появляется `Password:`,
ввод `1234` не принимается; из ssh (`echo 1234 | devel-su`) тот же пароль работает.
**Причина (две):** (1) `devel-su` читает пароль из `/dev/tty`, а обёртка подавала его
через pipe — в терминале пользователь видел prompt, но предыдущая попытка уже
«съела» ввод; (2) в тот момент был испорчен `/run` (B041), из-за чего PAM/logind
не могли проверить пароль вообще.
**Решение:** вход в chroot сделан **setuid-хелпером** `/usr/local/bin/debian-enter`
(C, 4755, исходник `packaging/debian-chroot/debian-enter.c`): сам делает
bind-mount `/home` → `/run/debian-home` с `exec`, bind `/proc /sys /dev /dev/pts`,
tmpfs `/tmp`, chroot, запускает `bash -l` (или переданную команду) и после выхода
размонтирует. Пароль не спрашивается. Обёртка `debian-chroot` использует хелпер,
если он есть, иначе — старый путь через `devel-su`.

## B043. G-сенсор: после D-Bus `requestSensor` данные перестают идти
**Симптом:** после эксперимента с `local.SensorManager.requestSensor` (без
`releaseSensor`) сенсор «залипает»: `QOrientationSensor` ещё отвечает, а
`QAccelerometer` отдаёт нули; в журнале `HybrisAdaptor::setInterval … not ok`.
**Причина:** сессия sensorfw остаётся с активным сенсором; MTK HAL 1.3 отвергает
смену интервала у уже активного датчика, и новые клиенты не получают событий.
**Решение:** не вызывать D-Bus `requestSensor` напрямую (только QtSensors/helper);
после `requestSensor` обязательно `releaseSensor`. Сброс — `aurora-sensor-reset`
(HAL + sensorfwd), надёжнее — перезагрузка.

## B044. G-сенсор: helper печатает «ACC 0,873 …» — парсер не видит значения
**Симптом:** helper работает, ориентация (`ORT`) приходит, а `ACC`/`MAG` — нет;
в журнале приложения `ACCEL helper: POLL 0,873 -3,256 9,110`.
**Причина:** локаль `ru_RU.UTF-8` — C `printf("%.3f")` и Qt печатают числа с
**запятой**, а Go-парсер (`strconv.ParseFloat`) ждёт точку.
**Решение:** в helper `setlocale(LC_NUMERIC, "C")`; в Go — нормализация запятой
(`parseNum`). Проверено: на экране `x=+0.83 y=-3.26 z=+9.11 |g|=9.71`.

## B045. MTK sensors HAL «залипает» после нескольких клиентов
**Симптом:** первые 1–2 сессии после перезагрузки дают ACC/компас, затем
`setInterval not ok` и нули; `systemctl restart sensorfwd` не помогает,
`setprop ctl.restart sensors-hal-1-0` — через раз.
**Решение:** для стабильных данных — перезагрузка и **один** клиент-потребитель
(наше приложение держит helper всё время работы). В приложении есть авто-сброс
(`aurora-sensor-reset`) после трёх неудач; если не помогло — перезагрузить.
Причина — vendor HAL 1.3 (binderized), см. B037.

## B046. `rpm -Uvh` падает: «needs 5MB more space on the / filesystem»
**Симптом:** установка обновлённого пакета не проходит, rootfs занят на 99%.
**Причина:** `rpmbuild` по умолчанию собирает в `$HOME/rpmbuild` (у root это
`/root/rpmbuild`, 144 МБ BUILDROOT на rootfs 2.3 ГБ), а собранный Go-бинарь
занимает 25 МБ.
**Решение:** собирать RPM в `/home/user/rpmbuild` (8.4 ГБ) — `build-rpm.sh` теперь
так и делает; старый `/root/rpmbuild` удалён. Держать на `/` не менее ~150 МБ.

## B047. Системный поворот экрана в Aurora 4 на NS220 залочен вендором
**Симптом:** G-сенсор «залипает» (B045), автоматический поворот UI не работает;
попытки включить поворот через dconf ничего не меняют.
**Причина:** в прошивке ключ dconf `/desktop/jolla/components/screen_rotation_angle`
зафиксирован и **заблокирован**: `/etc/dconf/db/vendor.d/screen-rotation.txt`
(`screen_rotation_angle=0`) и `locks/screen-rotation.txt`. Ключ lipstick
`system_settings/look_and_feel/display/orientation_lock` (он же в верхнем меню)
на поворот композитора не влияет: после установки `'landscape'` и перезапуска
lipstick окно остаётся 1200×1920. То есть ОС на этом устройстве портретная.
**Решение:** поворот делать **в приложении**: в `hello-ns220` кнопки
«⟲ 90° / ⟳ 90° / 0°» поворачивают сцену вокруг центра (флаг `-rot N` для
старта), входящие касания пересчитываются обратно (`unrotPoint`), HUD показывает
текущий угол. Системный UI при этом остаётся портретным.

## B048. Через приложение нельзя позвонить 112 (нет SIM/регистрации)
**Симптом:** интерес «можно ли позвонить 112».
**Факты:** модем `/ril_0` online (`Online=true`), IMEI `356399112625294`,
но `org.ofono.SimManager.GetProperties` → `Present=false`; интерфейса
`NetworkRegistration` в сборке нет; `Emergency=false`.
**Вывод:** без SIM-карты регистрации в сети нет, экстренный вызов невозможен.
Звонок на 112 мы намеренно **не проверяем** (реальная служба спасения).
С появлением SIM — проверять только тестовым номером оператора.
