#!/bin/bash
# С хоста: доставить исходники hello-ns220 на планшет, собрать там (через devel-su —
# кэш модулей root-овый), при желании запустить и снять PNG окна самим приложением
# (lipstick-скриншот через D-Bus закрыт политикой: «Access denied»).
#
#   ./scripts/deploy.sh              # доставка + сборка
#   ./scripts/deploy.sh --run        # + запуск (Mali, полный экран)
#   ./scripts/deploy.sh --run --shot # + снимок окна (PNG в текущем каталоге)
#
# Переменные: DEV=user@192.168.2.15  PASS=1234  DIR=/home/user/hello-ns220
set -euo pipefail
DEV=${DEV:-user@192.168.2.15}
PASS=${PASS:-1234}
DIR=${DIR:-/home/user/hello-ns220}
HERE=$(cd "$(dirname "$0")/.." && pwd)
SSH="sshpass -p $PASS ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=8"
unset HTTP_PROXY HTTPS_PROXY || true

RUN=0; SHOT=0
for a in "$@"; do
  case "$a" in
    --run) RUN=1 ;;
    --shot) SHOT=1; RUN=1 ;;
    *) echo "неизвестный аргумент: $a" >&2; exit 2 ;;
  esac
done

echo "[*] копирую исходники в $DEV:$DIR"
tar czf - -C "$HERE" --exclude=hello-ns220 --exclude=hello-ns220.armv7 \
    --exclude='*.png' --exclude='*.rpm' --exclude=.git . \
  | $SSH "$DEV" "mkdir -p $DIR && tar xzf - -C $DIR"

echo "[*] сборка на планшете (devel-su: GOMODCACHE/GOCACHE принадлежат root)"
$SSH "$DEV" "echo $PASS | devel-su env GOCACHE=/tmp/gocache-hello GOFLAGS=-mod=mod \
    sh -c 'cd $DIR && chmod +x build.sh run.sh scripts/*.sh packaging/*.sh && ./build.sh'" \
  2>&1 | grep -v -e 'Warning: Permanently' -e '^Password' | tail -5

if [ "$RUN" = 1 ]; then
  echo "[*] запуск на планшете"
  if [ "$SHOT" = 1 ]; then
    $SSH "$DEV" "rm -f /home/user/hello-shot.png; cd $DIR && \
        ( ./run.sh -selftest -shot /home/user/hello-shot.png -shot-delay 8s -stats 3s \
          > /home/user/hello-run.log 2>&1 & echo \$! > /home/user/hello.pid ); \
        sleep 12; kill \$(cat /home/user/hello.pid) 2>/dev/null; sleep 1; \
        grep -E 'SELFTEST done|SHOT|STATS' /home/user/hello-run.log | tail -4" \
      2>&1 | grep -v 'Warning: Permanently'
    scp -q -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        "$DEV:/home/user/hello-shot.png" "$HERE/hello-shot.png" && \
        echo "[*] снимок: $HERE/hello-shot.png"
  else
    $SSH "$DEV" "cd $DIR && ./run.sh" 2>&1 | grep -v 'Warning: Permanently'
  fi
fi
