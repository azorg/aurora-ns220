#!/bin/sh
# File: "status.sh" — состояние сервисов внутри Debian-chroot на NS220.
set -u

# Порт csshd (должен совпадать с network.ports в /etc/cssh/csshd.yaml).
CSSHD_PORT=${CSSHD_PORT:-2222}

alive() { [ -f "$1" ] && kill -0 "$(cat "$1" 2>/dev/null)" 2>/dev/null; }

# pid_by_cmdline шаблон - pid процесса, запущенного не через /opt/ns220
# (например, systemd-юнитом csshd-chroot/cnet-chroot).
pid_by_cmdline() { pgrep -f "$1" 2>/dev/null | head -1; }

echo "--- chroot: $(cat /etc/debian_version 2>/dev/null) $(uname -m)"
CSSHD_PID=""
alive /run/csshd.pid && CSSHD_PID=$(cat /run/csshd.pid)
[ -n "$CSSHD_PID" ] || CSSHD_PID=$(pid_by_cmdline 'csshd -D -f /etc/cssh/csshd.yaml')
if [ -n "$CSSHD_PID" ]; then
	if _hex=$(printf '%04X' "$CSSHD_PORT") &&
		{ awk -v want="$_hex" 'NR > 1 { n = split($2, a, ":"); if (n == 2 && a[2] == want && $4 == "0A") f = 1 } END { exit !f }' /proc/net/tcp 2>/dev/null ||
			awk -v want="$_hex" 'NR > 1 { n = split($2, a, ":"); if (n == 2 && a[2] == want && $4 == "0A") f = 1 } END { exit !f }' /proc/net/tcp6 2>/dev/null; }; then
		echo "csshd:  running (pid $CSSHD_PID), слушает :$CSSHD_PORT"
	else
		echo "csshd:  running (pid $CSSHD_PID), НО :$CSSHD_PORT не слушается"
	fi
else
	echo "csshd:  stopped"
fi
CNET_PID=""
alive /run/cnet.pid && CNET_PID=$(cat /run/cnet.pid)
[ -n "$CNET_PID" ] || CNET_PID=$(pid_by_cmdline 'cnet run --role node')
if [ -n "$CNET_PID" ]; then
	echo "cnet:   running (pid $CNET_PID)"
	ip -6 addr show cnet 2>/dev/null | awk '/inet6/{print "        tunnel: "$2}'
	/usr/local/bin/cnet node --config /etc/cnet/config/node.yaml --showid 2>/dev/null |
		sed -n 's/^/- IPv6: /p'
else
	echo "cnet:   stopped"
fi
echo "--- домашний каталог Aurora:"
ls -ld /home/aurora >/dev/null 2>&1 && echo "  /home/aurora -> $(head -c 60 /dev/null; ls /home/aurora 2>/dev/null | wc -l) элементов" ||
	echo "  /home/aurora не смонтирован"
