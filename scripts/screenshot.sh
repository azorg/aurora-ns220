#!/bin/sh
# Скриншот экрана Aurora через D-Bus lipstick.
#   screenshot.sh [путь-в-домашнем-каталоге]
# lipstick принимает только пути внутри home пользователя сессии.
set -e
UID_="${AURORA_UID:-100000}"
OUT="${1:-/home/user/shot.png}"
export XDG_RUNTIME_DIR="/run/user/$UID_"
export DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$UID_/dbus/user_bus_socket"
rm -f "$OUT"
busctl --user call org.nemomobile.lipstick \
    /org/nemomobile/lipstick/screenshot \
    org.nemomobile.lipstick saveScreenshot s "$OUT" >/dev/null
[ -s "$OUT" ] || { echo "скриншот не создан" >&2; exit 1; }
echo "$OUT"
