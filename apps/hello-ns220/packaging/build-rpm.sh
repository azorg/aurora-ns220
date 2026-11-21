#!/bin/sh
# Сборка RPM демо-приложения НА ПЛАНШЕТЕ (Aurora, armv7hl).
#
#   ./packaging/build-rpm.sh          # собрать пакет (бинарник берётся из ..)
#   SIGN=1 ./packaging/build-rpm.sh   # ещё и подписать своим GPG-ключом
#
# Переменные: RPM_TOPDIR (по умолчанию ~/rpmbuild), GPG_NAME (имя ключа,
# по умолчанию aurora-ns220@local), OUTDIR (куда положить готовый rpm).
set -e
HERE=$(cd "$(dirname "$0")" && pwd)          # apps/hello-ns220/packaging
APP=$(cd "$HERE/.." && pwd)                  # apps/hello-ns220
TOPDIR=${RPM_TOPDIR:-$HOME/rpmbuild}
OUTDIR=${OUTDIR:-$APP/packaging}
GPG_NAME=${GPG_NAME:-aurora-ns220@local}
# Ключ подписи храним у пользователя (вне git): /home/user/.gnupg.
# Сборку можно запускать и от root (devel-su) — GNUPGHOME передаётся явно.
export GNUPGHOME=${GNUPGHOME:-/home/user/.gnupg}
SPEC=$HERE/aurora-hello-ns220.spec

# 1. бинарник
if [ ! -x "$APP/hello-ns220" ]; then
    echo "[*] бинарника нет — собираю"
    ( cd "$APP" && ./build.sh )
fi

# 2. дерево rpmbuild
mkdir -p "$TOPDIR/BUILD" "$TOPDIR/RPMS" "$TOPDIR/SOURCES" "$TOPDIR/SPECS" "$TOPDIR/SRPMS"
cp -f "$APP/hello-ns220" "$TOPDIR/SOURCES/hello-ns220"
cp -f "$HERE/aurora-hello-ns220.desktop" "$TOPDIR/SOURCES/"
cp -f "$HERE/aurora-hello-ns220.png" "$TOPDIR/SOURCES/"
cp -f "$HERE/aurora-hello-ns220-86.png" "$TOPDIR/SOURCES/"
cp -f "$HERE/aurora-hello-ns220-launch.sh" "$TOPDIR/SOURCES/"

# 3. сборка
echo "[*] rpmbuild (topdir=$TOPDIR)"
# busybox-xargs в Aurora не умеет -d: brp-strip-static-archive падает,
# поэтому глушим его (см. doc/06-PACKAGING.md); debuginfo не нужен.
rpmbuild -bb --define "_topdir $TOPDIR" \
    --define "__brp_strip_static_archive /bin/true" \
    --define "debug_package %{nil}" \
    "$SPEC" 2>&1 | tail -25

RPM=$(ls -t "$TOPDIR"/RPMS/*/aurora-hello-ns220-*.rpm 2>/dev/null | head -1)
[ -n "$RPM" ] || { echo "[!] rpm не найден" >&2; exit 1; }
echo "[*] пакет: $RPM"

# 4. подпись (своим GPG-ключом; доверия OMP это не даёт — см. план 0009)
if [ "${SIGN:-0}" = 1 ]; then
    if ! gpg2 --batch --list-secret-keys "$GPG_NAME" >/dev/null 2>&1; then
        echo "[*] создаю GPG-ключ $GPG_NAME (без пароля)"
        mkdir -p "$GNUPGHOME"; chmod 700 "$GNUPGHOME"
        cat > /tmp/aurora-gpg.params <<EOF
%no-protection
Key-Type: RSA
Key-Length: 3072
Key-Usage: sign
Name-Real: aurora-ns220
Name-Email: $GPG_NAME
Expire-Date: 0
%commit
EOF
        gpg2 --batch --gen-key /tmp/aurora-gpg.params >/dev/null 2>&1
    fi
    echo "[*] rpm --addsign"
    rpm --addsign --define "_gpg_name $GPG_NAME" \
        --define "__gpg /usr/bin/gpg2" "$RPM" < /dev/null
    echo "[*] проверка подписи:"
    rpm -Kv "$RPM" || true
    cp -f "$RPM" "$OUTDIR/aurora-hello-ns220-arm7-signed.rpm"
    echo "[*] готово: $OUTDIR/aurora-hello-ns220-arm7-signed.rpm"
else
    cp -f "$RPM" "$OUTDIR"/
    echo "[*] готово (без подписи): $OUTDIR/$(basename "$RPM")"
fi

ls -l "$OUTDIR"/aurora-hello-ns220-*.rpm
echo
rpm -qip "$OUTDIR"/aurora-hello-ns220-*.rpm 2>/dev/null | sed -n '1,14p'
rpm -qpR "$OUTDIR"/aurora-hello-ns220-*.rpm 2>/dev/null | head -20
