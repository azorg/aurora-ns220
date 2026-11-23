#!/bin/sh
# File: "ping-wrapper.sh" — `ping` в Debian-chroot на NS220.
#
# Зачем: в Aurora (Sailfish) политика безопасности не даёт процессам,
# запущенным от root внутри chroot, создавать ICMP-сокеты:
#     /bin/ping: socket: Permission denied
# при этом unprivileged ICMP (ping socket, net.ipv4.ping_group_range =
# 0..2147483647) для uid 100000 с группой inet=3003 работает.
# Обёртка от root перезапускает ping от пользователя user (та же
# учётная запись, что и в Aurora), поэтому `ping` работает в обоих
# случаях. Установка: /usr/local/bin/ping (PATH раньше /usr/bin).
#
# Для ICMPv6 и для ya.ru (AAAA) нужен явный -4: IPv6-аплинка у планшета
# нет (IPv6 есть только в оверлее fd00::/16).
if [ "$(id -u)" = 0 ]; then
    exec setpriv --reuid=100000 --regid=100000 --groups=3003 \
        /usr/bin/ping "$@"
fi
exec /usr/bin/ping "$@"
