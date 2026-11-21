# 06. Упаковка: RPM `aurora-fyne-runtime`

Цель: один пакет, после установки которого **любое Fyne-приложение** собирается и
запускается на Aurora без ручной возни.

## 6.1 Раскладка файлов

```
/opt/aurora-fyne/
├── mesa/usr/lib/            # libEGL.so.1, libGLESv2.so.2, libglapi, libgbm, libdrm, dri/swrast_dri.so
├── include/                 # EGL/, GLES2/, GLES3/, GL/, KHR/, linux/input-event-codes.h
├── pc/                      # libdrm.pc, wayland-egl.pc (+ копии egl.pc/glesv2.pc)
├── libs/                    # симлинки lib*.so → системные .so.N
├── glfw/                    # GLFW 3.4 (Wayland-only) — опционально, для C-тестов
└── go-glfw/                 # ПАТЧЕННЫЙ Go-модуль github.com/go-gl/glfw/v3.3/glfw
/usr/bin/fyne-aurora-build   # сборка проекта
/usr/bin/fyne-aurora-run     # запуск приложения
/etc/profile.d/fyne-aurora.sh
```

Тулчейн Go **не включаем** в RPM по умолчанию (400 МБ, редко обновляется) — можно сделать
отдельный `aurora-fyne-go` или положить в `/opt/aurora-fyne/go`.

## 6.2 Скрипты (готовые драфты — `../pkg/`)

`fyne-aurora-build`:

```sh
#!/bin/sh
set -e
[ -x /opt/aurora-fyne/go/bin/go ] && PATH=/opt/aurora-fyne/go/bin:$PATH
export GOPATH=${GOPATH:-/home/user/.cache/go}
export GOMODCACHE=${GOMODCACHE:-$GOPATH/pkg/mod}
export GOCACHE=${GOCACHE:-$GOPATH/build-cache}
export PKG_CONFIG_PATH=/opt/aurora-fyne/pc:/opt/aurora-fyne/usr/lib/pkgconfig:/usr/lib/pkgconfig
export CGO_CFLAGS="-D_GNU_SOURCE -DEGL_NO_X11 -I/opt/aurora-fyne/include"
export CGO_LDFLAGS="-L/opt/aurora-fyne/libs -lwayland-client -lwayland-cursor \
                    -lwayland-egl -lxkbcommon -lEGL -lGLESv2 -lpthread -lm -ldl"
exec go build -tags "wayland gles egl" "$@"
```

`fyne-aurora-run`:

```sh
#!/bin/sh
UID_=$(id -u)
export XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR:-/run/user/$UID_}
export WAYLAND_DISPLAY=${WAYLAND_DISPLAY:-/run/display/wayland-0}
export DBUS_SESSION_BUS_ADDRESS=${DBUS_SESSION_BUS_ADDRESS:-unix:path=$XDG_RUNTIME_DIR/dbus/user_bus_socket}
export LD_LIBRARY_PATH=/opt/aurora-fyne/mesa/usr/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}
export LIBGL_DRIVERS_PATH=/opt/aurora-fyne/mesa/usr/lib/dri
export MESA_LOADER_DRIVER_OVERRIDE=swrast
export LIBGL_ALWAYS_SOFTWARE=1
export GALLIUM_DRIVER=llvmpipe
export EGL_PLATFORM=wayland
export FYNE_SCALE=${FYNE_SCALE:-2.5}
exec "$@"
```

`/etc/profile.d/fyne-aurora.sh` — экспорт `PATH=/opt/aurora-fyne/go/bin:$PATH`,
`FYNE_SCALE`, подсказка про `-replace`.

## 6.3 Как проект подключает патченный GLFW

**Способ A (рекомендуемый): `replace` на системную копию**

```bash
go mod edit -replace github.com/go-gl/glfw/v3.3/glfw=/opt/aurora-fyne/go-glfw
go mod tidy
```

**Способ B: vendor + патч**

```bash
go mod vendor
patch -p1 -d vendor/github.com/go-gl/glfw/v3.3/glfw < /usr/share/aurora-fyne/glfw-wl-shell.patch
go clean -cache
```

Оба способа требуют, чтобы `go-gl/glfw` был в зависимостях (он тянется через Fyne).

## 6.4 SPEC (драфт)

См. `../pkg/aurora-fyne-runtime.spec`. Ключевое:

