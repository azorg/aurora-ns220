#!/bin/sh
# Лаунчер aurora-hello-ns220: выставляет окружение, нужное Fyne/GLFW на Aurora,
# и запускает приложение. Используется из .desktop (меню Aurora) и вручную.
#
# Почему нужен лаунчер:
#   * без LANG/LC_ALL + Compose-файла xkbcommon не строит compose-таблицу
#     (в Aurora нет /usr/share/X11/locale, но Compose лежит в /usr/share/X11/xkb);
#   * системный EGL — libhybris, ему нужен EGL_PLATFORM=wayland, иначе
#     ассерт ANativeWindow;
#   * FYNE_SCALE=1.0 — на 1200x1920 без масштаба контент помещается целиком.
set -e
APP=/usr/bin/aurora-hello-ns220

# Журнал запуска: из меню Aurora у приложения нет терминала, и без этого
# любой сбой выглядит как «кружок и тишина». Пишем в домашний каталог
# (в /tmp у сессий systemd свой PrivateTmp).
LOG=${HELLO_LOG:-$HOME/hello-ns220.log}
exec >>"$LOG" 2>&1
echo "--- $(date '+%F %T') launch: $(id -un) uid=$(id -u) WAYLAND_DISPLAY=${WAYLAND_DISPLAY:-} EGL_PLATFORM=${EGL_PLATFORM:-}" 

export LANG=${LANG:-ru_RU.UTF-8}
export LC_ALL=${LC_ALL:-$LANG}
if [ -z "${XCOMPOSEFILE:-}" ]; then
    for f in /usr/share/X11/locale/en_US.UTF-8/Compose \
             /usr/share/X11/locale/ru_RU.UTF-8/Compose \
             /usr/share/X11/xkb/compose/en_US.UTF-8 \
             /usr/share/X11/xkb/compose/ru_RU.UTF-8; do
        [ -r "$f" ] && { XCOMPOSEFILE=$f; export XCOMPOSEFILE; break; }
    done
fi

export EGL_PLATFORM=${EGL_PLATFORM:-wayland}
export FYNE_SCALE=${FYNE_SCALE:-1.0}
export XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR:-/run/user/$(id -u)}
export WAYLAND_DISPLAY=${WAYLAND_DISPLAY:-/run/display/wayland-0}
[ -n "$DBUS_SESSION_BUS_ADDRESS" ] || \
    export DBUS_SESSION_BUS_ADDRESS="unix:path=$XDG_RUNTIME_DIR/dbus/user_bus_socket"

# не подмешивать Mesa llvmpipe поверх libhybris
unset LD_LIBRARY_PATH LIBGL_DRIVERS_PATH MESA_LOADER_DRIVER_OVERRIDE GALLIUM_DRIVER

exec "$APP" "$@"
