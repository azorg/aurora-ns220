#!/bin/sh
# Запуск Fyne-приложения на Aurora с авто-выбором пути рендеринга.
#
#   APP=./myapp GPU=auto scripts/run-fyne-gpu.sh [аргументы приложения...]
#
# GPU=auto (по умолчанию) — пробуем libhybris/Mali, при неудаче откат на llvmpipe.
# GPU=hw                 — только Mali (ошибка, если недоступен).
# GPU=sw                 — только llvmpipe.
#
# Переменные: APP (обязательна), LOG (по умолчанию /tmp/fyne-gpu.log),
#             FYNE_SCALE (по умолчанию 1.0 — на 1200x1920 при 2.5 контент обрезается).
set +e

APP=${APP:?APP=/path/to/binary}
GPU=${GPU:-auto}
LOG=${LOG:-/tmp/fyne-gpu.log}
FYNE_SCALE=${FYNE_SCALE:-1.0}
export FYNE_SCALE

export XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR:-/run/user/100000}
export WAYLAND_DISPLAY=${WAYLAND_DISPLAY:-/run/display/wayland-0}
DBUS_SESSION_BUS_ADDRESS=${DBUS_SESSION_BUS_ADDRESS:-unix:path=/run/user/100000/dbus/user_bus_socket}
export DBUS_SESSION_BUS_ADDRESS

# общие: убираем всё, что может подмешать Mesa поверх libhybris
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

if [ "$GPU" != sw ]; then
  start_hw "$@"
  S=0; while [ $S -lt 10 ]; do sleep 1; kill -0 $PID 2>/dev/null || break; S=$((S+1)); done
  if kill -0 $PID 2>/dev/null && has_mali $PID; then
    echo "gpu: Mali-T720 (hardware), pid=$PID" | tee -a "$LOG"; exit 0
  fi
  echo "gpu: hardware path failed, falling back to llvmpipe" | tee -a "$LOG"
  kill -9 $PID 2>/dev/null
  [ "$GPU" = hw ] && exit 1
fi

start_sw "$@"
sleep 3
if kill -0 $PID 2>/dev/null; then
  echo "gpu: llvmpipe (software), pid=$PID" | tee -a "$LOG"; exit 0
fi
echo "gpu: both paths failed, see $LOG" | tee -a "$LOG"; exit 1
