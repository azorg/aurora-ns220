#!/bin/sh
# File: "install-ovl-hosts.sh"
#
# Прописывает в /etc/hosts планшета (Aurora) и в /etc/hosts Debian-chroot
# все оверлейные хосты cnet node-* из канонического файла
# ~/work/cnet/doc/test-cluster/ovl-hosts.
#
# Блок управляемый: строки между маркерами
#   # >>> cnet overlay hosts >>>
#   # <<< cnet overlay hosts <<<
# перезаписываются при повторном запуске, остальной /etc/hosts не трогается.
#
# Запуск на планшете (root): sh install-ovl-hosts.sh <путь-к-ovl-hosts>
set -eu

SRC=${1:-}
[ -n "$SRC" ] && [ -f "$SRC" ] || {
	echo "usage: $0 <ovl-hosts>" >&2
	exit 2
}

# Только «живые» строки оверлея: <IPv6>\t<node-имя>, без комментариев.
ENTRIES=$(grep -E '^fd00:[0-9a-fA-F:]+[[:space:]]+node-[a-z0-9-]+' "$SRC" || true)
[ -n "$ENTRIES" ] || { echo "в $SRC нет строк оверлея" >&2; exit 1; }

apply() {
	f=$1
	[ -f "$f" ] || return 0
	tmp=$f.ns220.$$
	# вырезать прежний блок (если был)
	awk '
		/^# >>> cnet overlay hosts >>>$/ { skip = 1; next }
		/^# <<< cnet overlay hosts <<<$/ { skip = 0; next }
		!skip { print }
	' "$f" > "$tmp"
	{
		echo '# >>> cnet overlay hosts >>>'
		echo '# Оверлейные адреса cnet (node-*). Сгенерировано install-ovl-hosts.sh'
		echo '# из ~/work/cnet/doc/test-cluster/ovl-hosts; блок перезаписывается.'
		printf '%s\n' "$ENTRIES"
		echo '# <<< cnet overlay hosts <<<'
	} >> "$tmp"
	cat "$tmp" > "$f"
	rm -f "$tmp"
	echo "== $f: $(printf '%s\n' "$ENTRIES" | wc -l) записей"
}

apply /etc/hosts
apply /home/chroot/debian/etc/hosts
