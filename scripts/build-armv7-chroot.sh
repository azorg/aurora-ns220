#!/bin/sh
# File: "build-armv7-chroot.sh"
#
# Сборка на ХОСТЕ (x86_64) Linux/armv7 (armhf, GOARM=7) бинарников для
# Debian-chroot на планшете NS220: cnet и clear-ssh (cssh/csshd/cdialog).
#
#   scripts/build-armv7-chroot.sh [cnet|clearssh|all] [--out DIR]
#
# Зачем отдельный скрипт: обе программы кросс-собираются не «из коробки».
#   1) clear-ssh (как и cnet) собирается с CGO_ENABLED=0, но:
#      * go-fuse v2.12.1 не компилируется на 32-битных платформах
#        (syscall.Splice возвращает int, а в библиотеке ожидается int64):
#        patches/go-fuse-2.12.1-splice-32bit.patch;
#      * Go линкует бинарь с внешним линкером и подставляет ELF-interpreter
#        /lib/ld-linux.so.3 (soft-float), которого нет в armhf:
#        после сборки правим интерпретатор на /lib/ld-linux-armhf.so.3
#        утилитой patchelf.
#   2) cnet на 32-битных платформах требует правки 0052 (переполнение
#      BufSizeDecode) — она уже в репозитории cnet; отдельного патча нет.
#
# Результат: $OUT/{cnet,cssh,csshd,cdialog} (по умолчанию /tmp/armbuild).
set -eu

WHAT=${1:-all}
OUT=/tmp/armbuild
[ "${2:-}" = "--out" ] && OUT=${3:-$OUT}
mkdir -p "$OUT"

HERE=$(cd "$(dirname "$0")" && pwd)
PATCHES=$(cd "$HERE/../patches" && pwd)
WORK=${WORK:-$HOME/build/ns220-arm}
GOFUSE_VER=v2.12.1
GOFUSE_SRC=$WORK/go-fuse

build_cnet() {
	repo=${CNET_REPO:-$HOME/work/cnet}
	echo ">>> cnet ($repo): linux/arm GOARM=7"
	ver=$(cat "$repo/VERSION.txt" 2>/dev/null || echo dev)
	hash=$(git -C "$repo" rev-parse HEAD | head -c 7)
	(cd "$repo" && GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
		go build -trimpath \
		-ldflags "-s -w -X=cnet/version.Version=$ver \
			-X=cnet/version.GitHash=$hash \
			-X=cnet/version.BuildTime=$(date '+%Y.%m.%d-%H:%M')" \
		-o "$OUT/cnet" ./cmd/cnet)
	file "$OUT/cnet"
}

prepare_gofuse() {
	mod=$(go env GOMODCACHE)
	src=$mod/github.com/hanwen/go-fuse/v2@$GOFUSE_VER
	if [ ! -d "$GOFUSE_SRC" ]; then
		echo ">>> копия go-fuse $GOFUSE_VER -> $GOFUSE_SRC"
		mkdir -p "$GOFUSE_SRC"
		cp -a "$src/." "$GOFUSE_SRC/"
		chmod -R u+w "$GOFUSE_SRC"
		patch -p0 -d "$GOFUSE_SRC" < "$PATCHES/go-fuse-2.12.1-splice-32bit.patch"
	fi
}

build_clearssh() {
	repo=${CSSH_REPO:-$HOME/work/clear-ssh}
	prepare_gofuse
	# modfile с заменой go-fuse на локальную (пропатченную) копию:
	# go.mod репозитория не трогаем.
	MF=$WORK/go.armv7.mod
	cp "$repo/go.mod" "$MF"
	cp "$repo/go.sum" "$WORK/go.armv7.sum" 2>/dev/null || true
	grep -q 'hanwen/go-fuse' "$MF" || true
	echo "replace github.com/hanwen/go-fuse/v2 => $GOFUSE_SRC" >> "$MF"

	echo ">>> clear-ssh ($repo): linux/arm GOARM=7"
	ver=$(cat "$repo/VERSION.txt" 2>/dev/null || echo dev)
	for b in cssh csshd; do
		(cd "$repo" && GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
			go build -modfile="$MF" -tags gogost -trimpath \
			-ldflags "-s -w -X=cssh/version.Version=$ver" \
			-o "$OUT/$b.raw" ./cmd/$b)
		# interpreter soft-float -> hard-float armhf
		cp "$OUT/$b.raw" "$OUT/$b"
		rm -f "$OUT/$b.raw"
		patchelf --set-interpreter /lib/ld-linux-armhf.so.3 "$OUT/$b"
		file "$OUT/$b"
	done
	# cdialog: статический, без cgo — достаточно обычной кросс-сборки
	(cd "$repo" && GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
		go build -trimpath -ldflags "-s -w" -o "$OUT/cdialog" ./cmd/cdialog)
	file "$OUT/cdialog"
}

case "$WHAT" in
cnet) build_cnet ;;
clearssh | clear-ssh) build_clearssh ;;
all) build_cnet; build_clearssh ;;
*) echo "usage: $0 [cnet|clearssh|all] [--out DIR]" >&2; exit 2 ;;
esac

echo ">>> готово: $OUT"
ls -lh "$OUT" 2>/dev/null || true