* `BuildArch: armv7hl`, `Requires: wayland, libxkbcommon, glibc, gcc, mesa-llvmpipe-libEGL ...`
  (для runtime-библиотек Mesa указываем явные версии пакетов OMP);
* `%install` копирует дерево `/opt/aurora-fyne`, скрипты в `/usr/bin`, профиль в
  `/etc/profile.d`;
* `%post` — проверить `mount | grep ' /home '` на `noexec` и напечатать предупреждение;
* `%files` — `/opt/aurora-fyne`, `/usr/bin/fyne-aurora-*`, `/etc/profile.d/fyne-aurora.sh`,
  `/usr/share/aurora-fyne/glfw-wl-shell.patch`.

Сборка на устройстве:

```bash
rpmbuild -bb --define "_topdir $HOME/rpmbuild" \
  --define "__brp_strip_static_archive /bin/true" \
  pkg/aurora-fyne-runtime.spec
```

(требуется FQDN-hostname; `brp-strip-static-archive` обходим — busybox-`xargs` не умеет `-d`)

## 6.5 Установка собственного RPM на Aurora

Плагин `validation.so` требует подпись developer-сертификатом OMP. Варианты:

1. **Обход валидации** (при `ima_appraise=off` работает):
   ```bash
   rpm -i --define '__transaction_validation %{nil}' aurora-fyne-runtime-*.armv7hl.rpm
   ```
2. **Подпись своим CA**: добавить свой CA в `/etc/keys/packages/ca.pem` (бэкап сделать!) и
   подписать пакет `rpmsign-external sign`. Учтите: файлы получают `security.ima` от вашего
   ключа, но `.ima`-keyring его не примет — важно только при включённом IMA.

## 6.6 Что даст пакет разработчику

```bash
# 1. поставить пакет (один раз)
rpm -i --define '__transaction_validation %{nil}' aurora-fyne-runtime-*.rpm

# 2. в проекте
go mod edit -replace github.com/go-gl/glfw/v3.3/glfw=/opt/aurora-fyne/go-glfw
go mod tidy

# 3. собрать и запустить
fyne-aurora-build -o app .
fyne-aurora-run ./app
```

## 6.7 Замечания по переносимости

* Пакет собирается под конкретную версию Aurora (4.0.2.340) и ABI `armv7hl`.
* Mesa-библиотеки берутся из репозитория OMP — версия в `Requires` должна совпадать.
* При обновлении Aurora пакет надо пересобрать (пересчитать `Requires`/`Provides`).

## 6.6. Появление в меню Aurora и запуск (sailjail, иконка)

Проверено на `aurora-hello-ns220` (Aurora 4.0.2.340). Чтобы приложение появилось в
меню и запускалось, кроме `.desktop` нужно учесть песочницу и путь иконки.

### Песочница sailjail

Без секции `[X-Sailjail]` липстик запускает приложение в песочнице по «Default
Profile» (`/etc/sailjail/config`): при первом запуске появляется вопрос о
разрешениях, а само приложение падает — внутри песочницы нет доступа к
`/run/display/wayland-0` и `/dev/mali0`.

Для инструментов разработки (как у системных `fingerterm`, `csd`) песочницу можно
отключить:

```ini
[X-Sailjail]
Sandboxing=Disabled
```

Ключ `X-Nemo-Application-Type` в Aurora не обязателен; нестандартные значения
(например `no-invoker`) лучше не ставить — проверяйте, что приложение стартует
именно через `Exec`.

### Иконка

lipstick рисует иконку через `image://theme/<Icon>`, то есть ищет
`/usr/share/themes/aurora-default/meegotouch/z1.0/icons/<Icon>.png`.
В `/usr/share/icons/hicolor` нет `index.theme`, поэтому Qt-фолбэк не работает.
Ставьте PNG в каталог темы (имя = `Icon=`), 86×86 и 256×256.

### Диагностика

* Из меню нет терминала — лаунчер (`Exec=`) должен писать журнал
  (`exec >>"$HOME/app.log" 2>&1`), иначе сбой выглядит как «кружок и тишина».
* `journalctl -b -u sailjaild | tail` — отказы песочницы;
  `journalctl -b | grep image://theme` — жалобы на иконку.
* Проверка вне меню: `env -i HOME=$HOME PATH=/usr/bin:/bin <Exec-команда>` —
  имитирует пустое окружение бустера.
* Запуск «как из меню» без тапа: `invoker --type=generic <Exec-команда>`.
