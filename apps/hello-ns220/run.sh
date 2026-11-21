#!/bin/sh
# Запуск hello-ns220 на планшете (в сессии пользователя, Wayland lipstick).
#   ./run.sh [аргументы приложения...]
# GPU=hw|sw|auto (по умолчанию auto — Mali с откатом на llvmpipe).
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
APP=${APP:-$HERE/hello-ns220}
GPU=${GPU:-auto}
LOG=${LOG:-/tmp/hello-ns220.log}
UID_=${AURORA_UID:-100000}

export XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR:-/run/user/$UID_}
export WAYLAND_DISPLAY=${WAYLAND_DISPLAY:-/run/display/wayland-0}
export DBUS_SESSION_BUS_ADDRESS=${DBUS_SESSION_BUS_ADDRESS:-unix:path=$XDG_RUNTIME_DIR/dbus/user_bus_socket}
export FYNE_SCALE=${FYNE_SCALE:-1.0}

# Локаль и Compose: при POSIX/C xkbcommon не находит Compose-файл
# (в Aurora нет /usr/share/X11/locale) и окно не создаётся:
#   PlatformError: Wayland: Failed to create XKB compose table
export LANG=${LANG:-ru_RU.UTF-8}
export LC_ALL=${LC_ALL:-$LANG}
if [ -z "${XCOMPOSEFILE:-}" ]; then
    for f in /usr/share/X11/locale/en_US.UTF-8/Compose /usr/share/X11/locale/ru_RU.UTF-8/Compose; do
        [ -r "$f" ] && { XCOMPOSEFILE=$f; export XCOMPOSEFILE; break; }
    done
fi

unset LIBGL_DRIVERS_PATH LIBGL_ALWAYS_SOFTWARE MESA_LOADER_DRIVER_OVERRIDE GALLIUM_DRIVER
unset LD_LIBRARY_PATH

start_hw() { EGL_PLATFORM=wayland "$APP" "$@" >>"$LOG" 2>&1 & PID=$!; }
start_sw() {
    LD_LIBRARY_PATH=/home/opt/mesa/usr/lib \
    LIBGL_DRIVERS_PATH=/home/opt/mesa/usr/lib/dri \
    MESA_LOADER_DRIVER_OVERRIDE=swrast \
    LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe \
    EGL_PLATFORM=wayland "$APP" "$@" >>"$LOG" 2>&1 & PID=$!
}
has_mali() { grep -q "libGLES_mali\.so" /proc/$1/maps 2>/dev/null; }

case "$GPU" in
  sw) start_sw "$@" ;;
  hw) start_hw "$@" ;;
  *)
     start_hw "$@"; sleep 3
     if has_mali "$PID"; then echo "[i] GPU: Mali (libhybris)"; else
        echo "[!] Mali не поднялся — откат на llvmpipe"; kill "$PID" 2>/dev/null; wait "$PID" 2>/dev/null; start_sw "$@"; sleep 2
     fi ;;
esac

sleep 1
echo "[i] pid=$PID, лог: $LOG"
