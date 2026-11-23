#!/bin/sh
# File: "stop.sh" — остановка сервисов внутри Debian-chroot на NS220.
set -u
for p in cnet csshd; do
	f=/run/$p.pid
	if [ -f "$f" ]; then
		pid=$(cat "$f" 2>/dev/null)
		if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
			kill "$pid" 2>/dev/null && echo "$p: SIGTERM -> $pid"
			i=0
			while kill -0 "$pid" 2>/dev/null && [ $i -lt 10 ]; do
				sleep 1
				i=$((i + 1))
			done
			kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null
		fi
		rm -f "$f"
	fi
done
# cnet-туннель мог остаться (например, после kill -9)
ip link show cnet >/dev/null 2>&1 && ip link del cnet 2>/dev/null
echo "services stopped"
