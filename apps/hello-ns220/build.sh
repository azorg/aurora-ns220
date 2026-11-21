#!/bin/sh
# Сборка hello-ns220 НА ПЛАНШЕТЕ (Aurora, armv7l).
#   ./build.sh            -> ./hello-ns220
#   OUT=/tmp/hello ./build.sh
#
# Требуется стенд: /home/opt (go127, aurora-fyne/go-glfw34, usr/include, libs, pc).
set -e
HERE=$(cd "$(dirname "$0")" && pwd)
cd "$HERE"

ROOT=${AURORA_ROOT:-/home/opt}
GO=${GO:-$ROOT/go127/bin/go}

export PATH="$ROOT/go127/bin:$PATH"
export GOPATH=${GOPATH:-$ROOT/gopath127}
export GOMODCACHE=${GOMODCACHE:-$GOPATH/pkg/mod}
export GOCACHE=${GOCACHE:-$ROOT/gocache127}
export GOFLAGS=${GOFLAGS:--mod=mod}

export PKG_CONFIG_PATH="$ROOT/pc:$ROOT/usr/lib/pkgconfig:/usr/lib/pkgconfig"
export CGO_CFLAGS="-D_GNU_SOURCE -DEGL_NO_X11 -I$ROOT/usr/include"
export CGO_LDFLAGS="-L$ROOT/libs -lwayland-client -lwayland-cursor -lwayland-egl \
-lxkbcommon -lEGL -lGLESv2 -lpthread -lm -ldl"

# go.sum берём из общего стенда (оффлайн: модули уже в GOMODCACHE)
if [ ! -f go.sum ] && [ -f /home/user/clear-ssh/go.sum ]; then
    cp /home/user/clear-ssh/go.sum go.sum
fi

# /home в Aurora монтируется noexec — без remount go/бинарь не запускаются.
if ! [ -x "$GO" ]; then
    mount -o remount,exec /home 2>/dev/null || true
fi
if ! [ -x "$GO" ]; then
    echo "[!] $GO не исполняется: нужен 'mount -o remount,exec /home' (root)" >&2
    exit 1
fi

echo "[*] go: $("$GO" version)"
echo "[*] tags: wayland gles egl"
"$GO" build -tags "wayland gles egl" -o "${OUT:-$HERE/hello-ns220}" "$@" .
echo "[*] готово: ${OUT:-$HERE/hello-ns220}"
