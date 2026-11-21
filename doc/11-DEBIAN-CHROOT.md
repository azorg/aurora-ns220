# 11. Debian chroot на Aurora (полный доступ к пакетам Linux)

Зачем: в Aurora-репозитории нет `nmap`, `mc`, `vim`, `tmux`, `ncdu`, свежих `gcc/go/rust`.
Chroot с Debian даёт `apt` и весь дебиановский стек **на том же устройстве**, не мешая системе.

Ограничение по графике: GUI внутри chroot не выйдет (нет доступа к `lipstick`), но CLI —
полностью. Для GUI — Qt/Fyne вне chroot.

## 11.1 Сборка rootfs (на хосте, один раз)

```bash
# хост: Debian/Ubuntu с debootstrap
sudo apt-get install -y debootstrap
sudo mkdir -p /tmp/debroot
sudo debootstrap --foreign --arch=armhf --variant=minbase \
     --include=apt,ca-certificates bookworm /tmp/debroot http://deb.debian.org/debian
sudo tar -C /tmp/debroot -czf /tmp/debian-armhf-rootfs.tgz .
# ~84 МБ
```

`--foreign` не требует qemu: второй этап выполняется уже на планшете (он armv7).

## 11.2 Разворачивание на планшете

```bash
mount -o remount,exec /home          # /home по умолчанию noexec!
mkdir -p /home/chroot/debian
tar -xzf /tmp/debian-armhf-rootfs.tgz -C /home/chroot/debian

# монтирование и второй этап
R=/home/chroot/debian
for d in proc sys dev dev/pts; do mkdir -p $R/$d; mount --bind /$d $R/$d; done
cp /etc/resolv.conf $R/etc/resolv.conf
echo "127.0.0.1 localhost" > $R/etc/hosts

chroot $R /debootstrap/debootstrap --second-stage    # ~2 минуты
```

## 11.3 Обязательные фиксы apt (иначе «Could not resolve»)

```bash
R=/home/chroot/debian
# 1) публичный DNS (connman-резолвер внутри chroot не отвечает пользователю _apt)
printf "nameserver 8.8.8.8\nnameserver 1.1.1.1\n" > $R/etc/resolv.conf

# 2) IPv4-only: у планшета нет IPv6, apt получает AAAA и падает
# 3) отключить sandbox: _apt не резолвит DNS
cat > $R/etc/apt/apt.conf.d/99aurora <<'EOS'
APT::Sandbox::User "root";
Acquire::ForceIPv4 "true";
EOS

# 4) sources
cat > $R/etc/apt/sources.list <<'EOS'
deb http://deb.debian.org/debian bookworm main contrib non-free non-free-firmware
deb http://deb.debian.org/debian bookworm-updates main contrib non-free non-free-firmware
deb http://security.debian.org/debian-security bookworm-security main contrib non-free non-free-firmware
EOS

chroot $R apt-get update
```

## 11.4 Обёртка `/usr/local/bin/debian-chroot`

Скрипт `scripts/debian-chroot` (лежит рядом с документацией):

* `mount -o remount,exec /home` (иначе внутри chroot ничего не запускается);
* bind-mount `proc/sys/dev/dev/pts`, tmpfs на `/tmp`;
* копирует `resolv.conf`, кладёт `99aurora` (sandbox+IPv4) при необходимости;
* без аргументов — интерактивный `bash -l`, с аргументами — команда в chroot;
* при выходе размонтирует (trap).

Использование:

```bash
debian-chroot                       # shell
debian-chroot apt-get install -y tmux
debian-chroot nmap -sP 192.168.1.0/24
```

## 11.5 Что уже проверено и установлено

```
Debian GNU/Linux 12 (bookworm), armv7l
tmux 3.3a
GNU Midnight Commander 4.8.29
VIM 9.0
Nmap 7.93
ncdu 1.18
git 2.39.5
```

## 11.6 Замечания

* Ядро общее с Aurora (3.18.79) — старые syscalls; большинство CLI-утилит работает,
  но совсем новые вещи (io_uring и т.п.) могут не работать.
* `apt` с `Sandbox::User root` — компромисс (не sandbox), для dev-стенда приемлемо.
* В chroot **нельзя** собирать GUI-бинарники, которые потом запускать на Aurora:
  они слинкуются с glibc 2.36 и не запустятся на glibc 2.30 системы. Собирать GUI нужно
  в самой Aurora (см. `03-TOOLCHAIN.md`).
* Chroot можно использовать для сборки **статических** утилит (`CGO_ENABLED=0`), они запустятся.
* Размер: rootfs ~700 МБ после установки пакетов; держать в `/home` (там 4–5 ГБ свободно).
