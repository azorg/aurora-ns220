#!/bin/sh
# File: "start.sh" — запуск сервисов внутри Debian-chroot на NS220.
# Кладётся в chroot как /opt/ns220/start.sh; вызывается из ns220-svc на хосте.
set -u

mkdir -p /var/log/clearssh /var/log/cnet /run /var/lib/cnet/store

# Порт csshd (должен совпадать с network.ports в /etc/cssh/csshd.yaml).
CSSHD_PORT=${CSSHD_PORT:-2222}

alive() { [ -f "$1" ] && kill -0 "$(cat "$1" 2>/dev/null)" 2>/dev/null; }

# port_listen PORT - есть ли внутри chroot слушатель на TCP-порту.
# Читаем /proc/net/tcp{,6}: в chroot может не быть iproute2/ss, а
# /proc есть всегда. Порт в /proc/net/tcp — hex в поле local_address.
port_listen() {
	_hex=$(printf '%04X' "$1")
	for _f in /proc/net/tcp /proc/net/tcp6; do
		[ -r "$_f" ] || continue
		awk -v want="$_hex" '
			NR > 1 {
				n = split($2, a, ":")
				if (n == 2 && a[2] == want && $4 == "0A") { f = 1 }
			}
			END { exit !f }' "$_f" && return 0
	done
	return 1
}

# zombie_pid FILE - снять pid-файл, если процесс уже не жив
zombie_pid() {
	alive "$1" || rm -f "$1"
}

# --- csshd (ClearSSH server, порт 222) ---------------------------------
zombie_pid /run/csshd.pid
if alive /run/csshd.pid && port_listen "$CSSHD_PORT"; then
	echo "csshd: already running (pid $(cat /run/csshd.pid))"
else
	nohup /usr/local/bin/csshd -D -f /etc/cssh/csshd.yaml \
		>>/var/log/clearssh/csshd.out 2>&1 &
	echo $! >/run/csshd.pid
	echo "csshd: started (pid $(cat /run/csshd.pid))"
fi

# --- cnet node (node-ns220, IP-туннель, выход через node-de) -----------
zombie_pid /run/cnet.pid
if alive /run/cnet.pid; then
	echo "cnet: already running (pid $(cat /run/cnet.pid))"
else
	CNET_ROLE=node CNET_CONFIG=/etc/cnet/config/node.yaml \
		nohup /usr/local/bin/cnet run --role node \
		--config /etc/cnet/config/node.yaml --iptun=on \
		>>/var/log/cnet/node.out 2>&1 &
	echo $! >/run/cnet.pid
	echo "cnet: started (pid $(cat /run/cnet.pid))"
fi

# Дождаться готовности портов (csshd поднимается асинхронно,
# privsep-monitor + bind могут занять несколько секунд).
i=0
while [ $i -lt 20 ]; do
	i=$((i + 1))
	port_listen "$CSSHD_PORT" && [ -f /run/cnet.pid ] && kill -0 "$(cat /run/cnet.pid)" 2>/dev/null && break
	sleep 1
done
exec /opt/ns220/status.sh
