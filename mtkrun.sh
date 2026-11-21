#!/bin/bash
# usage: mtkrun.sh <mtk-client-args...>
# Starts the pty bridge (which performs the MTK handshake and holds the preloader session),
# waits for a successful handshake, then runs mtkclient through /tmp/ttyMTK.
set -u
BR=/tmp/bridge.pid
LOG=/tmp/bridge.log

[ -f "$BR" ] && kill "$(cat "$BR")" 2>/dev/null
sleep 0.5
rm -f "$LOG"
setsid nohup python3 /tmp/mtk_bridge7.py 3600 </dev/null >/dev/null 2>&1 &
echo $! > "$BR"
echo "[*] bridge started (pid $(cat $BR)), waiting for handshake..."
for i in $(seq 1 1800); do
    if grep -q "BRIDGE HS OK" "$LOG" 2>/dev/null; then
        echo "[*] handshake OK after $((i/10))s"
        break
    fi
    sleep 0.1
done
if ! grep -q "BRIDGE HS OK" "$LOG" 2>/dev/null; then
    echo "[!] no handshake in 180s; last log lines:"
    tail -5 "$LOG"
    exit 1
fi

cd /tmp/mtkclient-main || exit 1
echo "[*] running: python3 mtk.py $* --serialport /tmp/ttyMTK"
timeout 900 python3 mtk.py "$@" --serialport /tmp/ttyMTK
rc=$?
echo "[*] mtk exit=$rc"
kill "$(cat $BR)" 2>/dev/null
exit $rc
