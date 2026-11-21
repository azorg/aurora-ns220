#!/bin/sh
# Запуск cterm на GPU (Mali через libhybris), с автоматическим откатом на llvmpipe.
# Использование: run-cterm-hw.sh [аргументы cterm...]
# Переменные: CTERM_GPU=auto|hw|sw (по умолчанию auto), CTERM_BIN, CTERM_LOG.
set +e

BIN=${CTERM_BIN:-/home/user/clear-ssh/bin/cterm}
# Клавиатура должна быть у нижнего края экрана: в Wayland клиент не может
# задать позицию окна, поэтому с --osk открываемся на весь экран.
case " $* " in
  *" --osk "*)
    case " $* " in
      *" --osk-fullscreen "*) ;;
      *) set -- "$@" --osk-fullscreen ;;
    esac ;;
esac
LOG=${CTERM_LOG:-/tmp/cterm.log}
GPU=${CTERM_GPU:-auto}
# На 1200x1920 без масштабирования клавиатура влезает целиком; при FYNE_SCALE>1
# растут шрифты/клавиши и строки обрезаются справа. Переопределяется извне.
FYNE_SCALE=${FYNE_SCALE:-1.0}
export FYNE_SCALE

# Локаль: на планшете есть ru_RU.utf8. При POSIX/C xkbcommon не находит
# Compose-файл и раскладка/ввод кириллицы работают неправильно.
# Крупные клавиши планшета (~квадрат при 10-11 в ряд); на десктопе
# умолчание пакета - компактное (FYNEOSK_ROW=56/TOP=44).
export FYNEOSK_ROW=${FYNEOSK_ROW:-110}
export FYNEOSK_TOP=${FYNEOSK_TOP:-110}

export LANG=${LANG:-ru_RU.UTF-8}
export LC_ALL=${LC_ALL:-$LANG}

# Compose-файл X11: в Aurora каталога /usr/share/X11/locale нет, и
# xkbcommon ругается «Failed to create XKB compose table» (мёртвые клавиши
# не работают). Берём en_US-таблицу, если она установлена.
if [ -z "${XCOMPOSEFILE:-}" ]; then
  for f in /usr/share/X11/locale/en_US.UTF-8/Compose            /usr/share/X11/locale/ru_RU.UTF-8/Compose; do
    [ -r "$f" ] && { XCOMPOSEFILE=$f; export XCOMPOSEFILE; break; }
  done
fi

export XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR:-/run/user/100000}
export WAYLAND_DISPLAY=${WAYLAND_DISPLAY:-/run/display/wayland-0}
DBUS_SESSION_BUS_ADDRESS=${DBUS_SESSION_BUS_ADDRESS:-unix:path=/run/user/100000/dbus/user_bus_socket}
export DBUS_SESSION_BUS_ADDRESS

# общие для обоих режимов
unset LIBGL_DRIVERS_PATH LIBGL_ALWAYS_SOFTWARE MESA_LOADER_DRIVER_OVERRIDE GALLIUM_DRIVER
unset LD_LIBRARY_PATH

start_hw() {
  EGL_PLATFORM=wayland "$BIN" "$@" >>"$LOG" 2>&1 &
  PID=$!
}

start_sw() {
  LD_LIBRARY_PATH=/home/opt/mesa/usr/lib \
  LIBGL_DRIVERS_PATH=/home/opt/mesa/usr/lib/dri \
  MESA_LOADER_DRIVER_OVERRIDE=swrast \
  LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe \
  EGL_PLATFORM=wayland "$BIN" "$@" >>"$LOG" 2>&1 &
  PID=$!
}

has_mali() { grep -q "libGLES_mali\.so" /proc/$1/maps 2>/dev/null; }

if [ "$GPU" != sw ]; then
  start_hw "$@"
  S=0; while [ $S -lt 10 ]; do sleep 1; kill -0 $PID 2>/dev/null || break; S=$((S+1)); done
  if kill -0 $PID 2>/dev/null && has_mali $PID; then
    echo "gpu: Mali-T720 (hardware), pid=$PID" | tee -a "$LOG"
    exit 0
  fi
  echo "gpu: hardware path failed, falling back to llvmpipe" | tee -a "$LOG"
  kill -9 $PID 2>/dev/null
  [ "$GPU" = hw ] && exit 1
fi

start_sw "$@"
sleep 3
if kill -0 $PID 2>/dev/null; then
  echo "gpu: llvmpipe (software), pid=$PID" | tee -a "$LOG"
  exit 0
fi
echo "gpu: both paths failed, see $LOG" | tee -a "$LOG"
exit 1
