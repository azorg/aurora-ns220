#!/bin/sh
# File: "ns220-chroot-svc.sh"
#
# Постоянный Debian-chroot на NS220: монтирование rootfs и запуск сервисов
# внутри него (csshd на порту 222 и узел cnet node-ns220).
#
# Зачем отдельный скрипт: debian-chroot/debian-enter монтируют proc/sys/dev
# только на время сессии и размонтируют их при выходе. Сервисам (csshd,
# cnet) нужны постоянные /proc, /sys, /dev, /tmp — их держит этот скрипт.
# Пока стенд поднят, существует /run/ns220-svc.active; debian-chroot видит
# этот файл и не трогает монтирования (см. scripts/debian-chroot).
#
# Установка (на планшете):
#   sudo install -m0755 ns220-chroot-svc.sh /usr/local/bin/ns220-svc
#   sudo install -m0644 ns220-chroot.service /etc/systemd/system/
#   sudo systemctl daemon-reload && sudo systemctl enable --now ns220-chroot
#
# Использование:
#   ns220-svc mount|umount        — только монтирования (для отдельных юнитов
#                                   csshd-chroot/cnet-chroot)
#   ns220-svc start|stop|restart  — монтирования + оба сервиса внутри chroot
#   ns220-svc status

set -u

R=${NS220_CHROOT_ROOT:-/home/chroot/debian}
FLAG=/run/ns220-svc.active
INCHROOT=/opt/ns220

die() { echo "ns220-svc: $*" >&2; exit 1; }

mount_chroot() {
	[ -d "$R" ] || die "нет rootfs: $R"
	# /home в Aurora noexec — иначе внутри chroot ничего не запустится
	mount -o remount,exec /home 2>/dev/null || true
	# ВНИМАНИЕ: bind-монтировать rootfs на себя ради suid НЕЛЬЗЯ:
	# после этого ядро не может восстановить путь /proc/self/exe, и
	# csshd не может запустить privsep-worker
	# ("privsep executable: readlink /proc/self/exe: ENOENT").
	# Поэтому setuid-биты (/home смонтирован nosuid) в chroot не работают:
	# sudo/passwd внутри chroot не запускаются. root в chroot — отдельный
	# вход (`cssh -p 2222 root@...`), пароли — через `chpasswd`.
	for d in proc sys dev dev/pts; do
		mkdir -p "$R/$d"
		mountpoint -q "$R/$d" 2>/dev/null || mount --bind "/$d" "$R/$d"
	done
	mkdir -p "$R/tmp"
	mountpoint -q "$R/tmp" 2>/dev/null ||
		mount -t tmpfs -o mode=1777 tmpfs "$R/tmp"
	# DNS в chroot: публичные резолверы. В Aurora /etc/resolv.conf — это
	# заглушка connman (nameserver ::1 и 127.0.0.1); из chroot она работает
	# нестабильно и отдаёт в первую очередь AAAA, хотя IPv6-аплинка у
	# планшета нет (IPv6 есть только в оверлее fd00::/16). Публичные
	# резолверы + gai.conf с приоритетом IPv4 решают обе проблемы.
	mkdir -p "$R/etc"
	cat > "$R/etc/resolv.conf" <<'RESOLV'
# cnet-стенд NS220 (Debian chroot): публичные DNS.
# Причина: в Aurora resolv.conf — заглушка connman (::1/127.0.0.1),
# из chroot она ненадёжна; IPv6-аплинка нет, IPv6 только в оверлее.
nameserver 8.8.8.8
nameserver 1.1.1.1
nameserver 9.9.9.9
RESOLV
	# Приоритет IPv4 в getaddrinfo (RFC 3484), иначе curl/apt/pi
	# пробуют недоступный AAAA и ждут таймаут.
	touch "$R/etc/gai.conf"
	grep -q '^precedence ::ffff:0:0/96' "$R/etc/gai.conf" 2>/dev/null ||
		printf 'precedence ::ffff:0:0/96  100\n' >> "$R/etc/gai.conf"
	# hostname -> 127.0.0.1 один раз (иначе sudo ругается
	# "unable to resolve host"): убрать прежние строки и добавить одну.
	if [ -n "${HOSTNAME:-}" ]; then
		grep -v "127.0.0.1[[:space:]]*${HOSTNAME}" "$R/etc/hosts" 2>/dev/null > "$R/etc/hosts.tmp" || true
		[ -s "$R/etc/hosts.tmp" ] && mv "$R/etc/hosts.tmp" "$R/etc/hosts"
		printf '127.0.0.1\t%s\n' "$HOSTNAME" >> "$R/etc/hosts"
	fi

	# Домашний каталог пользователя Aurora и медиа внутри chroot:
	#   /home/aurora  <- /home/user        (скриншоты, загрузки, документы...)
	#   /media/aurora <- /run/media/user   (SD-карта и другие медиа)
	bind_dir "${AURORA_HOME:-/home/user}" "$R/home/aurora"
	bind_dir "${AURORA_MEDIA:-/run/media/user}" "$R/media/aurora"
	[ -L "$R/root/aurora" ] || ln -sfn /home/aurora "$R/root/aurora" 2>/dev/null || true
	[ -L "$R/aurora" ] || ln -sfn /home/aurora "$R/aurora" 2>/dev/null || true
}

bind_dir() {  # $1 = каталог хоста, $2 = цель внутри chroot
	[ -d "$1" ] || return 0
	mkdir -p "$2"
	mountpoint -q "$2" 2>/dev/null || mount --bind "$1" "$2"
}

umount_chroot() {
	for d in media/aurora home/aurora tmp dev/pts dev sys proc; do
		# снимать только то, что мы сами примонтировали
		[ -d "$R/$d" ] || continue
		while mountpoint -q "$R/$d" 2>/dev/null; do
			umount "$R/$d" 2>/dev/null || umount -l "$R/$d" 2>/dev/null || break
		done
	done
}

in_chroot() {
	chroot "$R" "$@"
}

case "${1:-}" in
mount)
	# Только смонтировать стенд (chroot + /home/aurora) и оставить флаг:
	# сервисы (csshd, cnet) запускают отдельные systemd-юниты
	# csshd-chroot.service/cnet-chroot.service.
	mount_chroot
	touch "$FLAG"
	echo "chroot mounted: $R (флаг $FLAG)"
	;;
umount)
	rm -f "$FLAG"
	umount_chroot
	echo "chroot unmounted: $R"
	;;
start)
	mount_chroot
	touch "$FLAG"
	in_chroot "$INCHROOT/start.sh" || die "ошибка запуска сервисов в chroot"
	;;
stop)
	[ -x "$R$INCHROOT/stop.sh" ] && in_chroot "$INCHROOT/stop.sh"
	rm -f "$FLAG"
	umount_chroot
	;;
restart)
	"$0" stop
	"$0" start
	;;
status)
	if [ -f "$FLAG" ]; then
		echo "chroot: mounted ($R)"
	else
		echo "chroot: not mounted by ns220-svc"
	fi
	[ -x "$R$INCHROOT/status.sh" ] && in_chroot "$INCHROOT/status.sh"
	;;
*)
	echo "usage: $0 mount|umount|start|stop|restart|status" >&2
	exit 2
	;;
esac
